/*
Copyright 2025 The Kubernetes Authors.

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
	"os/signal"
	"path/filepath"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/iterator"
)

const (
	shortUsageFmt = `Usage:

1. Verify allocated block metadata of a snapshot

   %[1]s -n Namespace -s Snapshot -src /dev/source -tgt /dev/target [Additional flags ...]

2. Verify changed block metadata between two snapshots

   %[1]s -n Namespace -s Snapshot -p PreviousSnapshot -src /dev/source -tgt /dev/target [Additional flags ...]

3. Display the full help message

   %[1]s -h
`
	usageFmt = `This command verifies allocated blocks of a VolumeSnapshot object.
If a previous VolumeSnapshot object is also specified then changed blocks between
the two snapshots, which must both be from the same PersistentVolume, are verified.

The command is usually invoked in a Pod in the cluster, as the gRPC client
needs to resolve the DNS address in the SnapshotMetadataService CR.

` + shortUsageFmt + `

Flags:
`
)

// globals set by flags
var (
	args                               iterator.Args
	kubeConfig                         string
	sourceDevicePath, targetDevicePath string
)

func parseFlags() {
	stringFlag := func(sVar *string, longName, shortName, defaultValue, description string) {
		flag.StringVar(sVar, longName, defaultValue, description)
		flag.StringVar(sVar, shortName, defaultValue, fmt.Sprintf("Shorthand for -%s", longName))
	}

	stringFlag(&args.Namespace, "namespace", "n", "", "The Namespace containing the VolumeSnapshot objects.")
	stringFlag(&args.SnapshotName, "snapshot", "s", "", "The name of the VolumeSnapshot for which metadata is to be displayed.")
	stringFlag(&args.PrevSnapshotName, "previous-snapshot", "p", "", "The name of an earlier VolumeSnapshot against which changed block metadata is to be displayed.")
	stringFlag(&sourceDevicePath, "source-device-path", "src", "", "Path of the source device. This device should be the PVC in block mode restored from the snapshot which is passed as the '-snapshot' flag.")
	stringFlag(&targetDevicePath, "target-device-path", "tgt", "", "Path of the target device. This device should be a PVC in block mode restored from the snapshot which is passed as the '-previous-snapshot' flag or a fresh PVC in block mode in case the flag is not passed.")

	if home := homedir.HomeDir(); home != "" {
		flag.StringVar(&kubeConfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "Path to the kubeconfig file.")
	} else {
		flag.StringVar(&kubeConfig, "kubeconfig", "", "Path to the kubeconfig file.")
	}

	flag.StringVar(&args.SAName, "service-account", "", "ServiceAccount used to create a security token. If unspecified the ServiceAccount of the Pod in which the command is invoked will be used.")
	flag.StringVar(&args.SANamespace, "service-account-namespace", "", "Namespace of the ServiceAccount used to create a security token. If unspecified the Namespace of the Pod in which the command is invoked will be used.")

	flag.Int64Var(&args.TokenExpirySecs, "token-expiry", 600, "Expiry time in seconds for the security token.")
	flag.Int64Var(&args.StartingOffset, "starting-offset", 0, "The starting byte offset.")

	var maxResults int
	flag.IntVar(&maxResults, "max-results", 0, "The maximum results per record.")

	var showHelp bool
	flag.BoolVar(&showHelp, "h", false, "Show the full usage message.")

	progName := filepath.Base(os.Args[0])
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "\n"+shortUsageFmt, progName)
	}

	if len(os.Args) > 1 {
		flag.Parse()
		args.MaxResults = int32(maxResults)
	} else {
		fmt.Fprintf(os.Stderr, "Missing required arguments\n")
		fmt.Fprintf(os.Stderr, shortUsageFmt, progName)
		os.Exit(1)
	}

	if showHelp {
		fmt.Fprintf(os.Stderr, usageFmt, progName)
		flag.PrintDefaults()
		os.Exit(0)
	}

	if sourceDevicePath == "" || targetDevicePath == "" {
		fmt.Fprintf(os.Stderr, "Missing required arguments\n")
		flag.Usage()
		os.Exit(1)
	}
}

func main() {
	// parse flags and set the output emitter
	parseFlags()

	// get the K8s config from either kubeConfig, in-cluster or default
	config, err := buildConfig(kubeConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading kubeconfig %s: %v\n", kubeConfig, err)
		os.Exit(1)
	}

	clients, err := iterator.BuildClients(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating clients: %v\n", err)
		os.Exit(1)
	}

	args.Clients = clients

	ctx, stopFn := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopFn()

	sourceDevice, err := os.Open(sourceDevicePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open source device %s: %q", sourceDevicePath, err)
		os.Exit(1)
	}
	defer sourceDevice.Close()

	targetDevice, err := os.OpenFile(targetDevicePath, os.O_RDWR, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open target device %s: %q", targetDevicePath, err)
		os.Exit(1)
	}
	defer targetDevice.Close()

	args.Emitter = &iterator.VerifierEmitter{
		SourceDevice: sourceDevice,
		TargetDevice: targetDevice,
	}

	if err := iterator.GetSnapshotMetadata(ctx, args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}

func buildConfig(kubeconfigPath string) (*rest.Config, error) {
	// If kubeconfig exists, try from kubeconfig file
	if _, err := os.Stat(kubeconfigPath); err == nil {
		return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}
	// try in-cluster config
	return rest.InClusterConfig()
}
