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
	"os"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/sidecar"
)

var (
	// Version gets set via the https://pkg.go.dev/cmd/link -X flag in the build scripts.
	version = "unknown"
)

func main() {
	rc := sidecar.Run(os.Args, version)

	os.Exit(rc)
}
