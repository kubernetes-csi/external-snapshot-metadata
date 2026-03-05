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

	authv1 "k8s.io/api/authentication/v1"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// TokenAuthenticator authenticates the user using K8s TokenReview API
type TokenAuthenticator struct {
	kubeClient kubernetes.Interface
}

func NewTokenAuthenticator(kubeClient kubernetes.Interface) *TokenAuthenticator {
	return &TokenAuthenticator{kubeClient: kubeClient}
}

// Authenticate the user with security token and find the user identity.
// TokenReview API is used to validated the token with given audience.
func (t *TokenAuthenticator) Authenticate(ctx context.Context, token string, audience string) (bool, *authv1.UserInfo, error) {
	// https://pkg.go.dev/k8s.io/api/authentication/v1#TokenReview
	tokenReview := authv1.TokenReview{
		Spec: authv1.TokenReviewSpec{
			Token:     token,
			Audiences: []string{audience},
		},
	}
	auth, err := t.kubeClient.AuthenticationV1().TokenReviews().Create(ctx, &tokenReview, apimetav1.CreateOptions{})
	if err != nil {
		klog.FromContext(ctx).Error(err, "TokenReviews.Create", "audiences", audience)
		return false, nil, err
	}
	if !auth.Status.Authenticated {
		return false, nil, nil
	}
	if len(auth.Status.Audiences) == 0 {
		return false, nil, nil
	}
	for _, aud := range auth.Status.Audiences {
		if aud == audience {
			return true, &auth.Status.User, nil
		}
	}
	return false, nil, nil
}
