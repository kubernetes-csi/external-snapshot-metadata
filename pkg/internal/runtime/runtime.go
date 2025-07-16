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

package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/connection"
	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	csirpc "github.com/kubernetes-csi/csi-lib-utils/rpc"
	snapshot "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	cbt "github.com/kubernetes-csi/external-snapshot-metadata/client/clientset/versioned"
)

type Args struct {
	// Address of the CSI driver socket.
	CSIAddress string
	// CSITimeout is the timeout for CSI driver communications.
	CSITimeout time.Duration
	// Burst for the K8s apiserver.
	KubeAPIBurst int
	// QPS for the K8s apiserver.
	KubeAPIQPS float32
	// Absolute path to a kubeconfig file if operating out of cluster.
	Kubeconfig string
	// GRPC port number
	GRPCPort int
	// Absolute path to the TLS cert file.
	TLSCertFile string
	// Absolute path to the TLS key file.
	TLSKeyFile string
	// HttpEndpoint is the address of the metrics sever
	HttpEndpoint string
	// MetricsPath is the path where metrics will be recorded
	MetricsPath string
	// Audience string is used for authentication.
	Audience string
}

func (args *Args) Validate() error {
	switch {
	case args.CSIAddress == "":
		return errors.New("CSIAddress is required")
	case args.CSITimeout == 0:
		return errors.New("CSITimeout is required")
	case args.GRPCPort <= 0:
		return errors.New("invalid GRPCPort")
	case args.TLSCertFile == "":
		return errors.New("missing TLSCertFile")
	case args.TLSKeyFile == "":
		return errors.New("missing TLSKeyFile")
	}

	return nil
}

func New(args Args) (*Runtime, error) {
	if err := args.Validate(); err != nil {
		return nil, err
	}

	rt := &Runtime{Args: args}

	if err := rt.initialize(); err != nil {
		return nil, err
	}

	return rt, nil
}

// Runtime contains client connection objects needed for the sidecar.
type Runtime struct {
	Args

	Config         *rest.Config
	KubeClient     kubernetes.Interface
	CBTClient      cbt.Interface
	SnapshotClient snapshot.Interface
	CSIConn        *grpc.ClientConn
	MetricsManager metrics.CSIMetricsManager
	DriverName     string
	Audience       string
}

// initialize obtains the clients and then the CSI driver name.
func (rt *Runtime) initialize() error {
	if err := rt.kubeConnect(rt.Kubeconfig, rt.KubeAPIQPS, rt.KubeAPIBurst); err != nil {
		return err
	}

	if err := rt.csiConnect(rt.CSIAddress); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), rt.CSITimeout)
	defer cancel()

	driverName, err := csirpc.GetDriverName(ctx, rt.CSIConn)
	if err != nil {
		return fmt.Errorf("error getting CSI driver name: %w", err)
	}

	rt.DriverName = driverName

	return nil
}

// kubeConnect creates the client config and creates the Kubernetes client.
// It uses the specified kubeconfig if not empty, otherwise assumes an in-cluster invocation.
func (rt *Runtime) kubeConnect(kubeconfig string, kubeAPIQPS float32, kubeAPIBurst int) error {
	var (
		config *rest.Config
		err    error
	)

	if kubeconfig != "" {
		if config, err = clientcmd.BuildConfigFromFlags("", kubeconfig); err != nil {
			return fmt.Errorf("error in kubeconfig: %w", err)
		}
	} else {
		if config, err = rest.InClusterConfig(); err != nil {
			return fmt.Errorf("error in cluster configuration: %w", err)
		}
	}

	config.QPS = kubeAPIQPS
	config.Burst = kubeAPIBurst

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("error creating kubernetes client: %w", err)
	}

	cbtClient, err := cbt.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("error creating kubernetes client for cbt resources: %w", err)
	}

	snapshotClient, err := snapshot.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("error creating kubernetes client for snapshot resources: %w", err)
	}

	rt.Config = config
	rt.KubeClient = kubeClient
	rt.CBTClient = cbtClient
	rt.SnapshotClient = snapshotClient

	return nil
}

// csiConnect establishes a connection to the CSI driver.
func (rt *Runtime) csiConnect(csiAddress string) error {
	ctx := context.Background()

	metricsManager := metrics.NewCSIMetricsManagerWithOptions("",
		metrics.WithSubsystem(SubSystem),
		metrics.WithLabelNames(LabelTargetSnapshotName, LabelBaseSnapshotName))
	csiConn, err := connection.Connect(
		ctx,
		csiAddress,
		metricsManager,
		connection.OnConnectionLoss(connection.ExitOnConnectionLoss()))
	if err != nil {
		return fmt.Errorf("error connecting to CSI driver: %w", err)
	}

	rt.CSIConn = csiConn
	rt.MetricsManager = metricsManager

	return nil
}

// WaitTillCSIDriverIsValidated waits until the CSI driver becomes ready, and
// then confirms that it supports the snapshot metadata service.
func (rt *Runtime) WaitTillCSIDriverIsValidated() error {
	ctx := context.Background()

	if err := csirpc.ProbeForever(ctx, rt.CSIConn, rt.CSITimeout); err != nil {
		return fmt.Errorf("error waiting for CSI driver to become ready: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), rt.CSITimeout)
	defer cancel()

	pcs, err := csirpc.GetPluginCapabilities(ctx, rt.CSIConn)
	if err != nil {
		return fmt.Errorf("error getting CSI plugin capabilities: %w", err)
	}

	if _, found := pcs[csi.PluginCapability_Service_SNAPSHOT_METADATA_SERVICE]; !found {
		return fmt.Errorf("CSI driver %s does not support the SNAPSHOT_METADATA_SERVICE", rt.DriverName)
	}

	return nil
}
