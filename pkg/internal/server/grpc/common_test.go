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

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"
	authv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/authorization/v1"
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
	SecurityToken string
	DriverName    string
	Audience      string
	Namespace     string

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

func (th *testHarness) ServerWithClientAPIs() *Server {
	s := &Server{
		config: ServerConfig{Runtime: &runtime.Runtime{
			CBTClient:  th.FakeCBTClient(),
			KubeClient: th.FakeKubeClient(),
			DriverName: th.DriverName,
		}},
		healthServer: newHealthServer(),
	}

	return s
}

func (th *testHarness) StartGRPCServer(t *testing.T) *Server {
	buffer := 1024 * 1024

	th.listener = bufconn.Listen(buffer)
	th.grpcServer = grpc.NewServer()

	s := th.ServerWithClientAPIs()
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

func (th *testHarness) FakeKubeClient() *fake.Clientset {
	kubeClient := fake.NewSimpleClientset()

	kubeClient.PrependReactor("create", "tokenreviews", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
		ca := action.(clientgotesting.CreateAction)
		trs := ca.GetObject().(*authv1.TokenReview)
		trs.Status.Authenticated = false
		if trs.Spec.Token == th.SecurityToken {
			trs.Status.Authenticated = true
			trs.Status.Audiences = []string{th.Audience, "other-" + th.Audience}
		}
		return true, trs, nil
	})

	kubeClient.PrependReactor("create", "subjectaccessreviews", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
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

	return kubeClient
}

func (th *testHarness) FakeCBTClient() *fakecbt.Clientset {
	cbtClient := fakecbt.NewSimpleClientset()
	cbtClient.PrependReactor("get", "snapshotmetadataservices", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
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

	return cbtClient
}
