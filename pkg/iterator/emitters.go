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

package iterator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// JSONEmitter formats the metadata in JSON.
type JSONEmitter struct {
	Writer       io.Writer
	listNotEmpty bool
}

func (e *JSONEmitter) SnapshotMetadataIteratorRecord(recordNumber int, metadata IteratorMetadata) bool {
	prefix := "," // termination of previous record
	if recordNumber == 1 {
		prefix = "[" // no previous, start of list
		e.listNotEmpty = true
	}

	var b bytes.Buffer
	_ = json.NewEncoder(&b).Encode(metadata)

	fmt.Fprintf(e.Writer, "%s%s", prefix, strings.TrimSuffix(b.String(), "\n"))

	return true
}

func (e *JSONEmitter) SnapshotMetadataIteratorDone(_ int) {
	if e.listNotEmpty {
		fmt.Fprintf(e.Writer, "]") // termination of previous, end of list
	} else {
		fmt.Fprintf(e.Writer, "[]") // empty list
	}
}

// TableEmitter formats the metadata as a table.
type TableEmitter struct {
	Writer io.Writer
}

const (
	// A TiB is this long: 1099511627776
	// BlockMetadataType is 15 chars max
	tableHeader1 = "Record#   VolCapBytes  BlockMetadataType   ByteOffset     SizeBytes"
	tableHeader2 = "------- -------------- ----------------- -------------- --------------"
	tableRowFmt  = "%7d %14d %17s %14d %14d\n"
)

func (e *TableEmitter) SnapshotMetadataIteratorRecord(recordNumber int, metadata IteratorMetadata) bool {
	if recordNumber == 1 {
		fmt.Fprintf(e.Writer, "%s\n%s\n", tableHeader1, tableHeader2)
	}

	bmt := metadata.BlockMetadataType.String()
	for _, bmd := range metadata.BlockMetadata {
		fmt.Fprintf(e.Writer, tableRowFmt, recordNumber, metadata.VolumeCapacityBytes, bmt, bmd.ByteOffset, bmd.SizeBytes)
	}

	return true
}

func (e *TableEmitter) SnapshotMetadataIteratorDone(_ int) {}
