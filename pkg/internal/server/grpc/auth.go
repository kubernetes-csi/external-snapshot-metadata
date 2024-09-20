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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kauthorizer "k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/klog/v2"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/internal/authn"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/internal/authz"
)

func (s *Server) authenticateAndAuthorize(ctx context.Context, token string, namespace string) error {
	// Authenticate request with security token and find the user identity
	authenticated, userInfo, err := s.authenticateRequest(ctx, token)
	if err != nil {
		return status.Errorf(codes.Internal, msgInternalFailedToAuthenticateFmt, err)
	}
	if !authenticated {
		klog.FromContext(ctx).Error(err, msgUnauthenticatedUser, "userInfo", userInfo)
		return status.Error(codes.Unauthenticated, msgUnauthenticatedUser)
	}

	// Authorize user
	authorizer := authz.NewSARAuthorizer(s.kubeClient())
	decision, reason, err := authorizer.Authorize(ctx, userInfo, namespace)
	if err != nil {
		return status.Errorf(codes.Internal, mgsInternalFailedToAuthorizeFmt, err)
	}
	if decision != kauthorizer.DecisionAllow {
		klog.FromContext(ctx).Error(err, msgPermissionDeniedPrefix, "userInfo", userInfo)
		return status.Errorf(codes.PermissionDenied, msgPermissionDeniedFmt, reason)
	}

	return nil
}

func (s *Server) authenticateRequest(ctx context.Context, securityToken string) (bool, *authv1.UserInfo, error) {
	// Find audienceToken from SnapshotMetadataService CR for the driver
	audience, err := s.getAudienceForDriver(ctx)
	if err != nil {
		return false, nil, err
	}

	// Authenticate request with the security token
	authenticator := authn.NewTokenAuthenticator(s.kubeClient())
	return authenticator.Authenticate(ctx, securityToken, audience)
}

func (s *Server) getAudienceForDriver(ctx context.Context) (string, error) {
	sms, err := s.cbtClient().CbtV1alpha1().SnapshotMetadataServices().Get(ctx, s.driverName(), metav1.GetOptions{})
	if err != nil {
		klog.FromContext(ctx).Error(err, msgInternalFailedToFindCR, "driver", s.driverName())
		return "", fmt.Errorf(msgInternalFailedToFindCRFmt, s.driverName(), err)
	}

	return sms.Spec.Audience, nil
}
