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
	"k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"
)

func TestAuthenticate(t *testing.T) {
	ctx := context.Background()
	userInfo := &authv1.UserInfo{
		Username: "user-a",
	}
	for _, tc := range []struct {
		allowed          string
		reactor          clientgotesting.ReactionFunc
		expectedDecision authorizer.Decision
		expectedReason   string
		errExpected      bool
	}{
		// authorized user
		{
			reactor: func(action clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
				response := &v1.SubjectAccessReview{
					Status: v1.SubjectAccessReviewStatus{
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
			reactor: func(action clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
				response := &v1.SubjectAccessReview{
					Status: v1.SubjectAccessReviewStatus{
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
			reactor: func(action clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, errors.New("failed to create SubjectAccessReview")
			},
			expectedDecision: authorizer.DecisionDeny,
			expectedReason:   "mock reason",
			errExpected:      true,
		},
	} {
		fakeClientset := fake.NewSimpleClientset()
		fakeClientset.PrependReactor("create", "subjectaccessreviews", tc.reactor)
		authz := NewSARAuthorizer(fakeClientset)
		decision, reason, err := authz.Authorize(ctx, userInfo, "default")
		if tc.errExpected {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
			assert.Equal(t, tc.expectedDecision, decision)
			assert.Equal(t, tc.expectedReason, reason)
		}
	}
}
