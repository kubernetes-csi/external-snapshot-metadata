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
	"sync"
	"testing"
	"time"

	cw "github.com/kubernetes-csi/external-snapshotter/v8/pkg/webhook"
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

		// Should fail to load the invalid cert
		cw, err := cw.NewCertWatcher(rt.TLSCertFile, rt.TLSKeyFile)
		assert.Error(t, err)
		assert.Nil(t, cw)

		// Show fail to start due to missing certwatcher
		server, err := NewServer(ServerConfig{Runtime: &rt, Certwatcher: cw})
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

		cw, err := cw.NewCertWatcher(rt.TLSCertFile, rt.TLSKeyFile)
		assert.NoError(t, err)
		assert.NotNil(t, cw)

		s, err := NewServer(ServerConfig{Runtime: &rt, Certwatcher: cw})
		assert.NoError(t, err)
		assert.NotNil(t, s)
		assert.NotNil(t, s.grpcServer)
		assert.Equal(t, s.config.Runtime, &rt)
		assert.Equal(t, HandlerDefaultMaxStreamDuration, s.config.MaxStreamDur)

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

		cw, err := cw.NewCertWatcher(rt.TLSCertFile, rt.TLSKeyFile)
		assert.NoError(t, err)
		assert.NotNil(t, cw)

		expMaxStreamDur := HandlerDefaultMaxStreamDuration + time.Minute
		s, err := NewServer(ServerConfig{
			Runtime:      &rt,
			MaxStreamDur: expMaxStreamDur,
			Certwatcher:  cw,
		})
		assert.NoError(t, err)
		assert.NotNil(t, s)
		assert.NotNil(t, s.grpcServer)
		assert.Equal(t, s.config.Runtime, &rt)
		assert.Equal(t, expMaxStreamDur, s.config.MaxStreamDur)
		assert.False(t, s.IsReady()) // initialized to not ready

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

		assert.False(t, s.IsReady()) // not yet ready
		err = s.isCSIDriverReady(context.Background())
		assert.Error(t, err)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unavailable, st.Code())
		assert.Equal(t, msgUnavailableCSIDriverNotReady, st.Message())

		s.CSIDriverIsReady()

		assert.True(t, s.IsReady()) // now ready!
		assert.NoError(t, s.isCSIDriverReady(context.Background()))

		s.Stop()
		assert.False(t, s.IsReady())
		assert.Error(t, s.isCSIDriverReady(context.Background()))
	})
}

func TestNewServerTLSConfig(t *testing.T) {
	t.Run("tls12-with-ciphers-and-curves", func(t *testing.T) {
		rth := runtime.NewTestHarness().WithTestTLSFiles(t)
		defer rth.RemoveTestTLSFiles(t)
		rta := rth.RuntimeArgs()

		rt := &runtime.Runtime{
			Args: runtime.Args{
				GRPCPort:            rta.GRPCPort,
				TLSCertFile:         rta.TLSCertFile,
				TLSKeyFile:          rta.TLSKeyFile,
				TLSMinVersion:       "VersionTLS12",
				TLSCipherSuites:     "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
				TLSCurvePreferences: "X25519,CurveP256",
			},
			DriverName: "driver",
			CBTClient:  fakecbt.NewSimpleClientset(),
			KubeClient: fake.NewSimpleClientset(),
		}

		cw, err := cw.NewCertWatcher(rt.TLSCertFile, rt.TLSKeyFile)
		assert.NoError(t, err)

		s, err := NewServer(ServerConfig{
			Runtime:     rt,
			Certwatcher: cw,
		})
		assert.NoError(t, err)
		assert.NotNil(t, s)

		tc := s.tlsCfg
		assert.NotNil(t, tc)
		assert.Equal(t, uint16(tls.VersionTLS12), tc.MinVersion)
		assert.Len(t, tc.CipherSuites, 1)
		assert.Len(t, tc.CurvePreferences, 2)
	})

	t.Run("tls13-without-ciphers", func(t *testing.T) {
		rth := runtime.NewTestHarness().WithTestTLSFiles(t)
		defer rth.RemoveTestTLSFiles(t)
		rta := rth.RuntimeArgs()

		rt := &runtime.Runtime{
			Args: runtime.Args{
				GRPCPort:            rta.GRPCPort,
				TLSCertFile:         rta.TLSCertFile,
				TLSKeyFile:          rta.TLSKeyFile,
				TLSMinVersion:       "VersionTLS13",
				TLSCurvePreferences: "X25519",
			},
			DriverName: "driver",
			CBTClient:  fakecbt.NewSimpleClientset(),
			KubeClient: fake.NewSimpleClientset(),
		}

		cw, err := cw.NewCertWatcher(rt.TLSCertFile, rt.TLSKeyFile)
		assert.NoError(t, err)

		s, err := NewServer(ServerConfig{
			Runtime:     rt,
			Certwatcher: cw,
		})
		assert.NoError(t, err)
		assert.NotNil(t, s)

		tc := s.tlsCfg
		assert.NotNil(t, tc)
		assert.Equal(t, uint16(tls.VersionTLS13), tc.MinVersion)
		assert.Nil(t, tc.CipherSuites)
		assert.Len(t, tc.CurvePreferences, 1)
		assert.Equal(t, tls.X25519, tc.CurvePreferences[0])
	})

	t.Run("invalid-cipher-suite", func(t *testing.T) {
		rth := runtime.NewTestHarness().WithTestTLSFiles(t)
		defer rth.RemoveTestTLSFiles(t)
		rta := rth.RuntimeArgs()

		rt := &runtime.Runtime{
			Args: runtime.Args{
				GRPCPort:        rta.GRPCPort,
				TLSCertFile:     rta.TLSCertFile,
				TLSKeyFile:      rta.TLSKeyFile,
				TLSCipherSuites: "INVALID_CIPHER",
			},
		}

		cw, err := cw.NewCertWatcher(rt.TLSCertFile, rt.TLSKeyFile)
		assert.NoError(t, err)

		s, err := NewServer(ServerConfig{
			Runtime:     rt,
			Certwatcher: cw,
		})
		assert.Error(t, err)
		assert.Nil(t, s)
		assert.Contains(t, err.Error(), "unsupported or insecure cipher suite")
	})

	t.Run("invalid-curve", func(t *testing.T) {
		rth := runtime.NewTestHarness().WithTestTLSFiles(t)
		defer rth.RemoveTestTLSFiles(t)
		rta := rth.RuntimeArgs()

		rt := &runtime.Runtime{
			Args: runtime.Args{
				GRPCPort:            rta.GRPCPort,
				TLSCertFile:         rta.TLSCertFile,
				TLSKeyFile:          rta.TLSKeyFile,
				TLSCurvePreferences: "InvalidCurve",
			},
		}

		cw, err := cw.NewCertWatcher(rt.TLSCertFile, rt.TLSKeyFile)
		assert.NoError(t, err)

		s, err := NewServer(ServerConfig{
			Runtime:     rt,
			Certwatcher: cw,
		})
		assert.Error(t, err)
		assert.Nil(t, s)
		assert.Contains(t, err.Error(), "unsupported curve")
	})

	t.Run("invalid-tls-version", func(t *testing.T) {
		rth := runtime.NewTestHarness().WithTestTLSFiles(t)
		defer rth.RemoveTestTLSFiles(t)
		rta := rth.RuntimeArgs()

		rt := &runtime.Runtime{
			Args: runtime.Args{
				GRPCPort:      rta.GRPCPort,
				TLSCertFile:   rta.TLSCertFile,
				TLSKeyFile:    rta.TLSKeyFile,
				TLSMinVersion: "TLS12",
			},
		}

		cw, err := cw.NewCertWatcher(rt.TLSCertFile, rt.TLSKeyFile)
		assert.NoError(t, err)

		s, err := NewServer(ServerConfig{
			Runtime:     rt,
			Certwatcher: cw,
		})
		assert.Error(t, err)
		assert.Nil(t, s)
		assert.Contains(t, err.Error(), "unsupported TLS version")
	})
}

func TestServerOperationID(t *testing.T) {
	s := &Server{}

	var (
		opName = "opName"
		numOps = 30
		opIDs  = sync.Map{}
		wg     sync.WaitGroup
	)

	wg.Add(numOps)

	for i := 0; i < numOps; i++ {
		go func() {
			op := s.OperationID(opName) // use the same operation name
			_, loaded := opIDs.LoadOrStore(op, struct{}{})
			assert.False(t, loaded)
			wg.Done()
		}()
	}

	wg.Wait()

	count := 0
	opIDs.Range(func(key, value any) bool {
		count++
		opID := key.(string)
		assert.Contains(t, opID, opName)
		return true
	})
	assert.Equal(t, numOps, count) // all ids distinct
}
