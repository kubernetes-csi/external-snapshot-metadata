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
	"strconv"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"k8s.io/client-go/kubernetes"
	klog "k8s.io/klog/v2"

	cbt "github.com/kubernetes-csi/external-snapshot-metadata/client/clientset/versioned"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/internal/runtime"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"
)

type ServerConfig struct {
	Runtime *runtime.Runtime
}

type Server struct {
	api.UnimplementedSnapshotMetadataServer

	config       ServerConfig
	grpcServer   *grpc.Server
	healthServer *health.Server
}

func NewServer(config ServerConfig) (*Server, error) {
	options, err := buildOptions(config)
	if err != nil {
		return nil, err
	}

	return &Server{
		config:       config,
		grpcServer:   grpc.NewServer(options...),
		healthServer: newHealthServer(),
	}, nil
}

func (s *Server) driverName() string {
	return s.config.Runtime.DriverName
}

func (s *Server) cbtClient() cbt.Interface {
	return s.config.Runtime.CBTClient
}

func (s *Server) kubeClient() kubernetes.Interface {
	return s.config.Runtime.KubeClient
}

func (s *Server) snapshotClient() snapshot.Interface {
	return s.config.Runtime.SnapshotClient
}

func (s *Server) csiConnection() *grpc.ClientConn {
	return s.config.Runtime.CSIConn
}

func buildOptions(config ServerConfig) ([]grpc.ServerOption, error) {
	tlsOptions, err := buildTLSOption(config.Runtime.TLSCertFile, config.Runtime.TLSKeyFile)
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

// Start start the gRPC server in its own goroutine.
// The method guarantees that on successful return the gRPC server goroutine is running.
// The invoker should use the Stop() method to terminate the server when desired.
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", ":"+strconv.Itoa(s.config.Runtime.GRPCPort))
	if err != nil {
		klog.Errorf("failed to start grpc server: %v", err)
		return err
	}

	api.RegisterSnapshotMetadataServer(s.grpcServer, s)
	healthpb.RegisterHealthServer(s.grpcServer, s.healthServer)

	wgGoroutineStarted := sync.WaitGroup{}
	wgGoroutineStarted.Add(1)

	go func() {
		wgGoroutineStarted.Done()

		if err := s.grpcServer.Serve(listener); err != nil {
			klog.Fatalf("GRPC server failed: %v", err)
		}
	}()

	wgGoroutineStarted.Wait()

	return nil
}

// Stop terminates the gRPC server gracefully.
func (s *Server) Stop() {
	s.shuttingDown()
	s.grpcServer.GracefulStop()
}

// CSIDriverIsReady is used to notify the server that the CSI driver is available for use.
func (s *Server) CSIDriverIsReady() {
	s.setReady()
}
