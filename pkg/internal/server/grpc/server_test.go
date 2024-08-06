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
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/fake"

	fakecbt "github.com/kubernetes-csi/external-snapshot-metadata/client/clientset/versioned/fake"
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
		th := runtime.NewTestHarness().WithTestTLSFiles(t)
		defer th.RemoveTestTLSFiles(t)
		rta := th.RuntimeArgs()

		rt := *validConfig.Runtime // copy
		rt.TLSCertFile = rta.TLSCertFile
		rt.TLSKeyFile = rta.TLSKeyFile + "foo" // invalid path

		server, err := NewServer(ServerConfig{Runtime: &rt})
		assert.Error(t, err)
		assert.Nil(t, server)
	})

	t.Run("listen-error", func(t *testing.T) {
		th := runtime.NewTestHarness().WithTestTLSFiles(t)
		defer th.RemoveTestTLSFiles(t)
		rta := th.RuntimeArgs()

		rt := *validConfig.Runtime // copy
		rt.TLSCertFile = rta.TLSCertFile
		rt.TLSKeyFile = rta.TLSKeyFile
		rt.GRPCPort = -1 // invalid port

		s, err := NewServer(ServerConfig{Runtime: &rt})
		assert.NoError(t, err)
		assert.NotNil(t, s)
		assert.NotNil(t, s.grpcServer)
		assert.Equal(t, s.config.Runtime, &rt)
		assert.NotNil(t, s.sigChan)
		assert.NotNil(t, s.startedChan)

		err = s.Start()
		assert.Error(t, err)
	})

	t.Run("start-stop", func(t *testing.T) {
		th := runtime.NewTestHarness().WithTestTLSFiles(t)
		defer th.RemoveTestTLSFiles(t)
		rta := th.RuntimeArgs()

		rt := *validConfig.Runtime // copy
		rt.TLSCertFile = rta.TLSCertFile
		rt.TLSKeyFile = rta.TLSKeyFile

		s, err := NewServer(ServerConfig{Runtime: &rt})
		assert.NoError(t, err)
		assert.NotNil(t, s)
		assert.NotNil(t, s.grpcServer)
		assert.Equal(t, s.config.Runtime, &rt)
		assert.NotNil(t, s.sigChan)
		assert.NotNil(t, s.startedChan)

		err = s.Start()
		assert.NoError(t, err)

		// wait till the server goroutine has started
		<-s.startedChan

		// send a signal to the process to terminate the server
		go func() {
			time.Sleep(10 * time.Millisecond)
			syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
		}()

		s.WaitForTermination()
	})
}
