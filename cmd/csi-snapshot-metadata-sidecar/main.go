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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	klog "k8s.io/klog/v2"

	"github.com/kubernetes-csi/csi-lib-utils/connection"
	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	csirpc "github.com/kubernetes-csi/csi-lib-utils/rpc"
)

const (
	// Default timeout of short CSI calls like GetPluginInfo.
	defaultCSITimeout = time.Minute
)

// Command line flags.
var (
	kubeconfig  = flag.String("kubeconfig", "", "Absolute path to the kubeconfig file. Required only when running out of cluster.")
	csiAddress  = flag.String("csi-address", "/run/csi/socket", "Address of the CSI driver socket.")
	showVersion = flag.Bool("version", false, "Show version.")
	csiTimeout  = flag.Duration("timeout", defaultCSITimeout, "The timeout for any RPCs to the CSI driver. Default is 1 minute.")

	kubeAPIQPS   = flag.Float64("kube-api-qps", 5, "QPS to use while communicating with the kubernetes apiserver. Defaults to 5.0.")
	kubeAPIBurst = flag.Int("kube-api-burst", 10, "Burst to use while communicating with the kubernetes apiserver. Defaults to 10.")

	httpEndpoint = flag.String("http-endpoint", "", "The TCP network address where the HTTP server for diagnostics, including metrics and leader election health check, will listen (example: `:8080`). The default is empty string, which means the server is disabled.")
	metricsPath  = flag.String("metrics-path", "/metrics", "The HTTP path where prometheus metrics will be exposed. Default is `/metrics`.")
)

var (
	version = "unknown"
)

func main() {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	flag.Parse()

	if *showVersion {
		fmt.Println(os.Args[0], version)
		os.Exit(0)
	}

	klog.Infof("Version: %s", version)

	ss := SidecarService{}

	if err := ss.ClientInit(*kubeconfig, float32(*kubeAPIQPS), *kubeAPIBurst, *csiAddress); err != nil {
		klog.Error(err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *csiTimeout)
	defer cancel()

	if err := ss.CSICheckDriver(ctx, ss.CSIConn); err != nil {
		klog.Error(err)
		os.Exit(1)
	}

	klog.Infof("CSI driver name: %q", ss.DriverName)

	// TBD: initialize and start the SnapshotMetadata service
}

type SidecarService struct {
	Config     *rest.Config
	KubeClient *kubernetes.Clientset
	CSIConn    *grpc.ClientConn
	DriverName string
}

// ClientInit creates a K8s client and also connects to the CSI driver.
func (ss *SidecarService) ClientInit(
	kubeconfig string,
	kubeAPIQPS float32,
	kubeAPIBurst int,
	csiAddress string,
) error {
	if err := ss.kubeConnect(kubeconfig, kubeAPIQPS, kubeAPIBurst); err != nil {
		return err
	}

	if err := ss.csiConnect(csiAddress); err != nil {
		return err
	}

	return nil
}

// kubeConnect creates the client config and creates the Kubernets client.
// It uses the specified kubeconfig if not empty, otherwise assumes an in-cluster invocation.
func (ss *SidecarService) kubeConnect(kubeconfig string, kubeAPIQPS float32, kubeAPIBurst int) error {
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
		return fmt.Errorf("error creating kube client: %w", err)
	}

	ss.Config = config
	ss.KubeClient = kubeClient

	return nil
}

// csiConnect establishes a connection to the CSI driver.
func (ss *SidecarService) csiConnect(csiAddress string) error {
	ctx := context.Background()

	metricsManager := metrics.NewCSIMetricsManager("" /* driverName */)
	csiConn, err := connection.Connect(
		ctx,
		csiAddress,
		metricsManager,
		connection.OnConnectionLoss(connection.ExitOnConnectionLoss()))
	if err != nil {
		return fmt.Errorf("error connecting to CSI driver: %w", err)
	}

	ss.CSIConn = csiConn

	return nil
}

// CSICheckDriver obtains the CSI driver name and confirms that it supports the
// snapshot metadata service.
func (ss *SidecarService) CSICheckDriver(ctx context.Context, csiConn *grpc.ClientConn) error {
	// Find driver name
	driverName, err := csirpc.GetDriverName(ctx, csiConn)
	if err != nil {
		return fmt.Errorf("error getting CSI driver name: %w", err)
	}

	// TODO
	// - Wait for driver to be ready

	pcs, err := csirpc.GetPluginCapabilities(ctx, csiConn)
	if err != nil {
		return fmt.Errorf("error getting CSI plugin capabilities: %w", err)
	}

	// TODO: Require a release with the spec and csi-test updated to uncomment this code.
	// if _, found := pcs[csi.PluginCapability_Service_SNAPSHOT_METADATA_SERVICE]; !found {
	// 	klog.Errorf("CSI driver %s does not support the SNAPSHOT_METADATA_SERVICE", driverName)
	// 	return errors.New("SNAPSHOT_METADATA_SERVICE not supported")
	// }
	_ = pcs // fake a reference to compile

	ss.DriverName = driverName

	return nil
}
