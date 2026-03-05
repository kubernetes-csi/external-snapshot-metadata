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

package iterator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/internal/runtime"
)

func TestBuildClients(t *testing.T) {
	rth := runtime.NewTestHarness().WithFakeKubeConfig(t)
	defer rth.RemoveFakeKubeConfig(t)

	config, err := clientcmd.BuildConfigFromFlags("", rth.FakeKubeConfigFileName())
	assert.NoError(t, err)

	var goodClient Clients

	t.Run("success", func(t *testing.T) {
		clients, err := BuildClients(config)
		assert.NoError(t, err)

		assert.NotNil(t, clients.KubeClient)
		assert.NotNil(t, clients.SnapshotClient)
		assert.NotNil(t, clients.SmsCRClient)

		goodClient = clients
	})

	badConfig := *config
	badConfig.QPS = 3    // these combinations
	badConfig.Burst = -1 // are invalid

	t.Run("k8s-new-for-config-error", func(t *testing.T) {
		// kubeClient is the first client constructor
		client, err := BuildClients(&badConfig) // test the error path
		assert.Error(t, err)
		assert.ErrorContains(t, err, "kubernetes.NewForConfig")
		assert.Nil(t, client.KubeClient)
		assert.Nil(t, client.SnapshotClient)
		assert.Nil(t, client.SmsCRClient)
	})

	// test the remaining client constructors individually

	t.Run("snapshot-new-for-config-error", func(t *testing.T) {
		// snapshotClient is the second client constructor
		c := &Clients{}
		c.KubeClient = goodClient.KubeClient // first client

		err := c.makeAllClients(&badConfig)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "snapshot.NewForConfig")
		assert.Nil(t, c.SnapshotClient)
	})

	t.Run("smsCR-new-for-config-error", func(t *testing.T) {
		// SmsCRClient is the third client constructor
		c := &Clients{}
		c.KubeClient = goodClient.KubeClient         // first client
		c.SnapshotClient = goodClient.SnapshotClient // second client

		err := c.makeAllClients(&badConfig)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "SnapshotMetadataServiceCR.NewForConfig")
		assert.Nil(t, c.SmsCRClient)
	})
}
