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
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	cw "github.com/kubernetes-csi/external-snapshotter/v8/pkg/webhook"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	cbt "github.com/kubernetes-csi/external-snapshot-metadata/client/clientset/versioned"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/internal/runtime"
)

const (
	HandlerTraceLogLevel         = 4
	HandlerDetailedTraceLogLevel = 5

	HandlerDefaultMaxStreamDuration = time.Minute * 10
)

type ServerConfig struct {
	Runtime *runtime.Runtime

	// The maximum duration of a streaming session.
	// The handler will abort if either the CSI driver or
	// the client do not complete in this time.
	// If not set then HandlerDefaultMaxStreamDuration is used.
	MaxStreamDur time.Duration

	Certwatcher *cw.CertWatcher
}

type Server struct {
	api.UnimplementedSnapshotMetadataServer

	config       ServerConfig
	grpcServer   *grpc.Server
	healthServer *health.Server
	opNumber     int64
}

func NewServer(config ServerConfig) (*Server, error) {
	if config.MaxStreamDur <= 0 {
		config.MaxStreamDur = HandlerDefaultMaxStreamDuration
	}

	if config.Certwatcher == nil {
		return nil, errors.New("the certificate watcher/provider for the gRPC server is unset.")
	}

	return &Server{
		config: config,
		grpcServer: grpc.NewServer(
			grpc.Creds(
				credentials.NewTLS(
					&tls.Config{
						GetCertificate: config.Certwatcher.GetCertificate,
						ClientAuth:     tls.NoClientCert,
					},
				),
			),
		),
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

func (s *Server) audience() string {
	return s.config.Runtime.Audience
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

// OperationID generates a unique identifier for an operation.
func (s *Server) OperationID(op string) string {
	return fmt.Sprintf("%s-%d", op, atomic.AddInt64(&s.opNumber, 1))
}
