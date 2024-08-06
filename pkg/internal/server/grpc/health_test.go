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
	"testing"

	"github.com/stretchr/testify/assert"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestHealthService(t *testing.T) {
	t.Run("state-transitions", func(t *testing.T) {
		th := newTestHarness()
		server := th.ServerWithClientAPIs()

		assert.False(t, server.isReady())

		server.setReady()
		assert.True(t, server.isReady())

		server.setNotReady()
		assert.False(t, server.isReady())

		server.setReady()
		assert.True(t, server.isReady())

		server.shuttingDown()
		assert.False(t, server.isReady())

		server.setReady()
		assert.False(t, server.isReady()) // cannot change state once shutdown.
	})

	t.Run("client-health-checks", func(t *testing.T) {
		th := newTestHarness()
		server := th.StartGRPCServer(t)
		defer th.StopGRPCServer(t)

		client := th.GRPCHealthClient(t)
		ctx := context.Background()

		resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, resp.Status)

		server.CSIDriverIsReady()

		resp, err = client.Check(ctx, &healthpb.HealthCheckRequest{})
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.Status)

		server.setNotReady()

		resp, err = client.Check(ctx, &healthpb.HealthCheckRequest{})
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, resp.Status)

		server.setReady()

		resp, err = client.Check(ctx, &healthpb.HealthCheckRequest{})
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.Status)

		server.shuttingDown()

		resp, err = client.Check(ctx, &healthpb.HealthCheckRequest{})
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, resp.Status)
	})
}
