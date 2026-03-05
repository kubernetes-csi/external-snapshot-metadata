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

package authz

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	authv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"
)

func TestAuthenticate(t *testing.T) {
	ctx := context.Background()
	userInfo := &authv1.UserInfo{
		Username: "user-a",
		Groups:   []string{"group-1", "group-2"},
		UID:      "uid-123",
		Extra: map[string]authv1.ExtraValue{
			"extra-key": {"extra-value"},
		},
	}
	namespace := "test-namespace"

	for _, tc := range []struct {
		name             string
		reactor          clientgotesting.ReactionFunc
		expectedDecision authorizer.Decision
		expectedReason   string
		errExpected      bool
	}{
		// authorized user
		{
			name: "authorized user",
			reactor: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				// Introspect the request to validate correct arguments
				createAction := action.(clientgotesting.CreateAction)
				sar := createAction.GetObject().(*authzv1.SubjectAccessReview)

				// Validate user information
				assert.Equal(t, userInfo.Username, sar.Spec.User, "username should match")
				assert.Equal(t, userInfo.Groups, sar.Spec.Groups, "groups should match")
				assert.Equal(t, userInfo.UID, sar.Spec.UID, "UID should match")
				assert.Equal(t, len(userInfo.Extra), len(sar.Spec.Extra), "extra fields count should match")
				for k, v := range userInfo.Extra {
					assert.Equal(t, authzv1.ExtraValue(v), sar.Spec.Extra[k], "extra value should match")
				}

				// Validate resource attributes
				assert.NotNil(t, sar.Spec.ResourceAttributes, "resource attributes should be set")
				assert.Equal(t, "get", sar.Spec.ResourceAttributes.Verb, "verb should be 'get'")
				assert.Equal(t, namespace, sar.Spec.ResourceAttributes.Namespace, "namespace should match")
				assert.Equal(t, "snapshot.storage.k8s.io", sar.Spec.ResourceAttributes.Group, "group should be 'snapshot.storage.k8s.io'")
				assert.Equal(t, "v1", sar.Spec.ResourceAttributes.Version, "version should be 'v1'")
				assert.Equal(t, "volumesnapshots", sar.Spec.ResourceAttributes.Resource, "resource should be 'volumesnapshots'")

				response := &authzv1.SubjectAccessReview{
					Status: authzv1.SubjectAccessReviewStatus{
						Allowed: true,
						Reason:  "mock reason",
					},
				}
				return true, response, nil
			},
			expectedDecision: authorizer.DecisionAllow,
			expectedReason:   "mock reason",
			errExpected:      false,
		},
		// unauthorized user
		{
			name: "unauthorized user",
			reactor: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				// Introspect the request
				createAction := action.(clientgotesting.CreateAction)
				sar := createAction.GetObject().(*authzv1.SubjectAccessReview)

				// Validate request structure
				assert.Equal(t, userInfo.Username, sar.Spec.User, "username should match")
				assert.NotNil(t, sar.Spec.ResourceAttributes, "resource attributes should be set")

				response := &authzv1.SubjectAccessReview{
					Status: authzv1.SubjectAccessReviewStatus{
						Allowed: false,
						Reason:  "mock reason",
					},
				}
				return true, response, nil
			},
			expectedDecision: authorizer.DecisionDeny,
			expectedReason:   "mock reason",
			errExpected:      false,
		},
		// error in authorization
		{
			name: "error in authorization",
			reactor: func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
				return true, nil, errors.New("failed to create SubjectAccessReview")
			},
			expectedDecision: authorizer.DecisionDeny,
			expectedReason:   "",
			errExpected:      true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeClientset := fake.NewSimpleClientset()
			fakeClientset.PrependReactor("create", "subjectaccessreviews", tc.reactor)
			authz := NewSARAuthorizer(fakeClientset)
			decision, reason, err := authz.Authorize(ctx, userInfo, namespace)
			if tc.errExpected {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedDecision, decision)
				assert.Equal(t, tc.expectedReason, reason)
			}
		})
	}
}
