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
	"net"
	"testing"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	fakesnapshot "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	authv1 "k8s.io/api/authentication/v1"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	smsCRv1alpha1 "github.com/kubernetes-csi/external-snapshot-metadata/client/apis/snapshotmetadataservice/v1alpha1"
	fakeSmsCR "github.com/kubernetes-csi/external-snapshot-metadata/client/clientset/versioned/fake"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
)

type testHarness struct {
	Address          string
	Audience         string
	CACert           []byte
	CSIDriver        string
	MaxResults       int32
	Namespace        string
	PrevSnapshotName string
	SecurityToken    string
	ServiceAccount   string
	SnapshotName     string
	StartingOffset   int64

	FakeKubeClient     *fake.Clientset
	FakeSnapshotClient *fakesnapshot.Clientset
	FakeSmsCRClient    *fakeSmsCR.Clientset

	// fake emitter
	InSnapshotMetadataIteratorRecordNum  int
	InSnapshotMetadataIteratorRecordMeta []IteratorMetadata
	RetSnapshotMetadataIteratorRecord    bool

	InSnapshotMetadataIteratorDoneNR int

	// fake helpers
	CalledGetDefaultServiceAccount bool
	RetGetDefaultServiceAccount    string
	RetGetDefaultServiceAccountErr error

	CalledGetCSIDriverFromPrimarySnapshot bool
	RetGetCSIDriverFromPrimarySnapshot    string
	RetGetCSIDriverFromPrimarySnapshotErr error

	InGetSnapshotMetadataServiceCRCSIDriver string
	RetGetSnapshotMetadataServiceCRService  *smsCRv1alpha1.SnapshotMetadataService
	RetGetSnapshotMetadataServiceCRErr      error

	InCreateSecurityTokenSA       string
	InCreateSecurityTokenAudience string
	RetCreateSecurityToken        string
	RetCreateSecurityTokenErr     error

	InGetGRPCClientCA   []byte
	InGetGRPCClientURL  string
	RetGetGRPCClient    api.SnapshotMetadataClient
	RetGetGRPCClientErr error

	InGetAllocatedBlocksClient     api.SnapshotMetadataClient
	InGetAllocatedBlocksToken      string
	RetGetAllocatedBlocksClientErr error

	InGetChangedBlocksClient api.SnapshotMetadataClient
	InGetChangedBlocksToken  string
	RetGetChangedBlocksErr   error

	InCallContext context.Context
}

func newTestHarness() *testHarness {
	return &testHarness{
		Namespace:        "namespace",
		SnapshotName:     "snapshotName",
		PrevSnapshotName: "prevSnapshotName",
		ServiceAccount:   "serviceAccount",
		CSIDriver:        "csiDriver",
		Audience:         "audience",
		Address:          "sidecar.csiDriver.k8s.local", // invalid
		CACert:           []byte{1, 2, 3},               // invalid
		SecurityToken:    "securityToken",

		FakeKubeClient:     fake.NewClientset(),
		FakeSnapshotClient: fakesnapshot.NewSimpleClientset(),
		FakeSmsCRClient:    fakeSmsCR.NewSimpleClientset(),
	}
}

func (th *testHarness) Clients() Clients {
	return Clients{
		KubeClient:     th.FakeKubeClient,
		SnapshotClient: th.FakeSnapshotClient,
		SmsCRClient:    th.FakeSmsCRClient,
	}
}

func (th *testHarness) NewTestIterator() *iterator {
	iter := newIterator(th.Args())
	iter.h = th // use the fake test helpers
	return iter
}

func (th *testHarness) Args() Args {
	return Args{
		Clients:          th.Clients(),
		Namespace:        th.Namespace,
		SnapshotName:     th.SnapshotName,
		PrevSnapshotName: th.PrevSnapshotName,
		ServiceAccount:   th.ServiceAccount,
		StartingOffset:   th.StartingOffset,
		MaxResults:       th.MaxResults,
		Emitter:          th,
	}
}

func (th *testHarness) FakeCR() *smsCRv1alpha1.SnapshotMetadataService {
	return &smsCRv1alpha1.SnapshotMetadataService{
		ObjectMeta: apimetav1.ObjectMeta{
			Name: th.CSIDriver,
		},
		Spec: smsCRv1alpha1.SnapshotMetadataServiceSpec{
			Audience: th.Audience,
			Address:  th.Address,
			CACert:   th.CACert,
		},
	}
}

func (th *testHarness) FakeVS() (*snapshotv1.VolumeSnapshot, *snapshotv1.VolumeSnapshotContent) {
	vscName := "vsc-" + th.SnapshotName
	vs := &snapshotv1.VolumeSnapshot{
		ObjectMeta: apimetav1.ObjectMeta{
			Namespace: th.Namespace,
			Name:      th.SnapshotName,
		},
		Status: &snapshotv1.VolumeSnapshotStatus{
			BoundVolumeSnapshotContentName: &vscName,
		},
	}
	vsc := &snapshotv1.VolumeSnapshotContent{
		ObjectMeta: apimetav1.ObjectMeta{
			Name: vscName,
		},
		Spec: snapshotv1.VolumeSnapshotContentSpec{
			Driver: th.CSIDriver,
		},
	}
	return vs, vsc
}

func (th *testHarness) FakeAuthSelfSubjectReview() *authv1.SelfSubjectReview {
	return &authv1.SelfSubjectReview{
		Status: authv1.SelfSubjectReviewStatus{
			UserInfo: authv1.UserInfo{
				Username: K8sServiceAccountUserNamePrefix + th.Namespace + ":" + th.ServiceAccount,
			},
		},
	}
}

func (th *testHarness) FakeTokenRequest() *authv1.TokenRequest {
	expirySecs := th.Args().TokenExpirySecs
	return &authv1.TokenRequest{
		Spec: authv1.TokenRequestSpec{
			Audiences:         []string{th.Audience},
			ExpirationSeconds: &expirySecs,
		},
		Status: authv1.TokenRequestStatus{
			Token: th.SecurityToken,
		},
	}
}

func (th *testHarness) FakeGetMetadataAllocatedRequest() *api.GetMetadataAllocatedRequest {
	args := th.Args()
	return &api.GetMetadataAllocatedRequest{
		SecurityToken:  th.SecurityToken,
		Namespace:      th.Namespace,
		SnapshotName:   th.SnapshotName,
		StartingOffset: args.StartingOffset,
		MaxResults:     args.MaxResults,
	}
}

func (th *testHarness) FakeGetMetadataDeltaRequest() *api.GetMetadataDeltaRequest {
	args := th.Args()
	return &api.GetMetadataDeltaRequest{
		SecurityToken:      th.SecurityToken,
		Namespace:          th.Namespace,
		TargetSnapshotName: th.SnapshotName,
		BaseSnapshotName:   th.PrevSnapshotName,
		StartingOffset:     args.StartingOffset,
		MaxResults:         args.MaxResults,
	}
}

func (th *testHarness) FakeConn(t *testing.T) *grpc.ClientConn {
	buffer := 1024 * 1024
	listener := bufconn.Listen(buffer)

	conn, err := grpc.NewClient("passthrough://bufconn",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}), grpc.WithTransportCredentials(insecure.NewCredentials()))

	assert.NoError(t, err)

	return conn
}

func (th *testHarness) GRPCSnapshotMetadataClient(t *testing.T) api.SnapshotMetadataClient {
	return api.NewSnapshotMetadataClient(th.FakeConn(t))
}

// fake emitter
func (th *testHarness) SnapshotMetadataIteratorRecord(recordNumber int, metadata IteratorMetadata) bool {
	th.InSnapshotMetadataIteratorRecordMeta = append(th.InSnapshotMetadataIteratorRecordMeta, metadata)
	th.InSnapshotMetadataIteratorRecordNum = recordNumber

	return th.RetSnapshotMetadataIteratorRecord
}

func (th *testHarness) SnapshotMetadataIteratorDone(numberRecords int) {
	th.InSnapshotMetadataIteratorDoneNR = numberRecords
}

// fake helpers
func (th *testHarness) getDefaultServiceAccount(ctx context.Context) (string, error) {
	th.CalledGetDefaultServiceAccount = true
	return th.RetGetDefaultServiceAccount, th.RetGetDefaultServiceAccountErr
}

func (th *testHarness) getCSIDriverFromPrimarySnapshot(ctx context.Context) (string, error) {
	th.CalledGetCSIDriverFromPrimarySnapshot = true
	return th.RetGetCSIDriverFromPrimarySnapshot, th.RetGetCSIDriverFromPrimarySnapshotErr
}

func (th *testHarness) getSnapshotMetadataServiceCR(ctx context.Context, csiDriver string) (*smsCRv1alpha1.SnapshotMetadataService, error) {
	th.InGetSnapshotMetadataServiceCRCSIDriver = csiDriver
	return th.RetGetSnapshotMetadataServiceCRService, th.RetGetSnapshotMetadataServiceCRErr
}

func (th *testHarness) createSecurityToken(ctx context.Context, serviceAccount, audience string) (string, error) {
	th.InCreateSecurityTokenSA = serviceAccount
	th.InCreateSecurityTokenAudience = audience
	return th.RetCreateSecurityToken, th.RetCreateSecurityTokenErr
}

func (th *testHarness) getGRPCClient(caCert []byte, URL string) (api.SnapshotMetadataClient, error) {
	th.InGetGRPCClientCA = caCert
	th.InGetGRPCClientURL = URL
	return th.RetGetGRPCClient, th.RetGetGRPCClientErr
}

func (th *testHarness) getAllocatedBlocks(ctx context.Context, grpcClient api.SnapshotMetadataClient, securityToken string) error {
	th.InCallContext = ctx
	th.InGetAllocatedBlocksClient = grpcClient
	th.InGetAllocatedBlocksToken = securityToken
	return th.RetGetAllocatedBlocksClientErr
}

func (th *testHarness) getChangedBlocks(ctx context.Context, grpcClient api.SnapshotMetadataClient, securityToken string) error {
	th.InCallContext = ctx
	th.InGetChangedBlocksClient = grpcClient
	th.InGetChangedBlocksToken = securityToken
	return th.RetGetChangedBlocksErr
}
