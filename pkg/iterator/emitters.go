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
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
)

// JSONEmitter formats the metadata in JSON.
type JSONEmitter struct {
	Writer       io.Writer
	listNotEmpty bool
}

func (e *JSONEmitter) SnapshotMetadataIteratorRecord(recordNumber int, metadata IteratorMetadata) error {
	prefix := "," // termination of previous record
	if recordNumber == 1 {
		prefix = "[" // no previous, start of list
		e.listNotEmpty = true
	}

	var b bytes.Buffer
	_ = json.NewEncoder(&b).Encode(metadata)

	fmt.Fprintf(e.Writer, "%s%s", prefix, strings.TrimSuffix(b.String(), "\n"))

	return nil
}

func (e *JSONEmitter) SnapshotMetadataIteratorDone(_ int) error {
	if e.listNotEmpty {
		fmt.Fprintf(e.Writer, "]") // termination of previous, end of list
	} else {
		fmt.Fprintf(e.Writer, "[]") // empty list
	}

	return nil
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

func (e *TableEmitter) SnapshotMetadataIteratorRecord(recordNumber int, metadata IteratorMetadata) error {
	if recordNumber == 1 {
		fmt.Fprintf(e.Writer, "%s\n%s\n", tableHeader1, tableHeader2)
	}

	bmt := metadata.BlockMetadataType.String()
	for _, bmd := range metadata.BlockMetadata {
		fmt.Fprintf(e.Writer, tableRowFmt, recordNumber, metadata.VolumeCapacityBytes, bmt, bmd.ByteOffset, bmd.SizeBytes)
	}

	return nil
}

func (e *TableEmitter) SnapshotMetadataIteratorDone(_ int) error {
	return nil
}

// VerifierEmitter will write the changed blocks from the source device to the target device at the designated offset and
// compare the contents of the source and target devices at the end.
type VerifierEmitter struct {
	// SourceDevice contains the source device file descriptor.
	SourceDevice *os.File

	// TargetDevice contains the target device file descriptor.
	TargetDevice *os.File
}

const chunkSize = int64(1024 * 1024) // 1 MiB
var (
	ErrFailedToSeek       = errors.New("failed to seek")
	ErrContentsDoNotMatch = errors.New("source and target device contents do not match")
)

// copyBlock will copy chunks of 1 MiB at a time upto to sizeBytes from the source device to the target device.
func (verifierEmitter *VerifierEmitter) copyBlock(bmd *api.BlockMetadata) error {
	// Seek to the byteOffset of the source device.
	_, err := verifierEmitter.SourceDevice.Seek(bmd.ByteOffset, io.SeekStart)
	if err != nil {
		return fmt.Errorf("%w (source device(%q), offset(%d)): %w", ErrFailedToSeek, verifierEmitter.SourceDevice.Name(), bmd.ByteOffset, err)
	}

	// Seek to the byteOffset of the target device.
	_, err = verifierEmitter.TargetDevice.Seek(bmd.ByteOffset, io.SeekStart)
	if err != nil {
		return fmt.Errorf("%w (target device(%q), offset(%d)): %w", ErrFailedToSeek, verifierEmitter.TargetDevice.Name(), bmd.ByteOffset, err)
	}

	buffer := make([]byte, chunkSize)
	for written := int64(0); written < bmd.SizeBytes; {
		remaining := bmd.SizeBytes - written
		if remaining < chunkSize {
			buffer = make([]byte, remaining)
		}

		// Read a chunk from the source device.
		sourceBytes, sourceErr := verifierEmitter.SourceDevice.Read(buffer)
		if sourceErr != nil && sourceErr != io.EOF {
			return fmt.Errorf("failed to read from source device(%q): %w", verifierEmitter.SourceDevice.Name(), sourceErr)
		}

		// Write the chunk to the target device.
		_, targetErr := verifierEmitter.TargetDevice.Write(buffer[:sourceBytes])
		if targetErr != nil {
			return fmt.Errorf("failed to write to target device(%q): %w", verifierEmitter.TargetDevice.Name(), targetErr)
		}

		written += int64(sourceBytes)
		if sourceErr == io.EOF {
			break
		}
	}

	return nil
}

// SnapshotMetadataIteratorRecord will write the block from the source device to the target device at the designated offset.
func (verifierEmitter *VerifierEmitter) SnapshotMetadataIteratorRecord(_ int, metadata IteratorMetadata) error {
	for _, bmd := range metadata.BlockMetadata {
		if err := verifierEmitter.copyBlock(bmd); err != nil {
			return err
		}
	}

	return nil
}

// SnapshotMetadataIteratorDone will compare the contents of the source and target devices.
func (verifierEmitter *VerifierEmitter) SnapshotMetadataIteratorDone(_ int) error {
	// Seek to the start of the source and target devices.
	_, err := verifierEmitter.SourceDevice.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("%w (source device(%q), offset(%d)): %w", ErrFailedToSeek, verifierEmitter.SourceDevice.Name(), io.SeekStart, err)
	}
	_, err = verifierEmitter.TargetDevice.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("%w (target device(%q), offset(%d)): %w", ErrFailedToSeek, verifierEmitter.TargetDevice.Name(), io.SeekStart, err)
	}

	sourceBuffer := make([]byte, chunkSize)
	targetBuffer := make([]byte, chunkSize)
	for {
		// Read a chunk from the source and target devices.
		sourceBytes, sourceErr := verifierEmitter.SourceDevice.Read(sourceBuffer)
		targetBytes, targetErr := verifierEmitter.TargetDevice.Read(targetBuffer)

		if sourceErr != nil || targetErr != nil {
			if sourceErr == io.EOF && targetErr == io.EOF {
				// Both devices have been read completely.
				return nil
			} else if sourceErr == io.EOF || targetErr == io.EOF {
				// One device has been read completely but the other has not.
				return ErrContentsDoNotMatch
			} else {
				// An error occurred while reading from both devices.
				return fmt.Errorf("error reading source and target device contents: source(%q) target(%q)", sourceErr, targetErr)
			}
		}

		if (sourceBytes != targetBytes) || !bytes.Equal(sourceBuffer[:sourceBytes], targetBuffer[:targetBytes]) {
			return ErrContentsDoNotMatch
		}
	}
}
