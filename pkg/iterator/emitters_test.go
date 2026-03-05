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
	"crypto/rand"
	"math/big"
	"os"
	"testing"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
	"github.com/stretchr/testify/assert"
)

func TestJSONEmitter(t *testing.T) {
	var b bytes.Buffer
	e := &JSONEmitter{Writer: &b}

	t.Run("no records", func(t *testing.T) {
		b.Reset()
		e.SnapshotMetadataIteratorDone(100) // count doesn't matter
		assert.Equal(t, b.String(), "[]")
	})

	t.Run("one record", func(t *testing.T) {
		b.Reset()
		e.SnapshotMetadataIteratorRecord(1, IteratorMetadata{
			BlockMetadataType:   api.BlockMetadataType_FIXED_LENGTH,
			VolumeCapacityBytes: 100000,
			BlockMetadata: []*api.BlockMetadata{
				{
					ByteOffset: 0,
					SizeBytes:  1000,
				},
				{
					ByteOffset: 5000,
					SizeBytes:  1000,
				},
			},
		})
		e.SnapshotMetadataIteratorDone(200) // count doesn't matter
		exp := `[{"block_metadata_type":1,"volume_capacity_bytes":100000,"block_metadata":[{"size_bytes":1000},{"byte_offset":5000,"size_bytes":1000}]}]`
		assert.Equal(t, exp, b.String())
	})

	t.Run("multiple records", func(t *testing.T) {
		b.Reset()
		e.SnapshotMetadataIteratorRecord(1, IteratorMetadata{
			BlockMetadataType:   api.BlockMetadataType_FIXED_LENGTH,
			VolumeCapacityBytes: 100000,
			BlockMetadata: []*api.BlockMetadata{
				{
					ByteOffset: 0,
					SizeBytes:  1000,
				},
			},
		})
		e.SnapshotMetadataIteratorRecord(2, IteratorMetadata{
			BlockMetadataType:   api.BlockMetadataType_FIXED_LENGTH,
			VolumeCapacityBytes: 100000,
			BlockMetadata: []*api.BlockMetadata{
				{
					ByteOffset: 5000,
					SizeBytes:  1000,
				},
			},
		})
		e.SnapshotMetadataIteratorDone(2) // count doesn't matter
		exp := `[{"block_metadata_type":1,"volume_capacity_bytes":100000,"block_metadata":[{"size_bytes":1000}]},{"block_metadata_type":1,"volume_capacity_bytes":100000,"block_metadata":[{"byte_offset":5000,"size_bytes":1000}]}]`
		assert.Equal(t, exp, b.String())
	})
}

func TestTableEmitter(t *testing.T) {
	var b bytes.Buffer
	e := &TableEmitter{Writer: &b}

	oneTiB := int64(10 * 1099511627776)

	t.Run("no records", func(t *testing.T) {
		b.Reset()
		e.SnapshotMetadataIteratorDone(100) // count doesn't matter
		assert.Equal(t, b.String(), "")
	})

	t.Run("one record", func(t *testing.T) {
		b.Reset()
		e.SnapshotMetadataIteratorRecord(1, IteratorMetadata{
			BlockMetadataType:   api.BlockMetadataType_FIXED_LENGTH,
			VolumeCapacityBytes: oneTiB,
			BlockMetadata: []*api.BlockMetadata{
				{
					ByteOffset: 0,
					SizeBytes:  1000,
				},
				{
					ByteOffset: 5000,
					SizeBytes:  1000,
				},
			},
		})
		e.SnapshotMetadataIteratorDone(200) // count doesn't matter
		exp := `Record#   VolCapBytes  BlockMetadataType   ByteOffset     SizeBytes
------- -------------- ----------------- -------------- --------------
      1 10995116277760      FIXED_LENGTH              0           1000
      1 10995116277760      FIXED_LENGTH           5000           1000
`
		assert.Equal(t, exp, b.String())
	})

	t.Run("multiple records", func(t *testing.T) {
		b.Reset()
		e.SnapshotMetadataIteratorRecord(1, IteratorMetadata{
			BlockMetadataType:   api.BlockMetadataType_VARIABLE_LENGTH,
			VolumeCapacityBytes: 100000,
			BlockMetadata: []*api.BlockMetadata{
				{
					ByteOffset: 0,
					SizeBytes:  8000,
				},
			},
		})
		e.SnapshotMetadataIteratorRecord(2, IteratorMetadata{
			BlockMetadataType:   api.BlockMetadataType_VARIABLE_LENGTH,
			VolumeCapacityBytes: 100000,
			BlockMetadata: []*api.BlockMetadata{
				{
					ByteOffset: 300000,
					SizeBytes:  1000,
				},
				{
					ByteOffset: 700000,
					SizeBytes:  10000000,
				},
			},
		})
		e.SnapshotMetadataIteratorDone(2) // count doesn't matter
		exp := `Record#   VolCapBytes  BlockMetadataType   ByteOffset     SizeBytes
------- -------------- ----------------- -------------- --------------
      1         100000   VARIABLE_LENGTH              0           8000
      2         100000   VARIABLE_LENGTH         300000           1000
      2         100000   VARIABLE_LENGTH         700000       10000000
`
		assert.Equal(t, exp, b.String())
	})
}

func TestVerifierEmitter(t *testing.T) {
	capacity := 1024 * 1024
	dataBuffer := make([]byte, capacity)
	tmpDir := t.TempDir()
	sourceDevice, err := os.CreateTemp(tmpDir, "sourceDevice")
	assert.NoError(t, err)
	_, err = sourceDevice.Write(dataBuffer)
	assert.NoError(t, err)

	targetDevice, err := os.CreateTemp(tmpDir, "targetDevice")
	assert.NoError(t, err)
	_, err = targetDevice.Write(dataBuffer)
	assert.NoError(t, err)

	smallerDevice, err := os.CreateTemp(tmpDir, "smallerDevice")
	assert.NoError(t, err)
	_, err = smallerDevice.Write(dataBuffer[:capacity/2])
	assert.NoError(t, err)

	t.Run("no records", func(t *testing.T) {
		e := &VerifierEmitter{
			SourceDevice: sourceDevice,
			TargetDevice: targetDevice,
		}

		err := e.SnapshotMetadataIteratorDone(100) // count doesn't matter
		assert.NoError(t, err)
	})

	t.Run("one record", func(t *testing.T) {
		sizeBytes := 1111
		byteOffset := 0
		e := &VerifierEmitter{
			SourceDevice: sourceDevice,
			TargetDevice: targetDevice,
		}
		writeRandomData(t, sourceDevice, sizeBytes, byteOffset)

		// data contents should not match
		err = e.SnapshotMetadataIteratorDone(100) // count doesn't matter
		assert.ErrorIs(t, err, ErrContentsDoNotMatch)

		// trigger copy of changed blocks
		err = e.SnapshotMetadataIteratorRecord(1, IteratorMetadata{
			BlockMetadataType:   api.BlockMetadataType_FIXED_LENGTH,
			VolumeCapacityBytes: int64(capacity),
			BlockMetadata: []*api.BlockMetadata{
				{
					ByteOffset: int64(byteOffset),
					SizeBytes:  int64(sizeBytes),
				},
			},
		})
		assert.NoError(t, err)

		// data contents should match
		err = e.SnapshotMetadataIteratorDone(100) // count doesn't matter
		assert.NoError(t, err)
	})

	t.Run("multiple records", func(t *testing.T) {
		blockMetadataList := []*api.BlockMetadata{}
		bytesOffset := 0
		sizeBytes := 0
		e := &VerifierEmitter{
			SourceDevice: sourceDevice,
			TargetDevice: targetDevice,
		}
		for i := 0; i < 10; i++ {
			randOffset, err := rand.Int(rand.Reader, big.NewInt(1000))
			assert.NoError(t, err)
			randBytesSize, err := rand.Int(rand.Reader, big.NewInt(1000))
			assert.NoError(t, err)
			bytesOffset = bytesOffset + sizeBytes + int(randOffset.Int64())
			sizeBytes = int(randBytesSize.Int64())
			writeRandomData(t, sourceDevice, sizeBytes, bytesOffset)
			blockMetadataList = append(blockMetadataList, &api.BlockMetadata{
				ByteOffset: int64(bytesOffset),
				SizeBytes:  int64(sizeBytes),
			})
		}
		// data contents should not match
		err = e.SnapshotMetadataIteratorDone(100) // count doesn't matter
		assert.ErrorIs(t, err, ErrContentsDoNotMatch)

		// trigger copy of changed blocks
		err = e.SnapshotMetadataIteratorRecord(1, IteratorMetadata{
			BlockMetadataType:   api.BlockMetadataType_FIXED_LENGTH,
			VolumeCapacityBytes: int64(capacity),
			BlockMetadata:       blockMetadataList,
		})
		assert.NoError(t, err)

		// data contents should match
		err = e.SnapshotMetadataIteratorDone(100) // count doesn't matter
		assert.NoError(t, err)

	})

	t.Run("source device is smaller than target device", func(t *testing.T) {
		e := &VerifierEmitter{
			SourceDevice: smallerDevice,
			TargetDevice: targetDevice,
		}
		err = e.SnapshotMetadataIteratorDone(100) // count doesn't matter
		assert.ErrorIs(t, err, ErrContentsDoNotMatch)
	})

	t.Run("target device is smaller than source device", func(t *testing.T) {
		e := &VerifierEmitter{
			SourceDevice: sourceDevice,
			TargetDevice: smallerDevice,
		}
		err = e.SnapshotMetadataIteratorDone(100) // count doesn't matter
		assert.ErrorIs(t, err, ErrContentsDoNotMatch)
	})

	t.Run("closed device is passed as source device", func(t *testing.T) {
		assert.Nil(t, smallerDevice.Close())
		e := &VerifierEmitter{
			SourceDevice: smallerDevice,
			TargetDevice: targetDevice,
		}
		err = e.SnapshotMetadataIteratorRecord(1, IteratorMetadata{
			BlockMetadataType:   api.BlockMetadataType_FIXED_LENGTH,
			VolumeCapacityBytes: int64(capacity),
			BlockMetadata: []*api.BlockMetadata{
				{
					ByteOffset: 0,
					SizeBytes:  1000,
				},
			},
		})
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrFailedToSeek)
		assert.Contains(t, err.Error(), "file already closed")

		err := e.SnapshotMetadataIteratorDone(100) // count doesn't matter
		assert.ErrorIs(t, err, ErrFailedToSeek)
		assert.Contains(t, err.Error(), "file already closed")
	})

	t.Run("closed device is passed as target device", func(t *testing.T) {
		e := &VerifierEmitter{
			SourceDevice: sourceDevice,
			TargetDevice: smallerDevice,
		}
		err = e.SnapshotMetadataIteratorRecord(1, IteratorMetadata{
			BlockMetadataType:   api.BlockMetadataType_FIXED_LENGTH,
			VolumeCapacityBytes: int64(capacity),
			BlockMetadata: []*api.BlockMetadata{
				{
					ByteOffset: 0,
					SizeBytes:  1000,
				},
			},
		})
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrFailedToSeek)
		assert.Contains(t, err.Error(), "file already closed")

		err = e.SnapshotMetadataIteratorDone(100) // count doesn't matter
		assert.ErrorIs(t, err, ErrFailedToSeek)
		assert.Contains(t, err.Error(), "file already closed")
	})

}

func writeRandomData(t *testing.T, file *os.File, sizeBytes, byteOffset int) {
	dataBuffer := make([]byte, sizeBytes)
	// generate random data into dataBuffer
	_, err := rand.Read(dataBuffer)
	assert.NoError(t, err)
	_, err = file.WriteAt(dataBuffer, int64(byteOffset))
	assert.NoError(t, err)
}
