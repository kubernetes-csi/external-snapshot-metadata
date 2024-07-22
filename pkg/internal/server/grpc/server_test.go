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
	"fmt"
	"io"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"
	klog "k8s.io/klog/v2"

	smsv1alpha1 "github.com/kubernetes-csi/external-snapshot-metadata/client/apis/snapshotmetadataservice/v1alpha1"
	fakecbt "github.com/kubernetes-csi/external-snapshot-metadata/client/clientset/versioned/fake"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
)

func runTestServer() (api.SnapshotMetadataClient, func()) {
	audience := "xxxxxxaaaaa"
	kubeClient := fake.NewSimpleClientset()
	kubeClient.PrependReactor("create", "tokenreviews", func(action clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
		tokenReview := &authv1.TokenReview{
			Status: authv1.TokenReviewStatus{
				Authenticated: true,
				Audiences:     []string{audience, "xxxxxaaab"},
			},
		}
		return true, tokenReview, nil
	})
	kubeClient.PrependReactor("create", "subjectaccessreviews", func(action clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
		response := &v1.SubjectAccessReview{
			Status: v1.SubjectAccessReviewStatus{
				Allowed: true,
				Reason:  "mock reason",
			},
		}
		return true, response, nil
	})
	cbtClient := fakecbt.NewSimpleClientset()
	cbtClient.PrependReactor("get", "snapshotmetadataservices", func(action clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
		sms := &smsv1alpha1.SnapshotMetadataService{
			Spec: smsv1alpha1.SnapshotMetadataServiceSpec{
				Audience: audience,
			},
		}
		return true, sms, nil
	})

	buffer := 1024 * 1024
	listner := bufconn.Listen(buffer)
	s := &Server{
		kubeClient: kubeClient,
		cbtClient:  cbtClient,
		grpcServer: grpc.NewServer(),
	}
	api.RegisterSnapshotMetadataServer(s.grpcServer, s)
	go func() {
		if err := s.grpcServer.Serve(listner); err != nil {
			klog.Fatalf("error serving server: %v", err)
		}
	}()
	conn, err := grpc.NewClient("passthrough://bufconn",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listner.Dial()
		}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		klog.Fatalf("error connecting to server: %v", err)
	}

	closer := func() {
		err := listner.Close()
		if err != nil {
			klog.Fatalf("error closing listener: %v", err)
		}
		s.grpcServer.Stop()
	}

	client := api.NewSnapshotMetadataClient(conn)
	return client, closer
}

func TestSnapshotMetadata_GetMetadataDeltaInvalidRequest(t *testing.T) {
	ctx := context.Background()
	client, closer := runTestServer()
	defer closer()

	for _, tc := range []struct {
		req           *api.GetMetadataDeltaRequest
		errExpected   bool
		expStatusCode codes.Code
	}{
		{
			req:           &api.GetMetadataDeltaRequest{},
			errExpected:   true,
			expStatusCode: codes.InvalidArgument,
		},
		{
			req: &api.GetMetadataDeltaRequest{
				Namespace:          "test-ns",
				BaseSnapshotName:   "snap-1",
				TargetSnapshotName: "snap-2",
			},
			errExpected:   true,
			expStatusCode: codes.InvalidArgument,
		},
		{
			req: &api.GetMetadataDeltaRequest{
				SecurityToken:      "token",
				BaseSnapshotName:   "snap-1",
				TargetSnapshotName: "snap-2",
			},
			errExpected:   true,
			expStatusCode: codes.InvalidArgument,
		},
		{
			req: &api.GetMetadataDeltaRequest{
				SecurityToken:      "token",
				Namespace:          "test-ns",
				TargetSnapshotName: "snap-2",
			},
			errExpected:   true,
			expStatusCode: codes.InvalidArgument,
		},
		{
			req: &api.GetMetadataDeltaRequest{
				SecurityToken:    "token",
				Namespace:        "test-ns",
				BaseSnapshotName: "snap-1",
			},
			errExpected:   true,
			expStatusCode: codes.InvalidArgument,
		},
		{
			req: &api.GetMetadataDeltaRequest{
				SecurityToken:      "token",
				Namespace:          "test-ns",
				BaseSnapshotName:   "snap-1",
				TargetSnapshotName: "snap-2",
			},
			errExpected: false,
		},
	} {
		stream, err := client.GetMetadataDelta(ctx, tc.req)
		if err != nil {
			t.Error(err)
		}
		_, errStream := stream.Recv()
		err1 := validateErrorStatus(tc.errExpected, errStream)
		if err1 != nil {
			t.Error(err1)
			continue
		}
	}
}

func TestSnapshotMetadata_GetMetadataAllocatedInvalidRequest(t *testing.T) {
	ctx := context.Background()
	client, closer := runTestServer()
	defer closer()

	for _, tc := range []struct {
		req           *api.GetMetadataAllocatedRequest
		errExpected   bool
		expStatusCode codes.Code
	}{
		{
			req:           &api.GetMetadataAllocatedRequest{},
			errExpected:   true,
			expStatusCode: codes.InvalidArgument,
		},
		{
			req: &api.GetMetadataAllocatedRequest{
				Namespace:    "test-ns",
				SnapshotName: "snap-1",
			},
			errExpected:   true,
			expStatusCode: codes.InvalidArgument,
		},
		{
			req: &api.GetMetadataAllocatedRequest{
				SecurityToken: "token",
				SnapshotName:  "snap-1",
			},
			errExpected:   true,
			expStatusCode: codes.InvalidArgument,
		},
		{
			req: &api.GetMetadataAllocatedRequest{
				SecurityToken: "token",
				Namespace:     "test-ns",
			},
			errExpected:   true,
			expStatusCode: codes.InvalidArgument,
		},
		{
			req: &api.GetMetadataAllocatedRequest{
				SecurityToken: "token",
				Namespace:     "test-ns",
				SnapshotName:  "snap-1",
			},
			errExpected: false,
		},
	} {
		stream, err := client.GetMetadataAllocated(ctx, tc.req)
		if err != nil {
			t.Error(err)
		}
		_, errStream := stream.Recv()
		err1 := validateErrorStatus(tc.errExpected, errStream)
		if err1 != nil {
			t.Error(err1)
			continue
		}
	}
}

func validateErrorStatus(errExpected bool, errStream error) error {
	if !errExpected && errStream != nil {
		if errStream != io.EOF {
			return fmt.Errorf("received unexpected error: %v", errStream)
		}
		return nil
	}
	if errExpected && errStream == nil {
		return fmt.Errorf("expected rpc error with code %v, received nil", codes.InvalidArgument)
	}
	st, ok := status.FromError(errStream)
	if !ok {
		return fmt.Errorf("Failed to parse error")
	}
	if st.Code() != codes.InvalidArgument {
		return fmt.Errorf("expected rpc error with code %v, received %v", codes.InvalidArgument, st.Code())
	}
	return nil
}
