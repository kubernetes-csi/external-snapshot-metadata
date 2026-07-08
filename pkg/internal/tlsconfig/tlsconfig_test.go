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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTLSVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    uint16
		wantErr bool
	}{
		{name: "empty", input: "", want: 0},
		{name: "tls12", input: "VersionTLS12", want: tls.VersionTLS12},
		{name: "tls13", input: "VersionTLS13", want: tls.VersionTLS13},
		{name: "invalid", input: "TLS12", wantErr: true},
		{name: "tls10-rejected", input: "VersionTLS10", wantErr: true},
		{name: "tls11-rejected", input: "VersionTLS11", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseTLSVersion(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestParseCipherSuites(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
		wantErr bool
	}{
		{name: "empty", input: "", wantLen: 0},
		{name: "single", input: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", wantLen: 1},
		{name: "multiple", input: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384", wantLen: 2},
		{name: "with-spaces", input: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 , TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384", wantLen: 2},
		{name: "trailing-comma", input: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,", wantLen: 1},
		{name: "comma-only", input: ",", wantLen: 0},
		{name: "spaces-and-commas", input: ", ,", wantLen: 0},
		{name: "invalid", input: "FAKE_CIPHER", wantErr: true},
		{name: "insecure-rejected", input: "TLS_RSA_WITH_RC4_128_SHA", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseCipherSuites(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, got, tc.wantLen)
			}
		})
	}
}

func TestParseCurvePreferences(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []tls.CurveID
		wantErr bool
	}{
		{name: "empty", input: "", want: nil},
		{name: "x25519", input: "X25519", want: []tls.CurveID{tls.X25519}},
		{name: "multiple", input: "X25519,CurveP256,CurveP384", want: []tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384}},
		{name: "p521", input: "CurveP521", want: []tls.CurveID{tls.CurveP521}},
		{name: "mlkem768", input: "X25519MLKEM768", want: []tls.CurveID{tls.X25519MLKEM768}},
		{name: "with-spaces", input: "X25519 , CurveP256", want: []tls.CurveID{tls.X25519, tls.CurveP256}},
		{name: "comma-only", input: ",", want: nil},
		{name: "invalid", input: "InvalidCurve", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseCurvePreferences(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestSupportedCurves(t *testing.T) {
	expected := []string{"X25519", "CurveP256", "CurveP384", "CurveP521", "X25519MLKEM768"}
	for _, name := range expected {
		_, ok := tlsCurves[name]
		assert.True(t, ok, "expected curve %q in tlsCurves map", name)
	}
	assert.Equal(t, len(expected), len(tlsCurves))
}
