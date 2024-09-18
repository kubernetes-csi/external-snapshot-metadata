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

	authv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// SARAuthorizer authorizes user using K8s SubjectAccessReview API
type SARAuthorizer struct {
	kubeClient kubernetes.Interface
}

func NewSARAuthorizer(cli kubernetes.Interface) *SARAuthorizer {
	return &SARAuthorizer{kubeClient: cli}
}

// Authorize check if the user is authorized to access volumesnapshots resources using SubjectAccessReview API
func (s *SARAuthorizer) Authorize(ctx context.Context, userInfo *authv1.UserInfo, namespace string) (authorizer.Decision, string, error) {
	extra := make(map[string]authzv1.ExtraValue, len(userInfo.Extra))
	for u, e := range userInfo.Extra {
		extra[u] = authzv1.ExtraValue(e)
	}
	sar := &authzv1.SubjectAccessReview{
		Spec: authzv1.SubjectAccessReviewSpec{
			ResourceAttributes: volumeSnapshotResourceAttrib(namespace),
			User:               userInfo.Username,
			Groups:             userInfo.Groups,
			Extra:              extra,
			UID:                userInfo.UID,
		},
	}
	sarResp, err := s.kubeClient.AuthorizationV1().SubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		klog.FromContext(ctx).Error(err, "SubjectAccessReviews.Create", "userInfo", userInfo)
		return authorizer.DecisionDeny, "", err
	}
	if !sarResp.Status.Allowed || sarResp.Status.Denied {
		return authorizer.DecisionDeny, sarResp.Status.Reason, nil
	}
	return authorizer.DecisionAllow, sarResp.Status.Reason, nil
}

func volumeSnapshotResourceAttrib(namespace string) *authzv1.ResourceAttributes {
	return &authzv1.ResourceAttributes{
		Verb:      "get",
		Namespace: namespace,
		Group:     "snapshot.storage.k8s.io",
		Version:   "v1",
		Resource:  "volumesnapshots",
	}
}
