/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package iterator

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	fakesnapshot "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	fakeSmsCR "github.com/kubernetes-csi/external-snapshot-metadata/client/clientset/versioned/fake"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/internal/runtime"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/k8sclientmocks"
)

func TestValidateArgs(t *testing.T) {
	var err error

	args := Args{}
	err = args.Validate()
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidArgs)
	assert.ErrorContains(t, err, "Emitter")

	args.Emitter = &JSONEmitter{}
	err = args.Validate()
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidArgs)
	assert.ErrorContains(t, err, "Namespace")

	args.Namespace = "namespace"
	err = args.Validate()
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidArgs)
	assert.ErrorContains(t, err, "SnapshotName")

	args.SnapshotName = "snapshot"
	err = args.Validate()
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidArgs)
	// assert.ErrorContains(t, err, "ServiceAccount")
	assert.ErrorContains(t, err, "KubeClient")

	args.Clients.KubeClient = fake.NewSimpleClientset()
	err = args.Validate()
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidArgs)
	assert.ErrorContains(t, err, "SnapshotClient")

	args.Clients.SnapshotClient = fakesnapshot.NewSimpleClientset()
	err = args.Validate()
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidArgs)
	assert.ErrorContains(t, err, "SmsCRClient")

	args.Clients.SmsCRClient = fakeSmsCR.NewSimpleClientset()
	assert.NoError(t, args.Validate())

	args.TokenExpirySecs = -1
	err = args.Validate()
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidArgs)
	assert.ErrorContains(t, err, "TokenExpirySecs")

	args.TokenExpirySecs = 0
	args.MaxResults = -1
	err = args.Validate()
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidArgs)
	assert.ErrorContains(t, err, "MaxResults")

	// Invoke through GetSnapshotMetadata to cover that code path.
	err = GetSnapshotMetadata(context.Background(), args)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidArgs)
	assert.ErrorContains(t, err, "MaxResults")

	args.MaxResults = 5
	args.SAName = "serviceAccount"
	err = args.Validate()
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidArgs)
	assert.ErrorContains(t, err, "SAName provided")

	args.SAName = ""
	args.SANamespace = "serviceAccountNamespace"
	err = args.Validate()
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidArgs)
	assert.ErrorContains(t, err, "SANamespace provided")

	args.SAName = "serviceAccount"
	err = args.Validate()
	assert.NoError(t, err)
}

func TestNewIterator(t *testing.T) {
	args := Args{
		// no Client or else the Equal check will recurse infinitely
		Namespace:    "namespace",
		SnapshotName: "snapshot",
	}

	l := newIterator(args)
	assert.NotNil(t, l)

	assert.Equal(t, l, l.h)

	args.TokenExpirySecs = DefaultTokenExpirySeconds // set for comparison
	assert.Equal(t, args, l.Args)
}

func TestRun(t *testing.T) {
	testErr := errors.New("test-error")

	t.Run("get-changed-blocks-no-csi-driver-no-sa", func(t *testing.T) {
		th := newTestHarness()
		th.RetGetCSIDriverFromPrimarySnapshot = th.CSIDriver
		th.RetGetSnapshotMetadataServiceCRService = th.FakeCR()
		th.RetGetGRPCClient = th.GRPCSnapshotMetadataClient(t)
		th.RetCreateSecurityToken = "security-token"
		th.RetGetDefaultSAName = th.SAName
		th.RetGetDefaultSANamespace = th.SANamespace

		vs, vsc := th.FakeVS()
		th.RetGetVolumeSnapshot = vs
		th.RetGetVolumeSnapshotContent = vsc

		iter := th.NewTestIterator()
		iter.recordNum = 100
		iter.SAName = ""
		assert.NotEmpty(t, iter.PrevSnapshotName) // changed block flow

		err := iter.run(context.Background())
		assert.NoError(t, err)

		// check data passed through the helpers
		assert.True(t, th.CalledGetDefaultServiceAccount)
		assert.True(t, th.CalledGetCSIDriverFromPrimarySnapshot)
		assert.Equal(t, th.CSIDriver, th.InGetSnapshotMetadataServiceCRCSIDriver)
		assert.Equal(t, th.SAName, th.InCreateSecurityTokenSAName)
		assert.Equal(t, th.Audience, th.InCreateSecurityTokenAudience)
		assert.Equal(t, th.CACert, th.InGetGRPCClientCA)
		assert.Equal(t, th.Address, th.InGetGRPCClientURL)
		assert.Equal(t, th.RetGetGRPCClient, th.InGetChangedBlocksClient)
		assert.Equal(t, th.RetCreateSecurityToken, th.InGetChangedBlocksToken)

		// Done called
		assert.Equal(t, iter.recordNum, th.InSnapshotMetadataIteratorDoneNR)

		// the call context is canceled
		assert.ErrorIs(t, th.InCallContext.Err(), context.Canceled)
	})

	t.Run("get-changed-blocks-base-snapshot-id", func(t *testing.T) {
		th := newTestHarness()
		th.RetGetCSIDriverFromPrimarySnapshot = th.CSIDriver
		th.RetGetSnapshotMetadataServiceCRService = th.FakeCR()
		th.RetGetGRPCClient = th.GRPCSnapshotMetadataClient(t)
		th.RetCreateSecurityToken = "security-token"
		th.RetGetDefaultSAName = th.SAName
		th.RetGetDefaultSANamespace = th.SANamespace

		iter := th.NewTestIterator()
		iter.recordNum = 100
		iter.SAName = ""
		iter.PrevSnapshotName = ""
		iter.PrevSnapshotID = th.PrevSnapshotHandle
		assert.NotEmpty(t, iter.PrevSnapshotID) // changed block flow

		err := iter.run(context.Background())
		assert.NoError(t, err)

		// check data passed through the helpers
		assert.True(t, th.CalledGetDefaultServiceAccount)
		assert.True(t, th.CalledGetCSIDriverFromPrimarySnapshot)
		assert.Equal(t, th.CSIDriver, th.InGetSnapshotMetadataServiceCRCSIDriver)
		assert.Equal(t, th.SAName, th.InCreateSecurityTokenSAName)
		assert.Equal(t, th.Audience, th.InCreateSecurityTokenAudience)
		assert.Equal(t, th.CACert, th.InGetGRPCClientCA)
		assert.Equal(t, th.Address, th.InGetGRPCClientURL)
		assert.Equal(t, th.RetGetGRPCClient, th.InGetChangedBlocksClient)
		assert.Equal(t, th.RetCreateSecurityToken, th.InGetChangedBlocksToken)

		// Done called
		assert.Equal(t, iter.recordNum, th.InSnapshotMetadataIteratorDoneNR)

		// the call context is canceled
		assert.ErrorIs(t, th.InCallContext.Err(), context.Canceled)
	})

	t.Run("get-allocated-blocks-with-csi-driver", func(t *testing.T) {
		th := newTestHarness()
		th.RetGetSnapshotMetadataServiceCRService = th.FakeCR()
		th.RetGetGRPCClient = th.GRPCSnapshotMetadataClient(t)
		th.RetCreateSecurityToken = "security-token"

		iter := th.NewTestIterator()
		iter.recordNum = 100
		iter.PrevSnapshotName = ""
		iter.CSIDriver = th.CSIDriver

		err := iter.run(context.Background())
		assert.NoError(t, err)

		// check data passed through the helpers
		assert.False(t, th.CalledGetCSIDriverFromPrimarySnapshot)
		assert.Equal(t, th.CSIDriver, th.InGetSnapshotMetadataServiceCRCSIDriver)
		assert.Equal(t, th.Audience, th.InCreateSecurityTokenAudience)
		assert.Equal(t, th.CACert, th.InGetGRPCClientCA)
		assert.Equal(t, th.Address, th.InGetGRPCClientURL)
		assert.Equal(t, th.RetGetGRPCClient, th.InGetAllocatedBlocksClient)
		assert.Equal(t, th.RetCreateSecurityToken, th.InGetAllocatedBlocksToken)

		// Done called
		assert.Equal(t, iter.recordNum, th.InSnapshotMetadataIteratorDoneNR)

		// the call context is canceled
		assert.ErrorIs(t, th.InCallContext.Err(), context.Canceled)
	})

	t.Run("err-get-csi-driver-from-primary-snapshot", func(t *testing.T) {
		th := newTestHarness()
		th.RetGetCSIDriverFromPrimarySnapshotErr = testErr

		iter := th.NewTestIterator()
		assert.NotEmpty(t, iter.PrevSnapshotName) // changed block flow

		err := iter.run(context.Background())
		assert.ErrorIs(t, err, testErr)

		assert.True(t, th.CalledGetCSIDriverFromPrimarySnapshot)
	})

	t.Run("err-get-cr", func(t *testing.T) {
		th := newTestHarness()
		th.RetGetSnapshotMetadataServiceCRErr = testErr

		iter := th.NewTestIterator()
		iter.recordNum = 100
		iter.PrevSnapshotName = ""
		iter.CSIDriver = th.CSIDriver

		err := iter.run(context.Background())
		assert.ErrorIs(t, err, testErr)

		assert.Equal(t, th.CSIDriver, th.InGetSnapshotMetadataServiceCRCSIDriver)
	})

	t.Run("err-create-security-token", func(t *testing.T) {
		th := newTestHarness()
		th.RetGetSnapshotMetadataServiceCRService = th.FakeCR()
		th.RetGetGRPCClient = th.GRPCSnapshotMetadataClient(t)
		th.RetCreateSecurityTokenErr = testErr

		iter := th.NewTestIterator()
		iter.recordNum = 100
		iter.PrevSnapshotName = ""
		iter.CSIDriver = th.CSIDriver

		err := iter.run(context.Background())
		assert.ErrorIs(t, err, testErr)

		assert.Equal(t, th.Audience, th.InCreateSecurityTokenAudience)
	})

	t.Run("err-get-grpc-client", func(t *testing.T) {
		th := newTestHarness()
		th.RetGetSnapshotMetadataServiceCRService = th.FakeCR()
		th.RetGetGRPCClientErr = testErr
		th.RetCreateSecurityToken = "security-token"

		iter := th.NewTestIterator()
		iter.recordNum = 100
		iter.PrevSnapshotName = ""
		iter.CSIDriver = th.CSIDriver

		err := iter.run(context.Background())
		assert.ErrorIs(t, err, testErr)

		assert.Equal(t, th.CACert, th.InGetGRPCClientCA)
		assert.Equal(t, th.Address, th.InGetGRPCClientURL)
	})

	t.Run("err-get-previous-snapshot-id", func(t *testing.T) {
		th := newTestHarness()
		th.RetGetSnapshotMetadataServiceCRService = th.FakeCR()
		th.RetGetGRPCClient = th.GRPCSnapshotMetadataClient(t)
		th.RetCreateSecurityToken = "security-token"
		th.RetGetVolumeSnapshotErr = testErr

		iter := th.NewTestIterator()
		iter.recordNum = 100
		iter.CSIDriver = th.CSIDriver

		assert.Equal(t, th.PrevSnapshotName, iter.PrevSnapshotName)
		assert.Empty(t, iter.PrevSnapshotID)

		err := iter.run(context.Background())
		assert.ErrorIs(t, err, testErr)

		// check data passed through the helpers
		assert.Equal(t, th.PrevSnapshotName, th.InGetVolumeSnapshotName)

		// Done not called
		assert.Zero(t, th.InSnapshotMetadataIteratorDoneNR)

		// the call context is canceled
		assert.ErrorIs(t, th.InCallContext.Err(), context.Canceled)
	})

	t.Run("err-get-changed-blocks", func(t *testing.T) {
		th := newTestHarness()
		th.RetGetSnapshotMetadataServiceCRService = th.FakeCR()
		th.RetGetGRPCClient = th.GRPCSnapshotMetadataClient(t)
		th.RetCreateSecurityToken = "security-token"
		th.RetGetChangedBlocksErr = testErr

		vs, vsc := th.FakeVS()
		th.RetGetVolumeSnapshot = vs
		th.RetGetVolumeSnapshotContent = vsc

		iter := th.NewTestIterator()
		iter.recordNum = 100
		iter.CSIDriver = th.CSIDriver

		assert.Equal(t, th.PrevSnapshotName, iter.PrevSnapshotName)
		assert.Empty(t, iter.PrevSnapshotID)

		err := iter.run(context.Background())
		assert.ErrorIs(t, err, testErr)

		// check data passed through the helpers
		assert.Equal(t, th.RetGetGRPCClient, th.InGetChangedBlocksClient)
		assert.Equal(t, th.RetCreateSecurityToken, th.InGetChangedBlocksToken)
		assert.Equal(t, th.PrevSnapshotName, th.InGetVolumeSnapshotName)

		// Done not called
		assert.Zero(t, th.InSnapshotMetadataIteratorDoneNR)

		// the call context is canceled
		assert.ErrorIs(t, th.InCallContext.Err(), context.Canceled)
	})

	t.Run("err-get-allocated-blocks", func(t *testing.T) {
		th := newTestHarness()
		th.RetGetSnapshotMetadataServiceCRService = th.FakeCR()
		th.RetGetGRPCClient = th.GRPCSnapshotMetadataClient(t)
		th.RetCreateSecurityToken = "security-token"
		th.RetGetAllocatedBlocksClientErr = testErr

		iter := th.NewTestIterator()
		iter.recordNum = 100
		iter.CSIDriver = th.CSIDriver
		iter.PrevSnapshotName = ""

		err := iter.run(context.Background())
		assert.ErrorIs(t, err, testErr)

		// check data passed through the helpers
		assert.Equal(t, th.RetGetGRPCClient, th.InGetAllocatedBlocksClient)
		assert.Equal(t, th.RetCreateSecurityToken, th.InGetAllocatedBlocksToken)

		// Done not called
		assert.Zero(t, th.InSnapshotMetadataIteratorDoneNR)

		// the call context is canceled
		assert.ErrorIs(t, th.InCallContext.Err(), context.Canceled)
	})
}

func TestGetDefaultServiceAccount(t *testing.T) {
	t.Run("self-subject-review-err", func(t *testing.T) {
		th := newTestHarness()
		args := th.Args()
		args.SAName = ""
		args.SANamespace = ""

		// invoke via GetSnapshotMetadata directly to cover that code path
		err := GetSnapshotMetadata(context.Background(), args)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "SelfSubjectReviews.Create")
	})

	t.Run("not-a-service-account", func(t *testing.T) {
		th := newTestHarness()
		iter := th.NewTestIterator()

		th.FakeKubeClient.PrependReactor("create", "selfsubjectreviews", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
			ssr := th.FakeAuthSelfSubjectReview()
			ssr.Status.UserInfo.Username += ":additionalfield"
			return true, ssr, nil
		})

		saNS, saName, err := iter.getDefaultServiceAccount(context.Background())
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidArgs)
		assert.Empty(t, saName)
		assert.Empty(t, saNS)
	})

	t.Run("success", func(t *testing.T) {
		th := newTestHarness()
		iter := th.NewTestIterator()

		th.FakeKubeClient.PrependReactor("create", "selfsubjectreviews", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
			ssr := th.FakeAuthSelfSubjectReview()
			return true, ssr, nil
		})

		saNS, saName, err := iter.getDefaultServiceAccount(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, th.SAName, saName)
		assert.Equal(t, th.SANamespace, saNS)
	})
}

func TestGetCSIDriverFromPrimarySnapshot(t *testing.T) {
	t.Run("snapshot-get-err", func(t *testing.T) {
		th := newTestHarness()
		args := th.Args()

		// Invoke via GetSnapshotMetadata directly to cover that
		// code path.
		err := GetSnapshotMetadata(context.Background(), args)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "VolumeSnapshots.Get")
	})

	t.Run("snapshot-has-nil-status", func(t *testing.T) {
		th := newTestHarness()
		iter := th.NewTestIterator()
		vs, _ := th.FakeVS()

		th.FakeSnapshotClient.PrependReactor("get", "volumesnapshots", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
			ga := action.(clientgotesting.GetAction)
			vs.Namespace = ga.GetNamespace()
			vs.Name = ga.GetName()
			vs.Status = nil
			return true, vs, nil
		})

		csiDriver, err := iter.getCSIDriverFromPrimarySnapshot(context.Background())
		assert.Error(t, err)
		assert.ErrorContains(t, err, "has no bound VolumeSnapshotContent")
		assert.Empty(t, csiDriver)
	})

	t.Run("snapshot-not-bound", func(t *testing.T) {
		th := newTestHarness()
		iter := th.NewTestIterator()
		vs, _ := th.FakeVS()

		th.FakeSnapshotClient.PrependReactor("get", "volumesnapshots", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
			ga := action.(clientgotesting.GetAction)
			vs.Namespace = ga.GetNamespace()
			vs.Name = ga.GetName()
			vs.Status.BoundVolumeSnapshotContentName = nil
			return true, vs, nil
		})

		csiDriver, err := iter.getCSIDriverFromPrimarySnapshot(context.Background())
		assert.Error(t, err)
		assert.ErrorContains(t, err, "has no bound VolumeSnapshotContent")
		assert.Empty(t, csiDriver)
	})

	t.Run("get-vsc-error", func(t *testing.T) {
		th := newTestHarness()
		iter := th.NewTestIterator()
		vs, _ := th.FakeVS()

		th.FakeSnapshotClient.PrependReactor("get", "volumesnapshots", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
			ga := action.(clientgotesting.GetAction)
			vs.Namespace = ga.GetNamespace()
			vs.Name = ga.GetName()
			return true, vs, nil
		})

		csiDriver, err := iter.getCSIDriverFromPrimarySnapshot(context.Background())
		assert.Error(t, err)
		assert.ErrorContains(t, err, "VolumeSnapshotContents.Get")
		assert.Empty(t, csiDriver)
	})

	t.Run("success", func(t *testing.T) {
		th := newTestHarness()
		iter := th.NewTestIterator()
		vs, vsc := th.FakeVS()

		th.FakeSnapshotClient.PrependReactor("get", "volumesnapshots", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
			ga := action.(clientgotesting.GetAction)
			vs.Namespace = ga.GetNamespace()
			vs.Name = ga.GetName()
			return true, vs, nil
		})

		th.FakeSnapshotClient.PrependReactor("get", "volumesnapshotcontents", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
			ga := action.(clientgotesting.GetAction)
			vsc.Name = ga.GetName()
			return true, vsc, nil
		})

		csiDriver, err := iter.getCSIDriverFromPrimarySnapshot(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, th.CSIDriver, csiDriver)
	})
}

func TestGetSnapshotMetadataServiceCR(t *testing.T) {
	t.Run("get-error", func(t *testing.T) {
		th := newTestHarness()
		iter := th.NewTestIterator()

		sms, err := iter.getSnapshotMetadataServiceCR(context.Background(), th.CSIDriver)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "SnapshotMetadataServices.Get")
		assert.Nil(t, sms)
	})

	t.Run("success", func(t *testing.T) {
		th := newTestHarness()
		iter := th.NewTestIterator()
		fakeCR := th.FakeCR()

		th.FakeSmsCRClient.PrependReactor("get", "snapshotmetadataservices", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
			ga := action.(clientgotesting.GetAction)
			if ga.GetName() != th.CSIDriver {
				return false, nil, nil
			}
			return true, fakeCR, nil
		})

		sms, err := iter.getSnapshotMetadataServiceCR(context.Background(), th.CSIDriver)
		assert.NoError(t, err)
		assert.Equal(t, fakeCR, sms)
	})
}

func TestCreateSecurityToken(t *testing.T) {
	t.Run("create-err", func(t *testing.T) {
		th := newTestHarness()
		iter := th.NewTestIterator()

		securityToken, err := iter.createSecurityToken(context.Background(), th.SANamespace, th.SAName, th.Audience)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "ServiceAccounts.CreateToken")
		assert.Empty(t, securityToken)
	})

	t.Run("success", func(t *testing.T) {
		th := newTestHarness()
		iter := th.NewTestIterator()

		th.FakeKubeClient.PrependReactor("create", "serviceaccounts", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
			return true, th.FakeTokenRequest(), nil
		})

		securityToken, err := iter.createSecurityToken(context.Background(), th.SANamespace, th.SAName, th.Audience)
		assert.NoError(t, err)
		assert.Equal(t, th.SecurityToken, securityToken)
	})
}

func TestGetGRPCClient(t *testing.T) {
	rth := runtime.NewTestHarness().WithTestTLSFiles(t)
	defer rth.RemoveTestTLSFiles(t)

	rthArgs := rth.RuntimeArgs()
	assert.NotEmpty(t, rthArgs.TLSCertFile)
	caCert, err := os.ReadFile(rthArgs.TLSCertFile)
	assert.NoError(t, err)
	assert.NotNil(t, caCert)

	t.Run("invalid-ca-cert", func(t *testing.T) {
		th := newTestHarness()
		iter := th.NewTestIterator()

		client, err := iter.getGRPCClient([]byte{}, "")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCACert)
		assert.Nil(t, client)
	})

	t.Run("grpc-new-tls-err", func(t *testing.T) {
		th := newTestHarness()
		iter := th.NewTestIterator()

		client, err := iter.getGRPCClient(caCert, "\n") // invalid url
		assert.Error(t, err)
		assert.ErrorContains(t, err, "grpc.NewClient")
		assert.Nil(t, client)
	})

	t.Run("success", func(t *testing.T) {
		th := newTestHarness()
		iter := th.NewTestIterator()

		client, err := iter.getGRPCClient(caCert, "")
		assert.NoError(t, err)
		assert.NotNil(t, client)
	})
}

func TestGetAllocatedBlocks(t *testing.T) {
	errTest := errors.New("test-error")

	responses := []*api.GetMetadataAllocatedResponse{
		{
			BlockMetadataType:   api.BlockMetadataType_FIXED_LENGTH,
			VolumeCapacityBytes: 100000000,
			BlockMetadata: []*api.BlockMetadata{
				{
					ByteOffset: 1000,
					SizeBytes:  1000,
				},
				{
					ByteOffset: 2000,
					SizeBytes:  3000, // deliberately different
				},
			},
		},
		{
			BlockMetadataType:   api.BlockMetadataType_VARIABLE_LENGTH, // deliberately
			VolumeCapacityBytes: 100000001,                             // different
			BlockMetadata: []*api.BlockMetadata{
				{
					ByteOffset: 5000,
					SizeBytes:  1000,
				},
			},
		},
	}

	// helper to check the emitted data
	checkIterRecs := func(t *testing.T, th *testHarness, iter *iterator, responses []*api.GetMetadataAllocatedResponse) {
		assert.Equal(t, len(responses), iter.recordNum)
		assert.Equal(t, len(responses), th.InSnapshotMetadataIteratorRecordNum)
		// cannot directly compare BlockMetadata because of
		// internal pragma DoNotCopy
		iterRecs := th.InSnapshotMetadataIteratorRecordMeta
		for i, resp := range responses {
			assert.Equal(t, resp.BlockMetadataType, iterRecs[i].BlockMetadataType)
			assert.Equal(t, resp.VolumeCapacityBytes, iterRecs[i].VolumeCapacityBytes)
			assert.Len(t, iterRecs[i].BlockMetadata, len(resp.BlockMetadata))
			for j, bm := range resp.BlockMetadata {
				assert.Equal(t, bm.ByteOffset, iterRecs[i].BlockMetadata[j].ByteOffset)
				assert.Equal(t, bm.SizeBytes, iterRecs[i].BlockMetadata[j].SizeBytes)
			}
		}
	}

	t.Run("call-failed", func(t *testing.T) {
		th := newTestHarness()
		th.StartingOffset = 19990
		th.MaxResults = 32
		iter := th.NewTestIterator()

		mockController := gomock.NewController(t)
		mockClient := k8sclientmocks.NewMockSnapshotMetadataClient(mockController)
		defer mockController.Finish()

		expReq := th.FakeGetMetadataAllocatedRequest()
		mockClient.EXPECT().GetMetadataAllocated(gomock.Any(), expReq).Return(nil, errTest)

		err := iter.getAllocatedBlocks(context.Background(), mockClient, th.SecurityToken)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errTest)
	})

	t.Run("stream-rec-rec-EOF", func(t *testing.T) {
		th := newTestHarness()
		th.RetSnapshotMetadataIteratorRecord = nil
		iter := th.NewTestIterator()

		mockController := gomock.NewController(t)
		mockClient := k8sclientmocks.NewMockSnapshotMetadataClient(mockController)
		defer mockController.Finish()

		mockStream := k8sclientmocks.NewMockSnapshotMetadata_GetMetadataAllocatedClient(mockController)

		for _, resp := range responses {
			mockStream.EXPECT().Recv().Return(resp, nil)
		}
		mockStream.EXPECT().Recv().Return(nil, io.EOF)

		expReq := th.FakeGetMetadataAllocatedRequest()
		mockClient.EXPECT().GetMetadataAllocated(gomock.Any(), expReq).Return(mockStream, nil)

		err := iter.getAllocatedBlocks(context.Background(), mockClient, th.SecurityToken)
		assert.NoError(t, err)

		checkIterRecs(t, th, iter, responses)
	})

	t.Run("stream-rec-rec-Err", func(t *testing.T) {
		th := newTestHarness()
		th.RetSnapshotMetadataIteratorRecord = nil
		iter := th.NewTestIterator()

		mockController := gomock.NewController(t)
		mockClient := k8sclientmocks.NewMockSnapshotMetadataClient(mockController)
		defer mockController.Finish()

		mockStream := k8sclientmocks.NewMockSnapshotMetadata_GetMetadataAllocatedClient(mockController)

		for _, resp := range responses {
			mockStream.EXPECT().Recv().Return(resp, nil)
		}
		mockStream.EXPECT().Recv().Return(nil, errTest)

		expReq := th.FakeGetMetadataAllocatedRequest()
		mockClient.EXPECT().GetMetadataAllocated(gomock.Any(), expReq).Return(mockStream, nil)

		err := iter.getAllocatedBlocks(context.Background(), mockClient, th.SecurityToken)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errTest)

		checkIterRecs(t, th, iter, responses)
	})

	t.Run("stream-rec-ABORT", func(t *testing.T) {
		th := newTestHarness()
		th.RetSnapshotMetadataIteratorRecord = nil
		iter := th.NewTestIterator()

		mockController := gomock.NewController(t)
		mockClient := k8sclientmocks.NewMockSnapshotMetadataClient(mockController)
		defer mockController.Finish()

		mockStream := k8sclientmocks.NewMockSnapshotMetadata_GetMetadataAllocatedClient(mockController)

		mockStream.EXPECT().Recv().Return(responses[0], nil) // one record only
		th.RetSnapshotMetadataIteratorRecord = ErrCancelled  // then abort

		expReq := th.FakeGetMetadataAllocatedRequest()
		mockClient.EXPECT().GetMetadataAllocated(gomock.Any(), expReq).Return(mockStream, nil)

		err := iter.getAllocatedBlocks(context.Background(), mockClient, th.SecurityToken)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCancelled)

		checkIterRecs(t, th, iter, responses[:1])
	})
}

func TestGetChangedBlocks(t *testing.T) {
	errTest := errors.New("test-error")

	responses := []*api.GetMetadataDeltaResponse{
		{
			BlockMetadataType:   api.BlockMetadataType_FIXED_LENGTH,
			VolumeCapacityBytes: 100000000,
			BlockMetadata: []*api.BlockMetadata{
				{
					ByteOffset: 1000,
					SizeBytes:  1000,
				},
				{
					ByteOffset: 2000,
					SizeBytes:  3000, // deliberately different
				},
			},
		},
		{
			BlockMetadataType:   api.BlockMetadataType_VARIABLE_LENGTH, // deliberately
			VolumeCapacityBytes: 100000001,                             // different
			BlockMetadata: []*api.BlockMetadata{
				{
					ByteOffset: 5000,
					SizeBytes:  1000,
				},
			},
		},
	}

	// helper to check the emitted data
	checkIterRecs := func(t *testing.T, th *testHarness, iter *iterator, responses []*api.GetMetadataDeltaResponse) {
		assert.Equal(t, len(responses), iter.recordNum)
		assert.Equal(t, len(responses), th.InSnapshotMetadataIteratorRecordNum)
		// cannot directly compare BlockMetadata because of
		// internal pragma DoNotCopy
		iterRecs := th.InSnapshotMetadataIteratorRecordMeta
		for i, resp := range responses {
			assert.Equal(t, resp.BlockMetadataType, iterRecs[i].BlockMetadataType)
			assert.Equal(t, resp.VolumeCapacityBytes, iterRecs[i].VolumeCapacityBytes)
			assert.Len(t, iterRecs[i].BlockMetadata, len(resp.BlockMetadata))
			for j, bm := range resp.BlockMetadata {
				assert.Equal(t, bm.ByteOffset, iterRecs[i].BlockMetadata[j].ByteOffset)
				assert.Equal(t, bm.SizeBytes, iterRecs[i].BlockMetadata[j].SizeBytes)
			}
		}
	}

	t.Run("call-failed", func(t *testing.T) {
		th := newTestHarness()
		th.StartingOffset = 19990
		th.MaxResults = 32
		iter := th.NewTestIterator()
		iter.PrevSnapshotID = th.PrevSnapshotHandle
		iter.PrevSnapshotName = ""

		assert.NotEmpty(t, iter.PrevSnapshotID)

		mockController := gomock.NewController(t)
		mockClient := k8sclientmocks.NewMockSnapshotMetadataClient(mockController)
		defer mockController.Finish()

		expReq := th.FakeGetMetadataDeltaRequest()
		mockClient.EXPECT().GetMetadataDelta(gomock.Any(), expReq).Return(nil, errTest)

		err := iter.getChangedBlocks(context.Background(), mockClient, th.SecurityToken)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errTest)
	})

	t.Run("stream-rec-rec-EOF", func(t *testing.T) {
		th := newTestHarness()
		th.RetSnapshotMetadataIteratorRecord = nil
		iter := th.NewTestIterator()
		iter.PrevSnapshotID = th.PrevSnapshotHandle
		iter.PrevSnapshotName = ""

		assert.NotEmpty(t, iter.PrevSnapshotID)

		mockController := gomock.NewController(t)
		mockClient := k8sclientmocks.NewMockSnapshotMetadataClient(mockController)
		defer mockController.Finish()

		mockStream := k8sclientmocks.NewMockSnapshotMetadata_GetMetadataDeltaClient(mockController)

		for _, resp := range responses {
			mockStream.EXPECT().Recv().Return(resp, nil)
		}
		mockStream.EXPECT().Recv().Return(nil, io.EOF)

		expReq := th.FakeGetMetadataDeltaRequest()
		mockClient.EXPECT().GetMetadataDelta(gomock.Any(), expReq).Return(mockStream, nil)

		err := iter.getChangedBlocks(context.Background(), mockClient, th.SecurityToken)
		assert.NoError(t, err)

		checkIterRecs(t, th, iter, responses)
	})

	t.Run("stream-rec-rec-Err", func(t *testing.T) {
		th := newTestHarness()
		th.RetSnapshotMetadataIteratorRecord = nil
		iter := th.NewTestIterator()
		iter.PrevSnapshotID = th.PrevSnapshotHandle
		iter.PrevSnapshotName = ""

		assert.NotEmpty(t, iter.PrevSnapshotID)

		mockController := gomock.NewController(t)
		mockClient := k8sclientmocks.NewMockSnapshotMetadataClient(mockController)
		defer mockController.Finish()

		mockStream := k8sclientmocks.NewMockSnapshotMetadata_GetMetadataDeltaClient(mockController)

		for _, resp := range responses {
			mockStream.EXPECT().Recv().Return(resp, nil)
		}
		mockStream.EXPECT().Recv().Return(nil, errTest)

		expReq := th.FakeGetMetadataDeltaRequest()
		mockClient.EXPECT().GetMetadataDelta(gomock.Any(), expReq).Return(mockStream, nil)

		err := iter.getChangedBlocks(context.Background(), mockClient, th.SecurityToken)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errTest)

		checkIterRecs(t, th, iter, responses)
	})

	t.Run("stream-rec-ABORT", func(t *testing.T) {
		th := newTestHarness()
		th.RetSnapshotMetadataIteratorRecord = nil
		iter := th.NewTestIterator()
		iter.PrevSnapshotID = th.PrevSnapshotHandle
		iter.PrevSnapshotName = ""

		assert.NotEmpty(t, iter.PrevSnapshotID)

		mockController := gomock.NewController(t)
		mockClient := k8sclientmocks.NewMockSnapshotMetadataClient(mockController)
		defer mockController.Finish()

		mockStream := k8sclientmocks.NewMockSnapshotMetadata_GetMetadataDeltaClient(mockController)

		mockStream.EXPECT().Recv().Return(responses[0], nil) // one record only
		th.RetSnapshotMetadataIteratorRecord = ErrCancelled  // then abort

		expReq := th.FakeGetMetadataDeltaRequest()
		mockClient.EXPECT().GetMetadataDelta(gomock.Any(), expReq).Return(mockStream, nil)

		err := iter.getChangedBlocks(context.Background(), mockClient, th.SecurityToken)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCancelled)

		checkIterRecs(t, th, iter, responses[:1])
	})
}

func TestGetPrevSnapshotID(t *testing.T) {
	errTest := errors.New("test-error")

	t.Run("success", func(t *testing.T) {
		th := newTestHarness()
		vs, vsc := th.FakeVS()
		th.RetGetVolumeSnapshot = vs
		th.RetGetVolumeSnapshotContent = vsc

		iter := th.NewTestIterator()
		iter.PrevSnapshotID = ""
		iter.PrevSnapshotName = vs.Name

		snapID, err := iter.getPrevSnapshotID(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, *vsc.Status.SnapshotHandle, snapID)
	})

	t.Run("get-vsc-error", func(t *testing.T) {
		th := newTestHarness()
		vs, _ := th.FakeVS()
		th.RetGetVolumeSnapshot = vs
		th.RetGetVolumeSnapshotContentErr = errTest

		iter := th.NewTestIterator()
		iter.PrevSnapshotID = ""
		iter.PrevSnapshotName = vs.Name

		snapID, err := iter.getPrevSnapshotID(context.Background())
		assert.ErrorIs(t, err, errTest)
		assert.Empty(t, snapID)
	})

	t.Run("get-vs-error", func(t *testing.T) {
		th := newTestHarness()
		vs, _ := th.FakeVS()
		th.RetGetVolumeSnapshotErr = errTest

		iter := th.NewTestIterator()
		iter.PrevSnapshotID = ""
		iter.PrevSnapshotName = vs.Name

		snapID, err := iter.getPrevSnapshotID(context.Background())
		assert.ErrorIs(t, err, errTest)
		assert.Empty(t, snapID)
	})

	t.Run("getVolumeSnapshot", func(t *testing.T) {
		t.Run("get-error", func(t *testing.T) {
			th := newTestHarness()
			vs, _ := th.FakeVS()
			th.FakeSnapshotClient.PrependReactor("get", "volumesnapshots", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				return true, nil, errTest
			})

			iter := th.NewTestIterator()
			ret, err := iter.getVolumeSnapshot(context.Background(), vs.Namespace, vs.Name)
			assert.ErrorIs(t, err, errTest)
			assert.Nil(t, ret)
		})

		t.Run("ready-to-use-false", func(t *testing.T) {
			th := newTestHarness()
			vs, _ := th.FakeVS()
			vs.Status.ReadyToUse = ptr.To(false)
			th.FakeSnapshotClient.PrependReactor("get", "volumesnapshots", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				if ga.GetNamespace() == vs.Namespace && ga.GetName() == vs.Name {
					return true, vs, nil
				}
				return true, nil, errTest
			})

			iter := th.NewTestIterator()
			ret, err := iter.getVolumeSnapshot(context.Background(), vs.Namespace, vs.Name)
			assert.ErrorContains(t, err, "is not yet ready")
			assert.Nil(t, ret)
		})

		t.Run("not-bound", func(t *testing.T) {
			th := newTestHarness()
			vs, _ := th.FakeVS()
			vs.Status.BoundVolumeSnapshotContentName = nil
			th.FakeSnapshotClient.PrependReactor("get", "volumesnapshots", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				if ga.GetNamespace() == vs.Namespace && ga.GetName() == vs.Name {
					return true, vs, nil
				}
				return true, nil, errTest
			})

			iter := th.NewTestIterator()
			ret, err := iter.getVolumeSnapshot(context.Background(), vs.Namespace, vs.Name)
			assert.ErrorContains(t, err, "boundVolumeSnapshotContentName not set")
			assert.Nil(t, ret)
		})

		t.Run("success", func(t *testing.T) {
			t.Run("ready-to-use-nil", func(t *testing.T) {
				th := newTestHarness()
				vs, _ := th.FakeVS()
				vs.Status.ReadyToUse = nil
				th.FakeSnapshotClient.PrependReactor("get", "volumesnapshots", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
					ga := action.(clientgotesting.GetAction)
					if ga.GetNamespace() == vs.Namespace && ga.GetName() == vs.Name {
						return true, vs, nil
					}
					return true, nil, errTest
				})

				iter := th.NewTestIterator()
				ret, err := iter.getVolumeSnapshot(context.Background(), vs.Namespace, vs.Name)
				assert.NoError(t, err)
				assert.Equal(t, vs, ret)
			})

			t.Run("ready-to-use-true", func(t *testing.T) {
				th := newTestHarness()
				vs, _ := th.FakeVS()
				vs.Status.ReadyToUse = ptr.To(true)
				th.FakeSnapshotClient.PrependReactor("get", "volumesnapshots", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
					ga := action.(clientgotesting.GetAction)
					if ga.GetNamespace() == vs.Namespace && ga.GetName() == vs.Name {
						return true, vs, nil
					}
					return true, nil, errTest
				})

				iter := th.NewTestIterator()
				ret, err := iter.getVolumeSnapshot(context.Background(), vs.Namespace, vs.Name)
				assert.NoError(t, err)
				assert.Equal(t, vs, ret)
			})
		})
	})

	t.Run("getVolumeSnapshotContent", func(t *testing.T) {
		t.Run("get-error", func(t *testing.T) {
			th := newTestHarness()
			vs, _ := th.FakeVS()
			th.FakeSnapshotClient.PrependReactor("get", "volumesnapshotcontents", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				return true, nil, errTest
			})

			iter := th.NewTestIterator()
			ret, err := iter.getVolumeSnapshotContent(context.Background(), vs)
			assert.ErrorIs(t, err, errTest)
			assert.Nil(t, ret)
		})

		t.Run("invalid-ref-uid", func(t *testing.T) {
			th := newTestHarness()
			vs, vsc := th.FakeVS()
			vsc.Spec.VolumeSnapshotRef = v1.ObjectReference{}
			vsc.Spec.VolumeSnapshotRef.UID = vs.UID + "foo"
			th.FakeSnapshotClient.PrependReactor("get", "volumesnapshotcontents", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				if ga.GetName() == vsc.Name {
					return true, vsc, nil
				}
				return true, nil, errTest
			})

			iter := th.NewTestIterator()
			ret, err := iter.getVolumeSnapshotContent(context.Background(), vs)
			assert.ErrorContains(t, err, "volumeSnapshotRef.UID does not identify VolumeSnapshot")
			assert.Nil(t, ret)
		})

		t.Run("invalid-ref-ns", func(t *testing.T) {
			th := newTestHarness()
			vs, vsc := th.FakeVS()
			vsc.Spec.VolumeSnapshotRef = v1.ObjectReference{}
			vsc.Spec.VolumeSnapshotRef.Namespace = vs.Namespace + "foo"
			vsc.Spec.VolumeSnapshotRef.Name = vs.Name
			th.FakeSnapshotClient.PrependReactor("get", "volumesnapshotcontents", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				if ga.GetName() == vsc.Name {
					return true, vsc, nil
				}
				return true, nil, errTest
			})

			iter := th.NewTestIterator()
			ret, err := iter.getVolumeSnapshotContent(context.Background(), vs)
			assert.ErrorContains(t, err, "volumeSnapshotRef does not identify VolumeSnapshot")
			assert.Nil(t, ret)
		})

		t.Run("invalid-ref-name", func(t *testing.T) {
			th := newTestHarness()
			vs, vsc := th.FakeVS()
			vsc.Spec.VolumeSnapshotRef = v1.ObjectReference{}
			vsc.Spec.VolumeSnapshotRef.Namespace = vs.Namespace
			vsc.Spec.VolumeSnapshotRef.Name = vs.Name + "foo"
			th.FakeSnapshotClient.PrependReactor("get", "volumesnapshotcontents", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				if ga.GetName() == vsc.Name {
					return true, vsc, nil
				}
				return true, nil, errTest
			})

			iter := th.NewTestIterator()
			ret, err := iter.getVolumeSnapshotContent(context.Background(), vs)
			assert.ErrorContains(t, err, "volumeSnapshotRef does not identify VolumeSnapshot")
			assert.Nil(t, ret)
		})

		t.Run("ready-to-use-false", func(t *testing.T) {
			th := newTestHarness()
			vs, vsc := th.FakeVS()
			vsc.Status.ReadyToUse = ptr.To(false)
			th.FakeSnapshotClient.PrependReactor("get", "volumesnapshotcontents", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				if ga.GetName() == vsc.Name {
					return true, vsc, nil
				}
				return true, nil, errTest
			})

			iter := th.NewTestIterator()
			ret, err := iter.getVolumeSnapshotContent(context.Background(), vs)
			assert.ErrorContains(t, err, "is not yet ready")
			assert.Nil(t, ret)
		})

		t.Run("snapshot-handle-not-set", func(t *testing.T) {
			th := newTestHarness()
			vs, vsc := th.FakeVS()
			vsc.Status.SnapshotHandle = nil
			th.FakeSnapshotClient.PrependReactor("get", "volumesnapshotcontents", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				ga := action.(clientgotesting.GetAction)
				if ga.GetName() == vsc.Name {
					return true, vsc, nil
				}
				return true, nil, errTest
			})

			iter := th.NewTestIterator()
			ret, err := iter.getVolumeSnapshotContent(context.Background(), vs)
			assert.ErrorContains(t, err, "snapshot handle not set")
			assert.Nil(t, ret)
		})

		t.Run("success", func(t *testing.T) {
			t.Run("ready-to-use-nil", func(t *testing.T) {
				th := newTestHarness()
				vs, vsc := th.FakeVS()
				vsc.Status.ReadyToUse = nil
				th.FakeSnapshotClient.PrependReactor("get", "volumesnapshotcontents", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
					ga := action.(clientgotesting.GetAction)
					if ga.GetName() == vsc.Name {
						return true, vsc, nil
					}
					return true, nil, errTest
				})

				iter := th.NewTestIterator()
				ret, err := iter.getVolumeSnapshotContent(context.Background(), vs)
				assert.NoError(t, err)
				assert.Equal(t, vsc, ret)
			})

			t.Run("ready-to-use-true", func(t *testing.T) {
				th := newTestHarness()
				vs, vsc := th.FakeVS()
				vsc.Status.ReadyToUse = ptr.To(true)
				th.FakeSnapshotClient.PrependReactor("get", "volumesnapshotcontents", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
					ga := action.(clientgotesting.GetAction)
					if ga.GetName() == vsc.Name {
						return true, vsc, nil
					}
					return true, nil, errTest
				})

				iter := th.NewTestIterator()
				ret, err := iter.getVolumeSnapshotContent(context.Background(), vs)
				assert.NoError(t, err)
				assert.Equal(t, vsc, ret)
			})
		})
	})
}
