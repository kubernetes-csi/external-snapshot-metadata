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
	"testing"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	fakesnapshot "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned/fake"
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

	SecurityToken string
	DriverName    string
	Audience      string
	Namespace     string

	fakeKubeClient     *fake.Clientset
	fakeSnapshotClient *fakesnapshot.Clientset
	fakeCBTClient      *fakecbt.Clientset

	grpcServer *grpc.Server
	listener   *bufconn.Listener
}

func newTestHarness() *testHarness {
	return &testHarness{
		SecurityToken: "securityToken",
		DriverName:    "driver",
		Audience:      "audience",
		Namespace:     "namespace",
	}
}

func (th *testHarness) WithMockCSIDriver(t *testing.T) *testHarness {
	th.TestHarness = runtime.NewTestHarness().WithMockCSIDriver(t)
	return th
}

func (th *testHarness) WithFakeClientAPIs() *testHarness {
	th.fakeKubeClient = th.makeFakeKubeClient()
	th.fakeSnapshotClient = th.makeFakeSnapshotClient()
	th.fakeCBTClient = th.makeFakeCBTClient()
	return th
}

func (th *testHarness) Runtime() *runtime.Runtime {
	return &runtime.Runtime{
		CBTClient:      th.fakeCBTClient,
		KubeClient:     th.fakeKubeClient,
		SnapshotClient: th.fakeSnapshotClient,
		DriverName:     th.DriverName,
	}
}

func (th *testHarness) RuntimeWithMockCSIDriver(t *testing.T) *runtime.Runtime {
	assert.NotNil(t, th.MockCSIDriverConn, "needs WithMockCSIDriver")
	rt := th.Runtime()
	rt.CSIConn = th.MockCSIDriverConn
	rt.Args = th.RuntimeArgs()
	return rt
}

func (th *testHarness) ServerWithRuntime(t *testing.T, rt *runtime.Runtime) *Server {
	s := &Server{
		config:       ServerConfig{Runtime: rt},
		healthServer: newHealthServer(),
	}

	return s
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

	return fakeKubeClient
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
		vs := volumeSnapshot(ga.GetName(), ga.GetNamespace())
		return true, vs, nil
	})
	fakeSnapshotClient.PrependReactor("get", "volumesnapshotcontents", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
		ga := action.(clientgotesting.GetAction)
		vsc := volumeSnapshotContent(ga.GetName(), th.DriverName)
		return true, vsc, nil
	})
	return fakeSnapshotClient
}

func volumeSnapshot(name, namespace string) *snapshotv1.VolumeSnapshot {
	return &snapshotv1.VolumeSnapshot{
		ObjectMeta: apimetav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: snapshotv1.VolumeSnapshotSpec{
			VolumeSnapshotClassName: stringPtr("csi-snapshot-class"),
			Source: snapshotv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: stringPtr("pvc-1"),
			},
		},
		Status: &snapshotv1.VolumeSnapshotStatus{
			ReadyToUse:                     boolPtr(true),
			BoundVolumeSnapshotContentName: stringPtr(name),
		},
	}
}

func volumeSnapshotContent(name, driver string) *snapshotv1.VolumeSnapshotContent {
	return &snapshotv1.VolumeSnapshotContent{
		ObjectMeta: apimetav1.ObjectMeta{
			Name: name,
		},
		Spec: snapshotv1.VolumeSnapshotContentSpec{
			Driver: driver,
			Source: snapshotv1.VolumeSnapshotContentSource{
				VolumeHandle: stringPtr("volume-" + name),
			},
			VolumeSnapshotRef: corev1.ObjectReference{
				Name:      "snapshot-" + name,
				Namespace: "test",
			},
		},
		Status: &snapshotv1.VolumeSnapshotContentStatus{
			ReadyToUse:     boolPtr(true),
			SnapshotHandle: stringPtr(name + "-id"),
		},
	}
}

func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}
