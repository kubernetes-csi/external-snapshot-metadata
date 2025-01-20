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

package runtime

import (
	"time"

	"k8s.io/klog/v2"
)

const (
	LabelTargetSnapshotName = "target_snapshot"
	LabelBaseSnapshotName   = "base_snapshot"
	SubSystem               = "snapshot_metadata_controller"

	// MetadataAllocatedOperationName is the operation that tracks how long the controller takes to get the allocated blocks for a snapshot.
	// Specifically, the operation metric is emitted based on the following timestamps:
	// - Start_time: controller notices the first time that there is a GetMetadataAllocated RPC call to fetch the allocated blocks of metadata
	// - End_time:   controller notices that the RPC call is finished and the allocated blocks is streamed back to the driver
	MetadataAllocatedOperationName = "MetadataAllocated"

	// MetadataDeltaOperationName is the operation that tracks how long the controller takes to get the changed blocks between 2 snapshots
	// Specifically, the operation metric is emitted based on the following timestamps:
	// - Start_time: controller notices the first time that there is a GetMetadataDelta RPC call to fetch the changed blocks between 2 snapshots
	// - End_time:   controller notices that the RPC call is finished and the changed blocks is streamed back to the driver
	MetadataDeltaOperationName = "MetadataDelta"
)

// RecordMetricsWithLabels is a wrapper on the csi-lib-utils RecordMetrics function, that calls the
// "RecordMetrics" functions with the necessary labels added to the MetricsManager runtime.
func (rt *Runtime) RecordMetricsWithLabels(opLabel map[string]string, opName string, startTime time.Time, opErr error) {
	metricsWithLabel, err := rt.MetricsManager.WithLabelValues(opLabel)
	if err != nil {
		klog.Error(err, "failed to add labels to metrics")
		return
	}

	opDuration := time.Since(startTime)
	metricsWithLabel.RecordMetrics(opName, opErr, opDuration)
}
