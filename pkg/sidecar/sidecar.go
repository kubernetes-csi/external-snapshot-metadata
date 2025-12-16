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
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"k8s.io/klog/v2"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/internal/runtime"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/internal/server/grpc"
)

const (
	defaultCSISocket               = "/run/csi/socket"
	defaultCSITimeout              = time.Minute // Default timeout of short CSI calls like GetPluginInfo.
	defaultGRPCPort                = 50051
	defaultHTTPEndpoint            = ":8080"
	defaultKubeAPIBurst            = 10
	defaultHealthPath              = "/health"
	defaultKubeAPIQPS              = 5.0
	defaultKubeconfig              = ""
	defaultMaxStreamingDurationMin = 10
	defaultMetricsPath             = "/metrics"

	flagCSIAddress              = "csi-address"
	flagCSITimeout              = "timeout"
	flagGRPCPort                = "port"
	flagHTTPEndpoint            = "http-endpoint"
	flagKubeAPIBurst            = "kube-api-burst"
	flagKubeAPIQPS              = "kube-api-qps"
	flagKubeconfig              = "kubeconfig"
	flagMaxStreamingDurationMin = "max-streaming-duration-min"
	flagMetricsPath             = "metrics-path"
	flagTLSCert                 = "tls-cert"
	flagTLSKey                  = "tls-key"
	flagVersion                 = "version"
	flagAudience                = "audience"
	flagDisableMetrics          = "disable-metrics"

	// tlsCertEnvVar is an environment variable that specifies the path to tls certificate file.
	tlsCertEnvVar = "TLS_CERT_PATH"
	// tlsKeyEnvVar is an environment variable that specifies the path to tls private key file.
	tlsKeyEnvVar = "TLS_KEY_PATH"
)

// Run contains the body of the sidecar.
// The function returns the process exit code.
func Run(argv []string, version string) int {
	s := newSidecarFlagSet(argv[0], version)

	handledShowVersion, err := s.parseFlagsAndHandleShowVersion(argv[1:])
	if err != nil {
		klog.Error(err)
		return 1
	}

	if handledShowVersion {
		return 0
	}

	klog.Infof("Version: %s", s.version)

	rt, err := runtime.New(s.runtimeArgsFromFlags())
	if err != nil {
		klog.Error(err)
		return 1
	}

	klog.Infof("CSI driver name: %q", rt.DriverName)

	grpcServer, err := startGRPCServerAndValidateCSIDriver(s.createServerConfig(rt))
	if err != nil {
		klog.Error(err)
		return 1
	}

	// Start the HTTP server for health and metrics endpoints.
	mux := http.NewServeMux()

	// Always register the health endpoint - tracks gRPC server readiness.
	mux.HandleFunc(defaultHealthPath, func(w http.ResponseWriter, r *http.Request) {
		if grpcServer.IsReady() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not ready"))
		}
	})

	// Conditionally register metrics endpoint.
	if !*s.disableMetrics {
		rt.MetricsManager.RegisterToServer(mux, *s.metricsPath)
		rt.MetricsManager.SetDriverName(rt.DriverName)
	}

	go func() {
		klog.Infof("HTTP server listening at %q (health: %s, metrics: %v)", *s.httpEndpoint, defaultHealthPath, !*s.disableMetrics)
		if err := http.ListenAndServe(*s.httpEndpoint, mux); err != nil {
			klog.Fatalf("Failed to start HTTP server at %q: %s", *s.httpEndpoint, err)
		}
	}()

	shutdownOnTerminationSignal(grpcServer)

	return 0
}

type sidecarFlagSet struct {
	*flag.FlagSet

	version string

	// flag variables
	csiAddress         *string
	csiTimeout         *time.Duration
	grpcPort           *int
	httpEndpoint       *string
	kubeAPIBurst       *int
	kubeAPIQPS         *float64
	kubeconfig         *string
	maxStreamingDurMin *int
	metricsPath        *string
	showVersion        *bool
	tlsCert            *string
	tlsKey             *string
	audience           *string
	disableMetrics     *bool
}

var sidecarFlagSetErrorHandling flag.ErrorHandling = flag.ExitOnError // UT interception point.

func newSidecarFlagSet(name, version string) *sidecarFlagSet {
	s := &sidecarFlagSet{
		FlagSet: flag.NewFlagSet(name, sidecarFlagSetErrorHandling),
		version: version,
	}

	// initialize flag variables
	s.kubeconfig = s.String(flagKubeconfig, defaultKubeconfig, "Absolute path to the kubeconfig file. Required only when running out of cluster.")
	s.csiAddress = s.String(flagCSIAddress, defaultCSISocket, "Address of the CSI driver socket.")
	s.showVersion = s.Bool(flagVersion, false, "Show version.")
	s.csiTimeout = s.Duration(flagCSITimeout, defaultCSITimeout, "The timeout for any RPCs to the CSI driver. Default is 1 minute.")

	s.grpcPort = s.Int(flagGRPCPort, defaultGRPCPort, "GRPC SnapshotMetadata service port number")
	s.tlsCert = s.String(flagTLSCert, os.Getenv(tlsCertEnvVar), "Path to the TLS certificate file. Can also be set with the environment variable "+tlsCertEnvVar+".")
	s.tlsKey = s.String(flagTLSKey, os.Getenv(tlsKeyEnvVar), "Path to the TLS private key file. Can also be set with the environment variable "+tlsKeyEnvVar+".")
	s.audience = s.String(flagAudience, "", "Audience string used for authentication.")

	s.maxStreamingDurMin = s.Int(flagMaxStreamingDurationMin, defaultMaxStreamingDurationMin, "The maximum duration in minutes for any individual streaming session")

	s.kubeAPIQPS = s.Float64(flagKubeAPIQPS, defaultKubeAPIQPS, "QPS to use while communicating with the kubernetes apiserver. Defaults to 5.0.")
	s.kubeAPIBurst = s.Int(flagKubeAPIBurst, defaultKubeAPIBurst, "Burst to use while communicating with the kubernetes apiserver. Defaults to 10.")

	s.httpEndpoint = s.String(flagHTTPEndpoint, defaultHTTPEndpoint,
		"The TCP network address where the HTTP server for diagnostics, including health check and metrics, will listen. Defaults to "+defaultHTTPEndpoint+".")
	s.metricsPath = s.String(flagMetricsPath, defaultMetricsPath, "The HTTP path where prometheus metrics will be exposed. Defaults to "+defaultMetricsPath+".")
	s.disableMetrics = s.Bool(flagDisableMetrics, false, "Disable the metrics endpoint. The health endpoint will still be available.")

	// K8s logging initialization
	klog.InitFlags(s.FlagSet)
	s.Set("logtostderr", "true")

	return s
}

// parseFlags parses the given command line vector (len > 0)
// returns an indication if it handled the "showVersion" flag.
func (s *sidecarFlagSet) parseFlagsAndHandleShowVersion(args []string) (handledShowVersion bool, err error) {
	if err := s.Parse(args); err != nil {
		return false, err
	}

	if *s.showVersion {
		fmt.Println(s.Name(), s.version)
		return true, nil
	}

	return false, nil
}

func (s *sidecarFlagSet) runtimeArgsFromFlags() runtime.Args {
	return runtime.Args{
		CSIAddress:   *s.csiAddress,
		CSITimeout:   *s.csiTimeout,
		KubeAPIBurst: *s.kubeAPIBurst,
		KubeAPIQPS:   (float32)(*s.kubeAPIQPS),
		Kubeconfig:   *s.kubeconfig,
		GRPCPort:     *s.grpcPort,
		TLSCertFile:  *s.tlsCert,
		TLSKeyFile:   *s.tlsKey,
		HttpEndpoint: *s.httpEndpoint,
		MetricsPath:  *s.metricsPath,
		Audience:     *s.audience,
	}
}

// runtimeArgsToArgv is provided for unit testing.
// Keep in sync with runtimeArgsFromFlags.
func (s *sidecarFlagSet) runtimeArgsToArgv(progName string, rta runtime.Args) []string {
	argv := []string{progName}

	if rta.Kubeconfig != "" {
		argv = append(argv, "-"+flagKubeconfig, rta.Kubeconfig)
	}

	if rta.CSIAddress != defaultCSISocket {
		argv = append(argv, "-"+flagCSIAddress, rta.CSIAddress)
	}

	if rta.CSITimeout != defaultCSITimeout {
		argv = append(argv, "-"+flagCSITimeout, rta.CSITimeout.String())
	}

	if rta.GRPCPort != defaultGRPCPort {
		argv = append(argv, "-"+flagGRPCPort, strconv.Itoa(rta.GRPCPort))
	}

	if rta.TLSCertFile != "" {
		argv = append(argv, "-"+flagTLSCert, rta.TLSCertFile)
	}

	if rta.TLSKeyFile != "" {
		argv = append(argv, "-"+flagTLSKey, rta.TLSKeyFile)
	}

	if rta.KubeAPIBurst != defaultKubeAPIBurst {
		argv = append(argv, "-"+flagKubeAPIBurst, strconv.Itoa(rta.KubeAPIBurst))
	}

	if rta.KubeAPIQPS != defaultKubeAPIQPS {
		argv = append(argv, "-"+flagKubeAPIQPS, strconv.FormatFloat(float64(rta.KubeAPIQPS), 'f', -1, 32))
	}

	if rta.HttpEndpoint != defaultHTTPEndpoint {
		argv = append(argv, "-"+flagHTTPEndpoint, rta.HttpEndpoint)
	}

	if rta.MetricsPath != defaultMetricsPath {
		argv = append(argv, "-"+flagMetricsPath, rta.MetricsPath)
	}

	return argv
}

func (s *sidecarFlagSet) createServerConfig(rt *runtime.Runtime) grpc.ServerConfig {
	return grpc.ServerConfig{
		Runtime:      rt,
		MaxStreamDur: time.Duration(*s.maxStreamingDurMin) * time.Minute,
	}
}

// startGRPCServerAndValidateCSIDriver starts the GRPC server and waits
// for it to validate the CSI driver capabilities.
func startGRPCServerAndValidateCSIDriver(config grpc.ServerConfig) (*grpc.Server, error) {
	rt := config.Runtime

	// create the GRPC server.
	grpcServer, err := grpc.NewServer(config)
	if err != nil {
		klog.Errorf("Failed to start GRPC server: %v", err)
		return nil, err
	}

	// Start the GRPC server. This call does not block but
	// arranges for the sidecar health to be exposed.
	// Metadata requests are not served until the CSI driver becomes ready.
	if err := grpcServer.Start(); err != nil {
		return nil, err
	}

	klog.Infof("GRPC server started listening on port %d", rt.GRPCPort)

	// check for a compatible CSI driver.
	if err := rt.WaitTillCSIDriverIsValidated(); err != nil {
		return nil, err
	}

	// notify the server that it can start processing metadata requests.
	grpcServer.CSIDriverIsReady()

	return grpcServer, nil
}

// Note: these are UNIX specific.
var terminationSignals = []os.Signal{syscall.SIGINT, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM}

func shutdownOnTerminationSignal(grpcServer *grpc.Server) {
	// Arrange for delivery of the termination signals.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, terminationSignals...)

	// wait until the signal is received.
	sigReceived := <-sigChan

	klog.Infof("Terminating on signal '%s' (%d)", sigReceived.String(), sigReceived)
	grpcServer.Stop()
	klog.Infof("Shutdown complete")
}
