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

	httpEndpoint = flag.String("http-endpoint", "", "The TCP network address where the HTTP server for diagnostics, including metrics and leader election health check, will listen (example: `:8080`). The default is empty string, which means the server is disabled. Only one of `--metrics-address` and `--http-endpoint` can be set.")
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

	rc := runtimeConfig{}

	if err := rc.kubeConnect(*kubeconfig, float32(*kubeAPIQPS), *kubeAPIBurst); err != nil {
		os.Exit(1)
	}

	if err := rc.csiConnect(*csiAddress); err != nil {
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *csiTimeout)
	defer cancel()

	if err := rc.csiCheckDriver(ctx); err != nil {
		os.Exit(1)
	}

	klog.V(2).Infof("CSI driver name: %q", rc.driverName)

}

type runtimeConfig struct {
	config     *rest.Config
	kubeClient *kubernetes.Clientset
	csiConn    *grpc.ClientConn
	driverName string
}

// kubeConnect creates the client config and creates the Kubernets client.
// It uses the specified kubeconfig if not empty, otherwise assume in-cluster.
func (rc *runtimeConfig) kubeConnect(kubeconfig string, kubeAPIQPS float32, kubeAPIBurst int) error {
	var (
		config *rest.Config
		err    error
	)

	if kubeconfig != "" {
		if config, err = clientcmd.BuildConfigFromFlags("", kubeconfig); err != nil {
			klog.Errorf("error building config: %v", err)
			return err
		}
	}

	if config, err = rest.InClusterConfig(); err != nil {
		klog.Errorf("error getting in-cluster config: %v", err)
		return err
	}

	config.QPS = kubeAPIQPS
	config.Burst = kubeAPIBurst

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Errorf("error getting client: %v", err)
		return err
	}

	rc.config = config
	rc.kubeClient = kubeClient

	return nil
}

// csiConnect establishes a connection to the CSI driver.
func (rc *runtimeConfig) csiConnect(csiAddress string) error {
	ctx := context.Background()

	metricsManager := metrics.NewCSIMetricsManager("" /* driverName */)
	csiConn, err := connection.Connect(
		ctx,
		csiAddress,
		metricsManager,
		connection.OnConnectionLoss(connection.ExitOnConnectionLoss()))
	if err != nil {
		klog.Errorf("error connecting to CSI driver: %v", err)
		return err
	}

	rc.csiConn = csiConn

	return nil
}

// csiCheckDriver obtains the CSI driver name and confirms that it supports the
// snapshot metadata service
func (rc *runtimeConfig) csiCheckDriver(ctx context.Context) error {
	// Find driver name
	driverName, err := csirpc.GetDriverName(ctx, rc.csiConn)
	if err != nil {
		klog.Errorf("error getting CSI driver name: %v", err)
		return err
	}

	// TODO
	// - Wait for driver to be ready

	pcs, err := csirpc.GetPluginCapabilities(ctx, rc.csiConn)
	if err != nil {
		klog.Errorf("error getting CSI plugin capabilities: %v", err)
		return err
	}

	// Require a release with the spec and csi-test updated
	// if _, found := pcs[csi.PluginCapability_Service_SNAPSHOT_METADATA_SERVICE]; !found {
	// 	klog.Errorf("CSI driver %s does not support the SNAPSHOT_METADATA_SERVICE", driverName)
	// 	return errors.New("SNAPSHOT_METADATA_SERVICE not supported")
	// }
	_ = pcs // fake a reference to compile

	rc.driverName = driverName

	return nil
}
