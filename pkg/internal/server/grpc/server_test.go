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
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"k8s.io/client-go/kubernetes/fake"

	fakecbt "github.com/kubernetes-csi/external-snapshot-metadata/client/clientset/versioned/fake"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/internal/runtime"
)

func TestNewServer(t *testing.T) {
	validConfig := ServerConfig{
		Runtime: &runtime.Runtime{
			Args: runtime.Args{
				GRPCPort:    5001,
				TLSCertFile: "certFile",
				TLSKeyFile:  "keyFile",
			},
			DriverName: "driver",
			CBTClient:  fakecbt.NewSimpleClientset(),
			KubeClient: fake.NewSimpleClientset(),
		},
	}

	t.Run("tls-load-error", func(t *testing.T) {
		rth := runtime.NewTestHarness().WithTestTLSFiles(t)
		defer rth.RemoveTestTLSFiles(t)
		rta := rth.RuntimeArgs()

		rt := *validConfig.Runtime // copy
		rt.TLSCertFile = rta.TLSCertFile
		rt.TLSKeyFile = rta.TLSKeyFile + "foo" // invalid path

		server, err := NewServer(ServerConfig{Runtime: &rt})
		assert.Error(t, err)
		assert.Nil(t, server)
	})

	t.Run("listen-error", func(t *testing.T) {
		rth := runtime.NewTestHarness().WithTestTLSFiles(t)
		defer rth.RemoveTestTLSFiles(t)
		rta := rth.RuntimeArgs()

		rt := *validConfig.Runtime // copy
		rt.TLSCertFile = rta.TLSCertFile
		rt.TLSKeyFile = rta.TLSKeyFile
		rt.GRPCPort = -1 // invalid port

		s, err := NewServer(ServerConfig{Runtime: &rt})
		assert.NoError(t, err)
		assert.NotNil(t, s)
		assert.NotNil(t, s.grpcServer)
		assert.Equal(t, s.config.Runtime, &rt)

		err = s.Start()
		assert.Error(t, err)
	})

	t.Run("start-stop", func(t *testing.T) {
		rth := runtime.NewTestHarness().WithTestTLSFiles(t)
		defer rth.RemoveTestTLSFiles(t)
		rta := rth.RuntimeArgs()

		rt := *validConfig.Runtime // copy
		rt.TLSCertFile = rta.TLSCertFile
		rt.TLSKeyFile = rta.TLSKeyFile

		s, err := NewServer(ServerConfig{Runtime: &rt})
		assert.NoError(t, err)
		assert.NotNil(t, s)
		assert.NotNil(t, s.grpcServer)
		assert.Equal(t, s.config.Runtime, &rt)
		assert.False(t, s.isReady()) // initialized to not ready

		err = s.Start()
		assert.NoError(t, err)

		// check that services are registered
		si := s.grpcServer.GetServiceInfo()
		for _, serviceName := range []string{
			healthpb.Health_ServiceDesc.ServiceName,
			api.SnapshotMetadata_ServiceDesc.ServiceName,
		} {
			assert.Contains(t, si, serviceName)
		}

		assert.False(t, s.isReady()) // not yet ready
		err = s.isCSIDriverReady()
		assert.Error(t, err)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unavailable, st.Code())
		assert.Equal(t, msgUnavailableCSIDriverNotReady, st.Message())

		s.CSIDriverIsReady()

		assert.True(t, s.isReady()) // now ready!
		assert.NoError(t, s.isCSIDriverReady())

		s.Stop()
		assert.False(t, s.isReady())
		assert.Error(t, s.isCSIDriverReady())
	})
}
