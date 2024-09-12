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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"
)

func TestAuthenticateAndAuthorize(t *testing.T) {
	t.Run("crd-get-error", func(t *testing.T) {
		th := newTestHarness().WithFakeClientAPIs().WithMockCSIDriver(t)
		s := th.ServerWithRuntime(t, th.Runtime())
		s.config.Runtime.DriverName += "foo"
		assert.NotEqual(t, th.DriverName, s.driverName())

		// direct call
		retAudience, err := s.getAudienceForDriver(context.Background())
		assert.Error(t, err)
		assert.Empty(t, retAudience)

		// fail via authenticateAndAuthorize
		err = s.authenticateAndAuthorize(context.Background(), "some-token", "some-namespace")
		assert.Error(t, err)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.ErrorContains(t, err, msgInternalFailedToAuthenticatePrefix)
	})

	t.Run("crd-get-success", func(t *testing.T) {
		th := newTestHarness().WithFakeClientAPIs().WithMockCSIDriver(t)
		s := th.ServerWithRuntime(t, th.Runtime())

		retAudience, err := s.getAudienceForDriver(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, th.Audience, retAudience)
	})

	t.Run("authentication-error", func(t *testing.T) {
		th := newTestHarness().WithFakeClientAPIs().WithMockCSIDriver(t)
		s := th.ServerWithRuntime(t, th.Runtime())

		// intercept the creation and fail
		kubeClient := s.config.Runtime.KubeClient.(*fake.Clientset)
		kubeClient.PrependReactor("create", "tokenreviews", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
			return true, nil, errors.New("create-tokenreview-error")
		})

		// direct call
		authenticated, ui, err := s.authenticateRequest(context.Background(), th.SecurityToken+"foo")
		assert.Error(t, err)
		assert.False(t, authenticated)
		assert.Nil(t, ui)

		// fail via authenticateAndAuthorize
		err = s.authenticateAndAuthorize(context.Background(), th.SecurityToken+"foo", "some-namespace")
		assert.Error(t, err)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.ErrorContains(t, err, msgInternalFailedToAuthenticatePrefix)
	})

	t.Run("not-authenticated", func(t *testing.T) {
		th := newTestHarness().WithFakeClientAPIs().WithMockCSIDriver(t)
		s := th.ServerWithRuntime(t, th.Runtime())

		// direct call
		authenticated, ui, err := s.authenticateRequest(context.Background(), th.SecurityToken+"foo")
		assert.NoError(t, err)
		assert.False(t, authenticated)
		assert.Nil(t, ui)

		// fails via authenticateAndAuthorize
		err = s.authenticateAndAuthorize(context.Background(), th.SecurityToken+"foo", "some-namespace")
		assert.Error(t, err)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, st.Code())
		assert.ErrorContains(t, err, msgUnauthenticatedUser)
	})

	t.Run("authorization-error", func(t *testing.T) {
		th := newTestHarness().WithFakeClientAPIs().WithMockCSIDriver(t)
		s := th.ServerWithRuntime(t, th.Runtime())

		// intercept the creation and fail
		kubeClient := s.config.Runtime.KubeClient.(*fake.Clientset)
		kubeClient.PrependReactor("create", "subjectaccessreviews", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
			return true, nil, errors.New("create-subjectaccessreviews-error")
		})

		err := s.authenticateAndAuthorize(context.Background(), th.SecurityToken, th.Namespace)
		assert.Error(t, err)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
		assert.ErrorContains(t, err, mgsInternalFailedToAuthorizePrefix)
	})

	t.Run("not-authorized", func(t *testing.T) {
		th := newTestHarness().WithFakeClientAPIs().WithMockCSIDriver(t)
		s := th.ServerWithRuntime(t, th.Runtime())

		err := s.authenticateAndAuthorize(context.Background(), th.SecurityToken, th.Namespace+"foo")
		assert.Error(t, err)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.PermissionDenied, st.Code())
		assert.ErrorContains(t, err, msgPermissionDeniedPrefix)
	})

	t.Run("success", func(t *testing.T) {
		th := newTestHarness().WithFakeClientAPIs().WithMockCSIDriver(t)
		s := th.ServerWithRuntime(t, th.Runtime())

		err := s.authenticateAndAuthorize(context.Background(), th.SecurityToken, th.Namespace)
		assert.NoError(t, err)
	})
}
