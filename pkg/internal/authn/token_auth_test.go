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

package authn

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"
)

func TestAuthorization(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		token          string
		audience       string
		reactor        clientgotesting.ReactionFunc
		expectedResult bool
		errExpected    bool
	}{
		// authenticated token, valid audience
		{
			audience: "xxxxxaaaa",
			reactor: func(action clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
				tokenReview := &authv1.TokenReview{
					Status: authv1.TokenReviewStatus{
						Authenticated: true,
						Audiences:     []string{"xxxxxaaaa", "xxxxxaaab"},
					},
				}
				return true, tokenReview, nil
			},
			expectedResult: true,
			errExpected:    false,
		},
		// authenticated token, invalid audience
		{
			audience: "xxxxxinvalid",
			reactor: func(action clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
				tokenReview := &authv1.TokenReview{
					Status: authv1.TokenReviewStatus{
						Authenticated: true,
						Audiences:     []string{"xxxxxaaaa", "xxxxxaaab"},
					},
				}
				return true, tokenReview, nil
			},
			expectedResult: false,
			errExpected:    false,
		},
		// unauthenticated token
		{
			token:    "yyyyyyaaaa",
			audience: "xxxxxinvalid",
			reactor: func(action clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
				tokenReview := &authv1.TokenReview{
					Status: authv1.TokenReviewStatus{
						Authenticated: false,
					},
				}
				return false, tokenReview, nil
			},
			expectedResult: false,
			errExpected:    false,
		},
		// error in create tokenreview request
		{
			token:    "yyyyyyaaaa",
			audience: "xxxxxinvalid",
			reactor: func(action clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, errors.New("failed to create TokenReview")
			},
			expectedResult: false,
			errExpected:    true,
		},
	} {
		fakeClientset := fake.NewSimpleClientset()
		fakeClientset.PrependReactor("create", "tokenreviews", tc.reactor)
		authenticator := NewTokenAuthenticator(fakeClientset)
		result, _, err := authenticator.Authenticate(ctx, "yyyyyyaaaa", tc.audience)
		if tc.errExpected {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
			assert.Equal(t, tc.expectedResult, result)
		}
	}
}
