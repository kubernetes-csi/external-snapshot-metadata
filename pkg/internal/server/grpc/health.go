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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	klog "k8s.io/klog/v2"
)

func newHealthServer() *health.Server {
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)

	return healthServer
}

// isReady indicates whether the sidecar can serve metadata.
func (s *Server) isReady() bool {
	resp, err := s.healthServer.Check(context.Background(), &healthpb.HealthCheckRequest{})
	if err == nil && resp.Status == healthpb.HealthCheckResponse_SERVING {
		return true
	}

	return false
}

// setReady should be called when the sidecar is ready to serve metadata.
func (s *Server) setReady() {
	klog.Info("Setting status to SERVING")
	s.healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
}

func (s *Server) setNotReady() {
	klog.Info("Setting status to NOT_SERVING")
	s.healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
}

// shuttingDown transitions to the NotReady state and removes the ability to change state.
func (s *Server) shuttingDown() {
	klog.Info("Shutting down: setting status to NOT_SERVING")
	s.healthServer.Shutdown()
}

// isCSIDriverReady is a helper for the handlers that returns the appropriate error if the
// CSI driver is not ready.
func (s *Server) isCSIDriverReady() error {
	if s.isReady() {
		return nil
	}

	return status.Errorf(codes.Unavailable, msgUnavailableCSIDriverNotReady)
}
