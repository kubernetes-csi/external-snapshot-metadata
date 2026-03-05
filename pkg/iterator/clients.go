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
	"fmt"

	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	smsCR "github.com/kubernetes-csi/external-snapshot-metadata/client/clientset/versioned"
)

type Clients struct {
	KubeClient     kubernetes.Interface
	SnapshotClient snapshot.Interface
	SmsCRClient    smsCR.Interface
}

func (c Clients) Validate() error {
	switch {
	case c.KubeClient == nil:
		return fmt.Errorf("%w: missing KubeClient", ErrInvalidArgs)
	case c.SnapshotClient == nil:
		return fmt.Errorf("%w: missing SnapshotClient", ErrInvalidArgs)
	case c.SmsCRClient == nil:
		return fmt.Errorf("%w: missing SmsCRClient", ErrInvalidArgs)
	}

	return nil
}

// BuildClients constructs the necessary client interfaces from the given configuration.
func BuildClients(config *rest.Config) (Clients, error) {
	var clients Clients

	if err := clients.makeAllClients(config); err != nil {
		return Clients{}, err
	}

	return clients, nil
}

func (c *Clients) makeAllClients(config *rest.Config) error {
	var err error

	if c.KubeClient == nil {
		if c.KubeClient, err = c.kubeClient(config); err != nil {
			return err
		}
	}

	if c.SnapshotClient == nil {
		if c.SnapshotClient, err = c.snapshotClient(config); err != nil {
			return err
		}
	}

	if c.SmsCRClient == nil {
		if c.SmsCRClient, err = c.smsCRClient(config); err != nil {
			return err
		}
	}

	return nil
}

func (c Clients) kubeClient(config *rest.Config) (kubernetes.Interface, error) {
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("kubernetes.NewForConfig: %w", err)
	}

	return kubeClient, nil
}

func (c Clients) snapshotClient(config *rest.Config) (snapshot.Interface, error) {
	snapshotClient, err := snapshot.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("snapshot.NewForConfig: %w", err)
	}

	return snapshotClient, nil
}

func (c Clients) smsCRClient(config *rest.Config) (smsCR.Interface, error) {
	smsCRClient, err := smsCR.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("SnapshotMetadataServiceCR.NewForConfig: %w", err)
	}

	return smsCRClient, nil
}
