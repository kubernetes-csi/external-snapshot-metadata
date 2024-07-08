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
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"k8s.io/client-go/kubernetes"
	klog "k8s.io/klog/v2"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
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
	KubeCli    kubernetes.Interface
}

func NewServer(kubeCli kubernetes.Interface, config ServerConfig) (*Server, error) {
	options, err := buildOptions(config)
	if err != nil {
		return nil, err
	}
	server := grpc.NewServer(options...)
	return &Server{
		grpcServer: server,
		config:     config,
		KubeCli:    kubeCli,
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
