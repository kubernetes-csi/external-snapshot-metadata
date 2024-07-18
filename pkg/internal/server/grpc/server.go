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
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kauthorizer "k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/client-go/kubernetes"
	klog "k8s.io/klog/v2"

	cbt "github.com/kubernetes-csi/external-snapshot-metadata/client/clientset/versioned"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/internal/authn"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/internal/authz"
)

type ServerConfig struct {
	Port        int
	TLSKeyFile  string
	TLSCertFile string
}

type Server struct {
	api.UnimplementedSnapshotMetadataServer
	grpcServer *grpc.Server
	config     ServerConfig
	kubeClient kubernetes.Interface
	cbtClient  cbt.Interface
	driverName string
}

func NewServer(
	kubeClient kubernetes.Interface,
	cbtClient cbt.Interface,
	driverName string,
	config ServerConfig,
) (*Server, error) {
	options, err := buildOptions(config)
	if err != nil {
		return nil, err
	}
	server := grpc.NewServer(options...)
	return &Server{
		grpcServer: server,
		config:     config,
		kubeClient: kubeClient,
		cbtClient:  cbtClient,
		driverName: driverName,
	}, nil
}

func buildOptions(config ServerConfig) ([]grpc.ServerOption, error) {
	tlsOptions, err := buildTLSOption(config.TLSCertFile, config.TLSKeyFile)
	if err != nil {
		return nil, err
	}
	return []grpc.ServerOption{
		tlsOptions,
	}, nil
}

func buildTLSOption(cert, key string) (grpc.ServerOption, error) {
	serverCert, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return nil, fmt.Errorf("failed to load tls certificates: %v", err)
	}
	config := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.NoClientCert,
	}

	return grpc.Creds(credentials.NewTLS(config)), nil
}

func (s *Server) Start() {
	listener, err := net.Listen("tcp", ":"+strconv.Itoa(s.config.Port))
	if err != nil {
		klog.Fatalf("failed to start grpc server: %v", err)
	}
	s.registerService()

	go func() {
		klog.Info("csi-snapshot-metadata sidecar GRPC server started listening of port :", s.config.Port)
		if err := s.grpcServer.Serve(listener); err != nil {
			klog.Fatalf("failed to start grpc server: %v", err)
		}
	}()

	// Wait for termination signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(
		sigCh, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM,
	)
	<-sigCh
	klog.Info("gracefully shutting down")
	s.grpcServer.GracefulStop()
}

func (s *Server) getAudienceForDriver(ctx context.Context) (string, error) {
	sms, err := s.cbtClient.CbtV1alpha1().SnapshotMetadataServices().Get(ctx, s.driverName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get SnapshotMetadataService resource for driver %s: %v", s.driverName, err)
	}
	return sms.Spec.Audience, nil
}

func (s *Server) authRequest(ctx context.Context, securityToken string) (bool, *authv1.UserInfo, error) {
	// Find audienceToken from SnapshotMetadataService CR for the driver
	audience, err := s.getAudienceForDriver(ctx)
	if err != nil {
		return false, nil, err
	}
	// Authenticate request with the security token
	authenticator := authn.NewTokenAuthenticator(s.kubeClient)
	return authenticator.Authenticate(ctx, securityToken, audience)
}

func (s *Server) authenticateAndAuthorize(ctx context.Context, token string, namespace string) error {
	// Authenticate request with security token and find the user identity
	authenticated, userInfo, err := s.authRequest(ctx, token)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to authenticate user: %v", err)
	}
	if !authenticated {
		return status.Error(codes.Unauthenticated, "unauthenticated user")
	}
	// Authorize user
	authorizer := authz.NewSARAuthorizer(s.kubeClient)
	decision, reason, err := authorizer.Authorize(ctx, userInfo, namespace)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to authorize the user: %v", err)
	}
	if decision != kauthorizer.DecisionAllow {
		return status.Errorf(codes.PermissionDenied, "user does not have permissions to perform the operation, %s, %v", reason, err.Error())
	}
	return nil
}
