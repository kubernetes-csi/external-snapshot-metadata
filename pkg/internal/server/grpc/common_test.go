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

package grpc

import (
	"context"
	"net"
	"strings"
	"testing"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	fakesnapshot "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	snapshotutils "github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"
	authv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"

	smsv1alpha1 "github.com/kubernetes-csi/external-snapshot-metadata/client/apis/snapshotmetadataservice/v1alpha1"
	fakecbt "github.com/kubernetes-csi/external-snapshot-metadata/client/clientset/versioned/fake"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/internal/runtime"
)

type testHarness struct {
	*runtime.TestHarness

	Audience                string
	DriverName              string
	Namespace               string
	SecretName              string
	SecretNs                string
	SecurityToken           string
	VolumeSnapshotClassName string

	FakeKubeClient     *fake.Clientset
	FakeSnapshotClient *fakesnapshot.Clientset
	FakeCBTClient      *fakecbt.Clientset

	lenFakeKubeClientReactionChain     int
	lenFakeSnapshotClientReactionChain int
	lenFakeCBTClientReactionChain      int

	mockCSIDriverConn *grpc.ClientConn
	grpcServer        *grpc.Server
	listener          *bufconn.Listener
}

func newTestHarness() *testHarness {
	return &testHarness{
		Audience:                "audience",
		DriverName:              "driver",
		Namespace:               "namespace",
		SecretName:              "secret-name",
		SecretNs:                "secret-ns",
		SecurityToken:           "securityToken",
		VolumeSnapshotClassName: "csi-snapshot-class",
	}
}

func (th *testHarness) WithMockCSIDriver(t *testing.T) *testHarness {
	th.TestHarness = runtime.NewTestHarness().WithMockCSIDriver(t)
	th.mockCSIDriverConn = th.MockCSIDriverConn
	return th
}

func (th *testHarness) WithFakeClientAPIs() *testHarness {
	th.FakeKubeClient = th.makeFakeKubeClient()
	th.FakeSnapshotClient = th.makeFakeSnapshotClient()
	th.FakeCBTClient = th.makeFakeCBTClient()

	th.FakeCountReactors() // establish the baseline

	return th
}

func (th *testHarness) Runtime() *runtime.Runtime {
	return &runtime.Runtime{
		CBTClient:      th.FakeCBTClient,
		KubeClient:     th.FakeKubeClient,
		SnapshotClient: th.FakeSnapshotClient,
		DriverName:     th.DriverName,
		CSIConn:        th.mockCSIDriverConn,
	}
}

func (th *testHarness) ServerWithRuntime(t *testing.T, rt *runtime.Runtime) *Server {
	return &Server{
		config:       ServerConfig{Runtime: rt},
		healthServer: newHealthServer(),
	}
}

func (th *testHarness) StartGRPCServer(t *testing.T, rt *runtime.Runtime) *Server {
	buffer := 1024 * 1024

	th.listener = bufconn.Listen(buffer)
	th.grpcServer = grpc.NewServer()

	s := th.ServerWithRuntime(t, rt)
	s.grpcServer = th.grpcServer
	api.RegisterSnapshotMetadataServer(s.grpcServer, s)
	healthpb.RegisterHealthServer(s.grpcServer, s.healthServer)

	go func() {
		s.grpcServer.Serve(th.listener)
	}()

	return s
}

func (th *testHarness) StopGRPCServer(t *testing.T) {
	err := th.listener.Close()
	assert.NoError(t, err)

	th.grpcServer.Stop()
}

func (th *testHarness) GRPCSnapshotMetadataClient(t *testing.T) api.SnapshotMetadataClient {
	conn, err := grpc.NewClient("passthrough://bufconn",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return th.listener.Dial()
		}), grpc.WithTransportCredentials(insecure.NewCredentials()))

	assert.NoError(t, err)

	return api.NewSnapshotMetadataClient(conn)
}

func (th *testHarness) GRPCHealthClient(t *testing.T) healthpb.HealthClient {
	conn, err := grpc.NewClient("passthrough://bufconn",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return th.listener.Dial()
		}), grpc.WithTransportCredentials(insecure.NewCredentials()))

	assert.NoError(t, err)

	return healthpb.NewHealthClient(conn)
}

func (th *testHarness) makeFakeKubeClient() *fake.Clientset {
	fakeKubeClient := fake.NewSimpleClientset()
	fakeKubeClient.PrependReactor("create", "tokenreviews", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
		ca := action.(clientgotesting.CreateAction)
		trs := ca.GetObject().(*authv1.TokenReview)
		trs.Status.Authenticated = false
		if trs.Spec.Token == th.SecurityToken {
			trs.Status.Authenticated = true
			trs.Status.Audiences = []string{th.Audience, "other-" + th.Audience}
		}
		return true, trs, nil
	})
	fakeKubeClient.PrependReactor("create", "subjectaccessreviews", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
		ca := action.(clientgotesting.CreateAction)
		sar := ca.GetObject().(*v1.SubjectAccessReview)
		if sar.Spec.ResourceAttributes != nil && sar.Spec.ResourceAttributes.Namespace == th.Namespace {
			sar.Status.Allowed = true
		} else {
			sar.Status.Allowed = false
			sar.Status.Reason = "namespace mismatch"
		}
		return true, sar, nil
	})

	fakeKubeClient.PrependReactor("get", "secrets", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
		ga := action.(clientgotesting.GetAction)
		if ga.GetNamespace() != th.SecretNs || ga.GetName() != th.SecretName {
			return false, nil, nil
		}
		secret := &corev1.Secret{
			ObjectMeta: apimetav1.ObjectMeta{
				Namespace: ga.GetNamespace(),
				Name:      ga.GetName(),
			},
			Data: th.SecretData(),
		}
		return true, secret, nil
	})

	return fakeKubeClient
}

func (th *testHarness) SecretData() map[string][]byte {
	return map[string][]byte{
		"userID":  []byte("user-id"),
		"userKey": []byte("user-key"),
	}
}

func (th *testHarness) makeFakeCBTClient() *fakecbt.Clientset {
	fakeCBTClient := fakecbt.NewSimpleClientset()
	fakeCBTClient.PrependReactor("get", "snapshotmetadataservices", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
		ga := action.(clientgotesting.GetAction)
		if ga.GetName() != th.DriverName {
			return false, nil, nil
		}
		sms := &smsv1alpha1.SnapshotMetadataService{
			ObjectMeta: apimetav1.ObjectMeta{
				Name: th.DriverName,
			},
			Spec: smsv1alpha1.SnapshotMetadataServiceSpec{
				Audience: th.Audience,
			},
		}
		return true, sms, nil
	})
	return fakeCBTClient

}

func (th *testHarness) makeFakeSnapshotClient() *fakesnapshot.Clientset {
	fakeSnapshotClient := fakesnapshot.NewSimpleClientset()
	fakeSnapshotClient.PrependReactor("get", "volumesnapshots", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
		ga := action.(clientgotesting.GetAction)
		vs := th.VolumeSnapshot(ga.GetName(), ga.GetNamespace())
		return true, vs, nil
	})
	fakeSnapshotClient.PrependReactor("get", "volumesnapshotcontents", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
		ga := action.(clientgotesting.GetAction)
		vsc := th.VolumeSnapshotContent(ga.GetName(), th.DriverName)
		return true, vsc, nil
	})
	fakeSnapshotClient.PrependReactor("get", "volumesnapshotclasses", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
		ga := action.(clientgotesting.GetAction)
		if ga.GetName() != th.VolumeSnapshotClassName {
			return false, nil, nil
		}
		vs := th.VSCNoAnn() // Not the default but parameters set
		vs.Name = ga.GetName()
		return true, vs, nil
	})
	return fakeSnapshotClient
}

// FakeCountReactors saves the length of the fake reaction chains.
// Implicitly called by WithFakeClientAPIs but can be explicitly
// called if additional "permanent" reactors are added.
func (th *testHarness) FakeCountReactors() {
	th.lenFakeKubeClientReactionChain = len(th.FakeKubeClient.ReactionChain)
	th.lenFakeSnapshotClientReactionChain = len(th.FakeSnapshotClient.ReactionChain)
	th.lenFakeCBTClientReactionChain = len(th.FakeCBTClient.ReactionChain)
}

// FakePopPrependedReactors pops reactors from the front of the fake reaction chains.
// Do not use if AddReactor was used after the call to CountFakeReactors.
func (th *testHarness) FakePopPrependedReactors() {
	var numToPop int

	numToPop = len(th.FakeKubeClient.ReactionChain) - th.lenFakeKubeClientReactionChain
	if numToPop > 0 {
		th.FakeKubeClient.ReactionChain = th.FakeKubeClient.ReactionChain[numToPop:]
	}

	numToPop = len(th.FakeSnapshotClient.ReactionChain) - th.lenFakeSnapshotClientReactionChain
	if numToPop > 0 {
		th.FakeSnapshotClient.ReactionChain = th.FakeSnapshotClient.ReactionChain[numToPop:]
	}

	numToPop = len(th.FakeCBTClient.ReactionChain) - th.lenFakeCBTClientReactionChain
	if numToPop > 0 {
		th.FakeCBTClient.ReactionChain = th.FakeCBTClient.ReactionChain[numToPop:]
	}
}

// ContentNameFromSnapshot returns the content name for a snapshot name.
func (th *testHarness) ContentNameFromSnapshot(snapshotName string) string {
	return "snapcontent-" + snapshotName
}

// SnapshotNameFromContent is the inverse of ContentNameFromSnapshot().
func (th *testHarness) SnapshotNameFromContent(contentName string) string {
	return strings.TrimPrefix(contentName, "snapcontent-") // Identity if prefix missing.
}

// HandleFromSnapshot returns a snapshot identifier based on the snapshot name.
func (th *testHarness) HandleFromSnapshot(snapshotName string) string {
	return snapshotName + "-id"
}

func (th *testHarness) VolumeSnapshot(snapshotName, namespace string) *snapshotv1.VolumeSnapshot {
	return &snapshotv1.VolumeSnapshot{
		ObjectMeta: apimetav1.ObjectMeta{
			Name:      snapshotName,
			Namespace: namespace,
		},
		Spec: snapshotv1.VolumeSnapshotSpec{
			VolumeSnapshotClassName: stringPtr(th.VolumeSnapshotClassName),
			Source: snapshotv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: stringPtr("pvc-1"),
			},
		},
		Status: &snapshotv1.VolumeSnapshotStatus{
			ReadyToUse:                     boolPtr(true),
			BoundVolumeSnapshotContentName: stringPtr(th.ContentNameFromSnapshot(snapshotName)),
		},
	}
}

func (th *testHarness) VolumeSnapshotContent(contentName, driver string) *snapshotv1.VolumeSnapshotContent {
	return &snapshotv1.VolumeSnapshotContent{
		ObjectMeta: apimetav1.ObjectMeta{
			Name: contentName,
		},
		Spec: snapshotv1.VolumeSnapshotContentSpec{
			Driver: driver,
			Source: snapshotv1.VolumeSnapshotContentSource{
				VolumeHandle: stringPtr("volume-" + contentName),
			},
			VolumeSnapshotRef: corev1.ObjectReference{
				Name:      th.SnapshotNameFromContent(contentName),
				Namespace: "test",
			},
		},
		Status: &snapshotv1.VolumeSnapshotContentStatus{
			ReadyToUse:     boolPtr(true),
			SnapshotHandle: stringPtr(th.HandleFromSnapshot(th.SnapshotNameFromContent(contentName))),
		},
	}
}

// VSCParams returns the parameters for a VolumeSnapshotClass.
func (th *testHarness) VSCParams() map[string]string {
	return map[string]string{
		PrefixedSnapshotterSecretNamespaceKey: th.SecretNs,
		PrefixedSnapshotterSecretNameKey:      th.SecretName,
	}
}

func (th *testHarness) VSCIsDefaultTrue() *snapshotv1.VolumeSnapshotClass {
	return &snapshotv1.VolumeSnapshotClass{
		ObjectMeta: apimetav1.ObjectMeta{
			Name: "vsc-is-default",
			Annotations: map[string]string{
				snapshotutils.IsDefaultSnapshotClassAnnotation: "true",
			},
		},
		Driver:     th.DriverName,
		Parameters: th.VSCParams(),
	}
}

func (th *testHarness) VSCIsDefaultFalse() *snapshotv1.VolumeSnapshotClass {
	vsc := th.VSCIsDefaultTrue()
	vsc.Name = "vsc-is-not-the-default"
	vsc.Annotations[snapshotutils.IsDefaultSnapshotClassAnnotation] = "false"
	return vsc
}

func (th *testHarness) VSCOtherAnn() *snapshotv1.VolumeSnapshotClass {
	vsc := th.VSCIsDefaultTrue()
	vsc.Name = "vsc-other-annotation"
	vsc.Annotations = map[string]string{"other": "annotation"}
	return vsc
}

func (th *testHarness) VSCNoAnn() *snapshotv1.VolumeSnapshotClass {
	vsc := th.VSCIsDefaultTrue()
	vsc.Name = "vsc-no-annotation"
	vsc.Annotations = nil
	return vsc
}

func (th *testHarness) VSCOtherDriverDefault() *snapshotv1.VolumeSnapshotClass {
	vsc := th.VSCIsDefaultTrue()
	vsc.Name = "vsc-other-driver-default"
	vsc.Driver = "other-" + th.DriverName
	return vsc
}

func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func convStringByteMapToStringStringMap(inMap map[string][]byte) map[string]string {
	if inMap == nil {
		return nil
	}
	ret := make(map[string]string, len(inMap))
	for k, v := range inMap {
		ret[k] = string(v)
	}
	return ret
}
