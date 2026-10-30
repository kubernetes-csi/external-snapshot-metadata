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

package iterator

import (
	"context"
	"testing"

	fakesnapshot "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
)

func TestGetPrevSnapshotIDStatus(t *testing.T) {
	for _, tc := range []struct {
		name          string
		missingStatus string
		wantError     string
	}{
		{
			name:          "snapshot status not set",
			missingStatus: "snapshot",
			wantError:     "boundVolumeSnapshotContentName not set",
		},
		{
			name:          "content status not set",
			missingStatus: "content",
			wantError:     "snapshot handle not set",
		},
		{
			name: "ready-to-use not set",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			th := newTestHarness()
			th.SnapshotName = th.PrevSnapshotName
			vs, vsc := th.FakeVS()
			switch tc.missingStatus {
			case "snapshot":
				vs.Status = nil
			case "content":
				vsc.Status = nil
			}

			args := th.Args()
			args.SnapshotName = "current-snapshot"
			args.SnapshotClient = fakesnapshot.NewSimpleClientset(vs, vsc)
			if !assert.NoError(t, args.Validate()) {
				return
			}

			// Use the real helpers that resolve PrevSnapshotName to a CSI handle.
			iter := newIterator(args)
			var snapshotID string
			var err error
			if !assert.NotPanics(t, func() {
				snapshotID, err = iter.getPrevSnapshotID(context.Background())
			}) {
				return
			}

			if tc.wantError != "" {
				assert.ErrorContains(t, err, tc.wantError)
				assert.Empty(t, snapshotID)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, th.PrevSnapshotHandle, snapshotID)
			}
		})
	}
}
