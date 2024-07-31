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

package sidecar

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	klog "k8s.io/klog/v2"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/internal/runtime"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/internal/server/grpc"
)

const (
	// Default timeout of short CSI calls like GetPluginInfo.
	defaultCSITimeout = time.Minute

	// tlsCertEnvVar is an environment variable that specifies the path to tls certificate file.
	tlsCertEnvVar = "TLS_CERT_PATH"
	// tlsKeyEnvVar is an environment variable that specifies the path to tls private key file.
	tlsKeyEnvVar = "TLS_KEY_PATH"
)

// Command line flags.
var (
	kubeconfig  = flag.String("kubeconfig", "", "Absolute path to the kubeconfig file. Required only when running out of cluster.")
	csiAddress  = flag.String("csi-address", "/run/csi/socket", "Address of the CSI driver socket.")
	showVersion = flag.Bool("version", false, "Show version.")
	csiTimeout  = flag.Duration("timeout", defaultCSITimeout, "The timeout for any RPCs to the CSI driver. Default is 1 minute.")

	grpcPort = flag.Int("port", 50051, "gRPC SnapshotMetadata service port number")
	tlsCert  = flag.String("tls-cert", os.Getenv(tlsCertEnvVar), "Path to the TLS certificate file.")
	tlsKey   = flag.String("tls-key", os.Getenv(tlsKeyEnvVar), "Path to the TLS private key file.")

	kubeAPIQPS   = flag.Float64("kube-api-qps", 5, "QPS to use while communicating with the kubernetes apiserver. Defaults to 5.0.")
	kubeAPIBurst = flag.Int("kube-api-burst", 10, "Burst to use while communicating with the kubernetes apiserver. Defaults to 10.")

	httpEndpoint = flag.String("http-endpoint", "", "The TCP network address where the HTTP server for diagnostics, including metrics and leader election health check, will listen (example: `:8080`). The default is empty string, which means the server is disabled.")
	metricsPath  = flag.String("metrics-path", "/metrics", "The HTTP path where prometheus metrics will be exposed. Default is `/metrics`.")
)

// Run contains the body of the sidecar.
// The function returns the process exit code.
func Run(version string) int {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	flag.Parse()

	if *showVersion {
		fmt.Println(os.Args[0], version)
		return 0
	}

	klog.Infof("Version: %s", version)

	// create the runtime clients.
	// TODO: set up the HTTP server.
	rt, err := runtime.New(runtime.Args{
		CSIAddress:   *csiAddress,
		CSITimeout:   *csiTimeout,
		KubeAPIBurst: *kubeAPIBurst,
		KubeAPIQPS:   (float32)(*kubeAPIQPS),
		Kubeconfig:   *kubeconfig,
		GRPCPort:     *grpcPort,
		TLSCertFile:  *tlsCert,
		TLSKeyFile:   *tlsKey,
	})
	if err != nil {
		klog.Error(err)
		return 1
	}

	klog.Infof("CSI driver name: %q", rt.DriverName)

	// TBD May need to exposed metric HTTP end point
	// here because the wait for the CSI driver is open ended.

	// Create the gRPC server. It won't start serving metadata until
	// the CSIDriverIsReady method is called.
	grpcServer, err := grpc.NewServer(grpc.ServerConfig{Runtime: rt})
	if err != nil {
		klog.Fatalf("Failed to start GRPC server: %v", err)
	}

	// Start the gRPC server. This call does not block but
	// arranges for the sidecar health to be exposed.
	// Metadata requests are not served until the driver is ready.
	grpcServer.Start()

	// check for a compatible CSI driver.
	if err := rt.WaitTillCSIDriverIsValidated(); err != nil {
		klog.Error(err)
		return 1
	}

	grpcServer.CSIDriverIsReady() // start serving metadata.

	// Arrange for delivery of the termination signal.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	// wait until the signal is received.
	sigReceived := <-sigChan

	klog.Infof("Terminating on signal '%s' (%d)", sigReceived.String(), sigReceived)
	grpcServer.Stop()
	klog.Infof("Shutdown complete")

	return 0
}
