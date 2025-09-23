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

package grpc

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/internal/runtime"
)

func (s *Server) GetMetadataDelta(req *api.GetMetadataDeltaRequest, stream api.SnapshotMetadata_GetMetadataDeltaServer) (err error) {
	// Create a timeout context so that failure in either sending to the client or
	// receiving from the CSI driver will ultimately abort the handler session.
	// The context could also get canceled by the client.
	ctx, cancelFn := context.WithTimeout(s.getMetadataDeltaContextWithLogger(req, stream), s.config.MaxStreamDur)
	defer cancelFn()

	// Record metrics when the operation ends
	defer func(startTime time.Time) {
		opLabel := map[string]string{
			runtime.LabelTargetSnapshotName: fmt.Sprintf("%s/%s", req.Namespace, req.TargetSnapshotName),
			runtime.LabelBaseSnapshotID:     req.BaseSnapshotId,
		}
		s.config.Runtime.RecordMetricsWithLabels(opLabel, runtime.MetadataAllocatedOperationName, startTime, err)
	}(time.Now())

	if err := s.validateGetMetadataDeltaRequest(req); err != nil {
		klog.FromContext(ctx).Error(err, "validation failed")
		return err
	}

	if err := s.authenticateAndAuthorize(ctx, req.SecurityToken, req.Namespace); err != nil {
		return err
	}

	if err := s.isCSIDriverReady(ctx); err != nil {
		return err
	}

	csiReq, err := s.convertToCSIGetMetadataDeltaRequest(ctx, req)
	if err != nil {
		return err
	}

	// Invoke the CSI Driver's GetMetadataDelta gRPC and stream the response back to client
	klog.FromContext(ctx).V(HandlerTraceLogLevel).Info("calling CSI driver", "baseSnapshotId", csiReq.BaseSnapshotId, "targetSnapshotId", csiReq.TargetSnapshotId)
	csiStream, err := csi.NewSnapshotMetadataClient(s.csiConnection()).GetMetadataDelta(ctx, csiReq)
	if err != nil {
		klog.FromContext(ctx).Error(err, "csi.GetMetadataDelta")
		return err
	}

	err = s.streamGetMetadataDeltaResponse(ctx, stream, csiStream)
	return err
}

func (s *Server) getMetadataDeltaContextWithLogger(req *api.GetMetadataDeltaRequest, stream api.SnapshotMetadata_GetMetadataDeltaServer) context.Context {
	return klog.NewContext(stream.Context(),
		klog.LoggerWithValues(klog.Background(),
			"op", s.OperationID("GetMetadataDelta"),
			"namespace", req.Namespace,
			"baseSnapshotId", req.BaseSnapshotId,
			"targetSnapshotName", req.TargetSnapshotName,
			"startingOffset", req.StartingOffset,
			"maxResults", req.MaxResults,
		))
}

func (s *Server) validateGetMetadataDeltaRequest(req *api.GetMetadataDeltaRequest) error {
	if len(req.GetSecurityToken()) == 0 {
		return status.Errorf(codes.InvalidArgument, msgInvalidArgumentSecurityTokenMissing)
	}

	if len(req.GetNamespace()) == 0 {
		return status.Errorf(codes.InvalidArgument, msgInvalidArgumentNamespaceMissing)
	}

	if len(req.GetBaseSnapshotId()) == 0 {
		return status.Errorf(codes.InvalidArgument, msgInvalidArgumentBaseSnapshotIdMissing)
	}

	if len(req.GetTargetSnapshotName()) == 0 {
		return status.Errorf(codes.InvalidArgument, msgInvalidArgumentTargetSnapshotNameMissing)
	}

	return nil
}

func (s *Server) convertToCSIGetMetadataDeltaRequest(ctx context.Context, req *api.GetMetadataDeltaRequest) (*csi.GetMetadataDeltaRequest, error) {
	vsiTarget, err := s.getVolSnapshotInfo(ctx, req.Namespace, req.TargetSnapshotName)
	if err != nil {
		return nil, err
	}

	if vsiTarget.DriverName != s.driverName() {
		err = status.Errorf(codes.InvalidArgument, msgInvalidArgumentSnaphotDriverInvalidFmt, req.TargetSnapshotName, s.driverName())
		klog.FromContext(ctx).Error(err, "invalid driver")
		return nil, err
	}

	secretsMap, err := s.getSnapshotterCredentials(ctx, vsiTarget)
	if err != nil {
		return nil, err
	}

	return &csi.GetMetadataDeltaRequest{
		BaseSnapshotId:   req.BaseSnapshotId,
		TargetSnapshotId: vsiTarget.SnapshotHandle,
		StartingOffset:   req.StartingOffset,
		MaxResults:       req.MaxResults,
		Secrets:          secretsMap,
	}, nil
}

func (s *Server) streamGetMetadataDeltaResponse(ctx context.Context, clientStream api.SnapshotMetadata_GetMetadataDeltaServer, csiStream csi.SnapshotMetadata_GetMetadataDeltaClient) error { //nolint:dupl
	var (
		blockMetadataType   api.BlockMetadataType
		lastByteOffset      int64
		lastSize            int64
		logger              = klog.FromContext(ctx)
		numBlockMetadata    int
		responseNum         int
		volumeCapacityBytes int64
	)

	for {
		csiResp, err := csiStream.Recv()
		if err == io.EOF {
			logger.V(HandlerTraceLogLevel).WithValues(
				"blockMetadataType", blockMetadataType.String(),
				"lastByteOffset", lastByteOffset,
				"lastSize", lastSize,
				"lastResponseNum", responseNum,
				"volumeCapacityBytes", volumeCapacityBytes,
			).Info("stream EOF")
			return nil
		}

		if err != nil {
			logger.WithValues(
				"blockMetadataType", blockMetadataType.String(),
				"lastByteOffset", lastByteOffset,
				"lastSize", lastSize,
				"lastResponseNum", responseNum,
				"volumeCapacityBytes", volumeCapacityBytes,
			).Error(err, msgInternalFailedCSIDriverResponse)
			return s.statusPassOrWrapError(err, codes.Internal, msgInternalFailedCSIDriverResponseFmt, err)
		}

		responseNum++

		clientResp := s.convertToGetMetadataDeltaResponse(csiResp)
		blockMetadataType = clientResp.BlockMetadataType
		volumeCapacityBytes = clientResp.VolumeCapacityBytes
		numBlockMetadata = len(clientResp.BlockMetadata) - 1
		if numBlockMetadata >= 0 {
			lastByteOffset = clientResp.BlockMetadata[numBlockMetadata].ByteOffset
			lastSize = clientResp.BlockMetadata[numBlockMetadata].SizeBytes
		}

		if logger.V(HandlerDetailedTraceLogLevel).Enabled() {
			var b strings.Builder
			b.WriteString("[")
			for _, bmd := range clientResp.BlockMetadata {
				b.WriteString(fmt.Sprintf("{%d,%d}", bmd.ByteOffset, bmd.SizeBytes))
			}
			b.WriteString("]")
			logger.WithValues(
				"blockMetadataType", blockMetadataType.String(),
				"responseNum", responseNum,
				"volumeCapacityBytes", volumeCapacityBytes,
				"blockMetadata", b.String(),
				"numBlockMetadata", len(clientResp.BlockMetadata),
			).Info("stream response")
		}

		if err := clientStream.Send(clientResp); err != nil {
			logger.WithValues(
				"blockMetadataType", blockMetadataType.String(),
				"lastByteOffset", lastByteOffset,
				"lastSize", lastSize,
				"responseNum", responseNum,
				"volumeCapacityBytes", volumeCapacityBytes,
			).Error(err, msgInternalFailedToSendResponse)
			return s.statusPassOrWrapError(err, codes.Internal, msgInternalFailedToSendResponseFmt, err)
		}
	}
}

func (s *Server) convertToGetMetadataDeltaResponse(csiResp *csi.GetMetadataDeltaResponse) *api.GetMetadataDeltaResponse {
	apiResp := &api.GetMetadataDeltaResponse{
		BlockMetadataType:   api.BlockMetadataType(csiResp.BlockMetadataType),
		VolumeCapacityBytes: csiResp.VolumeCapacityBytes,
	}

	for _, b := range csiResp.GetBlockMetadata() {
		apiResp.BlockMetadata = append(apiResp.BlockMetadata, &api.BlockMetadata{
			ByteOffset: b.ByteOffset,
			SizeBytes:  b.SizeBytes,
		})
	}

	return apiResp
}
