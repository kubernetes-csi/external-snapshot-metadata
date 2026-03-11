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

package grpc

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/stretchr/testify/assert"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"
	"k8s.io/klog/v2"
)

func captureKlogOutput(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()

	var buf bytes.Buffer
	klog.LogToStderr(false)
	klog.SetOutput(&buf)

	return &buf, func() {
		klog.Flush()
		klog.SetOutput(os.Stderr)
		klog.LogToStderr(true)
	}
}

func TestKlogDoesNotLogSecurityToken(t *testing.T) {
	th := newTestHarness().WithFakeClientAPIs()
	s := th.ServerWithRuntime(t, th.Runtime())

	// Force TokenReviews.Create error to exercise logging.
	kubeClient := s.config.Runtime.KubeClient.(*fake.Clientset)
	kubeClient.PrependReactor("create", "tokenreviews", func(action clientgotesting.Action) (bool, apiruntime.Object, error) {
		return true, nil, errors.New("tokenreview-error")
	})

	buf, restore := captureKlogOutput(t)
	defer restore()

	securityToken := "super-secret-token"
	_, _, _ = s.authenticateRequest(context.Background(), securityToken)
	klog.Flush()

	logOutput := buf.String()
	assert.Contains(t, logOutput, "TokenReviews.Create")
	assert.NotContains(t, logOutput, securityToken)
}

func TestKlogDoesNotLogSecretData(t *testing.T) {
	th := newTestHarness().WithFakeClientAPIs()
	defer th.FakePopPrependedReactors()

	th.FakeKubeClient.PrependReactor("get", "secrets", func(action clientgotesting.Action) (bool, apiruntime.Object, error) {
		return true, nil, errors.New("secret-get-error")
	})

	s := th.ServerWithRuntime(t, th.Runtime())
	vsi := &volSnapshotInfo{
		DriverName: th.DriverName,
		VolumeSnapshot: &snapshotv1.VolumeSnapshot{
			Spec: snapshotv1.VolumeSnapshotSpec{
				VolumeSnapshotClassName: &th.VolumeSnapshotClassName,
			},
		},
	}

	buf, restore := captureKlogOutput(t)
	defer restore()

	_, _ = s.getSnapshotterCredentials(context.Background(), vsi)
	klog.Flush()

	logOutput := buf.String()
	assert.Contains(t, logOutput, msgUnavailableFailedToGetCredentials)
	assert.NotContains(t, logOutput, "user-id")
	assert.NotContains(t, logOutput, "user-key")
}
