/*
Copyright 2026 The Kubernetes Authors.

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

package tlsconfig

import (
	"crypto/tls"
	"fmt"
	"sort"
	"strings"
)

var tlsVersions = map[string]uint16{
	"VersionTLS12": tls.VersionTLS12,
	"VersionTLS13": tls.VersionTLS13,
}

var tlsCurves = map[string]tls.CurveID{
	"X25519":         tls.X25519,
	"CurveP256":      tls.CurveP256,
	"CurveP384":      tls.CurveP384,
	"CurveP521":      tls.CurveP521,
	"X25519MLKEM768": tls.X25519MLKEM768,
}

// ParseTLSVersion converts a TLS version string to its uint16 constant.
// Returns 0 for empty input (Go default). Accepts "VersionTLS12", "VersionTLS13".
func ParseTLSVersion(s string) (uint16, error) {
	if s == "" {
		return 0, nil
	}

	v, ok := tlsVersions[s]
	if !ok {
		return 0, fmt.Errorf("unsupported TLS version %q, must be one of: VersionTLS12, VersionTLS13", s)
	}

	return v, nil
}

// ParseCipherSuites converts a comma-separated list of Go cipher suite names to their uint16 IDs.
// Returns nil for empty input (Go defaults). Only accepts safe suites from tls.CipherSuites().
func ParseCipherSuites(csv string) ([]uint16, error) {
	if csv == "" {
		return nil, nil
	}

	cipherSuiteMap := make(map[string]uint16, len(tls.CipherSuites()))
	for _, cs := range tls.CipherSuites() {
		cipherSuiteMap[cs.Name] = cs.ID
	}

	names := strings.Split(csv, ",")
	suites := make([]uint16, 0, len(names))

	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		id, ok := cipherSuiteMap[name]
		if !ok {
			return nil, fmt.Errorf("unsupported or insecure cipher suite: %q", name)
		}

		suites = append(suites, id)
	}

	if len(suites) == 0 {
		return nil, nil
	}

	return suites, nil
}

// ParseCurvePreferences converts a comma-separated list of curve names to tls.CurveID values.
// Returns nil for empty input (Go defaults).
// Supported: X25519, CurveP256, CurveP384, CurveP521, X25519MLKEM768.
func ParseCurvePreferences(csv string) ([]tls.CurveID, error) {
	if csv == "" {
		return nil, nil
	}

	names := strings.Split(csv, ",")
	curves := make([]tls.CurveID, 0, len(names))

	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		id, ok := tlsCurves[name]
		if !ok {
			return nil, fmt.Errorf("unsupported curve %q, must be one of: %s", name, supportedCurveNames())
		}

		curves = append(curves, id)
	}

	if len(curves) == 0 {
		return nil, nil
	}

	return curves, nil
}

func supportedCurveNames() string {
	names := make([]string, 0, len(tlsCurves))
	for k := range tlsCurves {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
