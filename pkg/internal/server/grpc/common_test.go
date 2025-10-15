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
	"flag"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	fakesnapshot "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	snapshotutils "github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"
	authv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"
	"k8s.io/klog/v2"

	smsv1alpha1 "github.com/kubernetes-csi/external-snapshot-metadata/client/apis/snapshotmetadataservice/v1alpha1"
	fakecbt "github.com/kubernetes-csi/external-snapshot-metadata/client/clientset/versioned/fake"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/internal/runtime"
)

type testHarness struct {
	*runtime.TestHarness

	Audience                string
	DriverName              string
	Namespace               string
	SecretName              string
	SecretNs                string
	SecurityToken           string
	VolumeSnapshotClassName string

	MaxStreamDur time.Duration

	FakeKubeClient     *fake.Clientset
	FakeSnapshotClient *fakesnapshot.Clientset
	FakeCBTClient      *fakecbt.Clientset

	lenFakeKubeClientReactionChain     int
	lenFakeSnapshotClientReactionChain int
	lenFakeCBTClientReactionChain      int

	mockCSIDriverConn *grpc.ClientConn
	grpcServer        *grpc.Server
	listener          *bufconn.Listener
}

func newTestHarness() *testHarness {
	return &testHarness{
		Audience:                "audience",
		DriverName:              "driver",
		Namespace:               "namespace",
		SecretName:              "secret-name",
		SecretNs:                "secret-ns",
		SecurityToken:           "securityToken",
		VolumeSnapshotClassName: "csi-snapshot-class",
		MaxStreamDur:            HandlerDefaultMaxStreamDuration,
	}
}

func (th *testHarness) WithMockCSIDriver(t *testing.T) *testHarness {
	th.TestHarness = runtime.NewTestHarness().WithMockCSIDriver(t)
	th.mockCSIDriverConn = th.MockCSIDriverConn
	return th
}

func (th *testHarness) WithFakeClientAPIs() *testHarness {
	th.FakeKubeClient = th.makeFakeKubeClient()
	th.FakeSnapshotClient = th.makeFakeSnapshotClient()
	th.FakeCBTClient = th.makeFakeCBTClient()

	th.FakeCountReactors() // establish the baseline

	return th
}

func (th *testHarness) Runtime() *runtime.Runtime {
	return &runtime.Runtime{
		CBTClient:      th.FakeCBTClient,
		KubeClient:     th.FakeKubeClient,
		SnapshotClient: th.FakeSnapshotClient,
		DriverName:     th.DriverName,
		CSIConn:        th.mockCSIDriverConn,
		MetricsManager: metrics.NewCSIMetricsManagerWithOptions(th.DriverName, metrics.WithSubsystem(runtime.SubSystem), metrics.WithLabelNames(runtime.LabelTargetSnapshotName, runtime.LabelBaseSnapshotID)),
	}
}

func (th *testHarness) ServerWithRuntime(t *testing.T, rt *runtime.Runtime) *Server {
	return &Server{
		config: ServerConfig{
			Runtime:      rt,
			MaxStreamDur: th.MaxStreamDur,
		},
		healthServer: newHealthServer(),
	}
}

func (th *testHarness) StartGRPCServer(t *testing.T, rt *runtime.Runtime) *Server {
	buffer := 1024 * 1024

	th.listener = bufconn.Listen(buffer)
	th.grpcServer = grpc.NewServer()

	s := th.ServerWithRuntime(t, rt)
	assert.NotZero(t, s.config.MaxStreamDur)

	s.grpcServer = th.grpcServer
	api.RegisterSnapshotMetadataServer(s.grpcServer, s)
	healthpb.RegisterHealthServer(s.grpcServer, s.healthServer)

	go func() {
		s.grpcServer.Serve(th.listener)
	}()

	return s
}

func (th *testHarness) StopGRPCServer(t *testing.T) {
	err := th.listener.Close()
	assert.NoError(t, err)

	th.grpcServer.Stop()
}

func (th *testHarness) GRPCSnapshotMetadataClient(t *testing.T) api.SnapshotMetadataClient {
	conn, err := grpc.NewClient("passthrough://bufconn",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return th.listener.Dial()
		}), grpc.WithTransportCredentials(insecure.NewCredentials()))

	assert.NoError(t, err)

	return api.NewSnapshotMetadataClient(conn)
}

func (th *testHarness) GRPCHealthClient(t *testing.T) healthpb.HealthClient {
	conn, err := grpc.NewClient("passthrough://bufconn",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return th.listener.Dial()
		}), grpc.WithTransportCredentials(insecure.NewCredentials()))

	assert.NoError(t, err)

	return healthpb.NewHealthClient(conn)
}

func (th *testHarness) makeFakeKubeClient() *fake.Clientset {
	fakeKubeClient := fake.NewSimpleClientset()
	fakeKubeClient.PrependReactor("create", "tokenreviews", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
		ca := action.(clientgotesting.CreateAction)
		trs := ca.GetObject().(*authv1.TokenReview)
		trs.Status.Authenticated = false
		if trs.Spec.Token == th.SecurityToken {
			trs.Status.Authenticated = true
			trs.Status.Audiences = []string{th.Audience, "other-" + th.Audience}
		}
		return true, trs, nil
	})
	fakeKubeClient.PrependReactor("create", "subjectaccessreviews", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
		ca := action.(clientgotesting.CreateAction)
		sar := ca.GetObject().(*authzv1.SubjectAccessReview)
		if sar.Spec.ResourceAttributes != nil && sar.Spec.ResourceAttributes.Namespace == th.Namespace {
			sar.Status.Allowed = true
		} else {
			sar.Status.Allowed = false
			sar.Status.Reason = "namespace mismatch"
		}
		return true, sar, nil
	})

	fakeKubeClient.PrependReactor("get", "secrets", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
		ga := action.(clientgotesting.GetAction)
		if ga.GetNamespace() != th.SecretNs || ga.GetName() != th.SecretName {
			return false, nil, nil
		}
		secret := &corev1.Secret{
			ObjectMeta: apimetav1.ObjectMeta{
				Namespace: ga.GetNamespace(),
				Name:      ga.GetName(),
			},
			Data: th.SecretData(),
		}
		return true, secret, nil
	})

	return fakeKubeClient
}

func (th *testHarness) SecretData() map[string][]byte {
	return map[string][]byte{
		"userID":  []byte("user-id"),
		"userKey": []byte("user-key"),
	}
}

func (th *testHarness) makeFakeCBTClient() *fakecbt.Clientset {
	fakeCBTClient := fakecbt.NewSimpleClientset()
	fakeCBTClient.PrependReactor("get", "snapshotmetadataservices", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
		ga := action.(clientgotesting.GetAction)
		if ga.GetName() != th.DriverName {
			return false, nil, nil
		}
		sms := &smsv1alpha1.SnapshotMetadataService{
			ObjectMeta: apimetav1.ObjectMeta{
				Name: th.DriverName,
			},
			Spec: smsv1alpha1.SnapshotMetadataServiceSpec{
				Audience: th.Audience,
			},
		}
		return true, sms, nil
	})
	return fakeCBTClient
}

func (th *testHarness) makeFakeSnapshotClient() *fakesnapshot.Clientset {
	fakeSnapshotClient := fakesnapshot.NewSimpleClientset()
	fakeSnapshotClient.PrependReactor("get", "volumesnapshots", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
		ga := action.(clientgotesting.GetAction)
		vs := th.VolumeSnapshot(ga.GetName(), ga.GetNamespace())
		return true, vs, nil
	})
	fakeSnapshotClient.PrependReactor("get", "volumesnapshotcontents", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
		ga := action.(clientgotesting.GetAction)
		vsc := th.VolumeSnapshotContent(ga.GetName(), th.DriverName)
		return true, vsc, nil
	})
	fakeSnapshotClient.PrependReactor("get", "volumesnapshotclasses", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
		ga := action.(clientgotesting.GetAction)
		if ga.GetName() != th.VolumeSnapshotClassName {
			return false, nil, nil
		}
		vs := th.VSCNoAnn() // Not the default but parameters set
		vs.Name = ga.GetName()
		return true, vs, nil
	})
	return fakeSnapshotClient
}

// FakeCountReactors saves the length of the fake reaction chains.
// Implicitly called by WithFakeClientAPIs but can be explicitly
// called if additional "permanent" reactors are added.
func (th *testHarness) FakeCountReactors() {
	th.lenFakeKubeClientReactionChain = len(th.FakeKubeClient.ReactionChain)
	th.lenFakeSnapshotClientReactionChain = len(th.FakeSnapshotClient.ReactionChain)
	th.lenFakeCBTClientReactionChain = len(th.FakeCBTClient.ReactionChain)
}

// FakePopPrependedReactors pops reactors from the front of the fake reaction chains.
// Do not use if AddReactor was used after the call to CountFakeReactors.
func (th *testHarness) FakePopPrependedReactors() {
	var numToPop int

	numToPop = len(th.FakeKubeClient.ReactionChain) - th.lenFakeKubeClientReactionChain
	if numToPop > 0 {
		th.FakeKubeClient.ReactionChain = th.FakeKubeClient.ReactionChain[numToPop:]
	}

	numToPop = len(th.FakeSnapshotClient.ReactionChain) - th.lenFakeSnapshotClientReactionChain
	if numToPop > 0 {
		th.FakeSnapshotClient.ReactionChain = th.FakeSnapshotClient.ReactionChain[numToPop:]
	}

	numToPop = len(th.FakeCBTClient.ReactionChain) - th.lenFakeCBTClientReactionChain
	if numToPop > 0 {
		th.FakeCBTClient.ReactionChain = th.FakeCBTClient.ReactionChain[numToPop:]
	}
}

// ContentNameFromSnapshot returns the content name for a snapshot name.
func (th *testHarness) ContentNameFromSnapshot(snapshotName string) string {
	return "snapcontent-" + snapshotName
}

// SnapshotNameFromContent is the inverse of ContentNameFromSnapshot().
func (th *testHarness) SnapshotNameFromContent(contentName string) string {
	return strings.TrimPrefix(contentName, "snapcontent-") // Identity if prefix missing.
}

// HandleFromSnapshot returns a snapshot identifier based on the snapshot name.
func (th *testHarness) HandleFromSnapshot(snapshotName string) string {
	return snapshotName + "-id"
}

func (th *testHarness) VolumeSnapshot(snapshotName, namespace string) *snapshotv1.VolumeSnapshot {
	return &snapshotv1.VolumeSnapshot{
		ObjectMeta: apimetav1.ObjectMeta{
			Name:      snapshotName,
			Namespace: namespace,
		},
		Spec: snapshotv1.VolumeSnapshotSpec{
			VolumeSnapshotClassName: stringPtr(th.VolumeSnapshotClassName),
			Source: snapshotv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: stringPtr("pvc-1"),
			},
		},
		Status: &snapshotv1.VolumeSnapshotStatus{
			ReadyToUse:                     boolPtr(true),
			BoundVolumeSnapshotContentName: stringPtr(th.ContentNameFromSnapshot(snapshotName)),
		},
	}
}

func (th *testHarness) VolumeSnapshotContent(contentName, driver string) *snapshotv1.VolumeSnapshotContent {
	return &snapshotv1.VolumeSnapshotContent{
		ObjectMeta: apimetav1.ObjectMeta{
			Name: contentName,
		},
		Spec: snapshotv1.VolumeSnapshotContentSpec{
			Driver: driver,
			Source: snapshotv1.VolumeSnapshotContentSource{
				VolumeHandle: stringPtr("volume-" + contentName),
			},
			VolumeSnapshotRef: corev1.ObjectReference{
				Name:      th.SnapshotNameFromContent(contentName),
				Namespace: "test",
			},
		},
		Status: &snapshotv1.VolumeSnapshotContentStatus{
			ReadyToUse:     boolPtr(true),
			SnapshotHandle: stringPtr(th.HandleFromSnapshot(th.SnapshotNameFromContent(contentName))),
		},
	}
}

// VSCParams returns the parameters for a VolumeSnapshotClass.
func (th *testHarness) VSCParams() map[string]string {
	return map[string]string{
		PrefixedSnapshotterSecretNamespaceKey: th.SecretNs,
		PrefixedSnapshotterSecretNameKey:      th.SecretName,
	}
}

func (th *testHarness) VSCIsDefaultTrue() *snapshotv1.VolumeSnapshotClass {
	return &snapshotv1.VolumeSnapshotClass{
		ObjectMeta: apimetav1.ObjectMeta{
			Name: "vsc-is-default",
			Annotations: map[string]string{
				snapshotutils.IsDefaultSnapshotClassAnnotation: "true",
			},
		},
		Driver:     th.DriverName,
		Parameters: th.VSCParams(),
	}
}

func (th *testHarness) VSCIsDefaultFalse() *snapshotv1.VolumeSnapshotClass {
	vsc := th.VSCIsDefaultTrue()
	vsc.Name = "vsc-is-not-the-default"
	vsc.Annotations[snapshotutils.IsDefaultSnapshotClassAnnotation] = "false"
	return vsc
}

func (th *testHarness) VSCOtherAnn() *snapshotv1.VolumeSnapshotClass {
	vsc := th.VSCIsDefaultTrue()
	vsc.Name = "vsc-other-annotation"
	vsc.Annotations = map[string]string{"other": "annotation"}
	return vsc
}

func (th *testHarness) VSCNoAnn() *snapshotv1.VolumeSnapshotClass {
	vsc := th.VSCIsDefaultTrue()
	vsc.Name = "vsc-no-annotation"
	vsc.Annotations = nil
	return vsc
}

func (th *testHarness) VSCOtherDriverDefault() *snapshotv1.VolumeSnapshotClass {
	vsc := th.VSCIsDefaultTrue()
	vsc.Name = "vsc-other-driver-default"
	vsc.Driver = "other-" + th.DriverName
	return vsc
}

func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func convStringByteMapToStringStringMap(inMap map[string][]byte) map[string]string {
	if inMap == nil {
		return nil
	}
	ret := make(map[string]string, len(inMap))
	for k, v := range inMap {
		ret[k] = string(v)
	}
	return ret
}

type KlogRestoreVerbosityFunc func()

// SetKlogVerbosity sets up the default logger with the specified verbosity level.
func (th *testHarness) SetKlogVerbosity(verboseLevel int, uniquePrefix string) KlogRestoreVerbosityFunc {
	klog.ClearLogger()
	// Set the verbosity level using a new flag set.
	// It is not possible to set a verbose klog/v2/testlogger as the background logger
	// because the klog.V() performs its own checks.
	var level klog.Level
	level.Set(strconv.Itoa(verboseLevel))
	fs := flag.NewFlagSet(uniquePrefix+"Fs1", flag.ContinueOnError)
	fs.Var(&level, uniquePrefix+"V1", "test log verbosity level")
	klog.InitFlags(fs)

	return func() {
		// restore the verbosity level using a new flag set
		klog.ClearLogger()
		fs := flag.NewFlagSet(uniquePrefix+"Fs2", flag.ExitOnError)
		level.Set("1")
		fs.Var(&level, uniquePrefix+"V2", "test log verbosity level")
		klog.InitFlags(fs)
	}
}

// test data structure for context propagation testing.
type testSnapshotMetadataServerCtxPropagator struct {
	*csi.UnimplementedSnapshotMetadataServer

	chanToCloseBeforeReturn          chan struct{}
	chanToCloseOnEntry               chan struct{}
	chanToWaitOnBeforeFirstResponse  chan struct{}
	chanToWaitOnBeforeSecondResponse chan struct{}

	handlerWaitsForCtxError bool

	// the mux protects this block of variables
	mux           sync.Mutex
	handlerCalled bool
	send1Err      error
	send2Err      error
	streamCtxErr  error

	// test harness support
	th         *testHarness
	rth        *runtime.TestHarness
	grpcServer *Server
}

// newSMSHarnessForCtxPropagation sets up a test harness with the sidecar service
// connected to a fake CSI driver serving the testSnapshotMetadataServerCtxPropagator.
// The setup provides a mechanism to _deterministically_ sense a canceled context
// in the fake CSI driver, an intrinsically racy operation. The canceled context could
// have arisen because the client canceled its context, or the sidecar timedout in
// transmitting data to the client.
//
// The mechanism works as follows:
//
//   - The application client calls one of the sidecar's GetAllocatedAllocated() or
//     GetMetadataDelta() operations, which return a stream from which data can be received.
//
//   - The sidecar handler gets invoked with a stream on which to send responses to the
//     application client. It wraps the stream context with a deadline and defers the
//     cancellation function invocation.
//     It then makes the same call on the fake CSI driver and receives a stream from
//     which to read responses from the driver. It blocks in a loop reading from
//     the fake CSI driver stream and sending the response to the client.
//
//   - The handler in the fake CSI driver gets invoked with a stream on which to send the
//     responses to the sidecar. It will block waiting on the application client
//     to call synchronizeBeforeCancel().
//
//   - The application client calls synchronizeBeforeCancel(), which wakes up the fake
//     CSI driver handler and it sends the first response. The fake CSI driver handler
//     blocks again waiting for the invoker to call synchronizeAfterCancel().
//
//   - The response is routed through the sidecar back to the application client.
//
//   - The application client receives the first response without error.
//
// At this point we inject an error. Either one of:
//
//   - The application client cancels its context. At some point after this the
//     canceled context is detected in the sidecar; we cannot actually detect
//     this but expect the error to be logged by the sidecar.
//
//   - The application client does not read a response within the sidecar's timeout
//     which will trigger a cancellation of the sidecar context. We could expect to see this
//     logged by the side car, either when the handler fails or the client stream send fails.
//
// Post error synchronization:
//
//   - The application client calls synchronizeAfterCancel() which blocks it until the
//     fake CSI driver handler returns.
//
//   - The fake CSI driver handler wakes up and then loops waiting to detect
//     the failed context. When it breaks out of this loop it attempts to send a
//     second response, which must fail because the client has canceled its context.
//
//   - When the fake CSI driver handler returns the invoker gets unblocked and returns
//     from its call to synchronizeAfterCancel().
//
//   - After this the invoker should examine the SnapshotMetadataServer properties
//     to check for correctness.
func newSMSHarnessForCtxPropagation(t *testing.T, maxStreamDur time.Duration) (*testSnapshotMetadataServerCtxPropagator, *testHarness) {
	s := &testSnapshotMetadataServerCtxPropagator{}
	s.chanToCloseOnEntry = make(chan struct{})
	s.chanToCloseBeforeReturn = make(chan struct{})
	s.chanToWaitOnBeforeFirstResponse = make(chan struct{})
	s.chanToWaitOnBeforeSecondResponse = make(chan struct{})

	// set up a fake csi driver with the runtime test harness
	s.rth = runtime.NewTestHarness().WithFakeKubeConfig(t).WithFakeCSIDriver(t, s)
	rrt := s.rth.RuntimeForFakeCSIDriver(t)

	// configure a local test harness to connect to the fake csi driver
	s.th = newTestHarness()
	// 2 modes: client context canceled or sidecar context canceled
	if maxStreamDur > 0 {
		s.th.MaxStreamDur = maxStreamDur
	} else {
		s.handlerWaitsForCtxError = true
	}
	s.th.DriverName = rrt.DriverName
	s.th.WithFakeClientAPIs()
	rt := s.th.Runtime()
	rt.CSIConn = rrt.CSIConn
	s.grpcServer = s.th.StartGRPCServer(t, rt)
	s.grpcServer.CSIDriverIsReady()

	return s, s.th
}

func (s *testSnapshotMetadataServerCtxPropagator) cleanup(t *testing.T) {
	if s.th != nil {
		s.th.StopGRPCServer(t)
		s.rth.RemoveFakeKubeConfig(t)
		s.rth.TerminateFakeCSIDriver(t)
	}
}

func (s *testSnapshotMetadataServerCtxPropagator) sync(ctx context.Context, sendResp func() error) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	s.handlerCalled = true

	// synchronizeBeforeCancel() is needed to proceed
	close(s.chanToCloseOnEntry)
	<-s.chanToWaitOnBeforeFirstResponse

	// send the first response
	s.send1Err = sendResp()

	// synchronizeAfterCancel() is needed to proceed
	<-s.chanToWaitOnBeforeSecondResponse

	if s.handlerWaitsForCtxError {
		// wait for the client's canceled context to be detected
		for ctx.Err() == nil {
			time.Sleep(time.Millisecond)
		}

		s.streamCtxErr = ctx.Err()
	}

	// send additional responses until an error is encountered
	for s.send2Err == nil {
		s.send2Err = sendResp()
		time.Sleep(time.Millisecond * 10)
	}

	// allow the client blocked in synchronizeAfterCancel() to proceed
	close(s.chanToCloseBeforeReturn)

	return nil
}

func (s *testSnapshotMetadataServerCtxPropagator) synchronizeBeforeCancel() {
	// synchronize with the fake CSI driver
	<-s.chanToCloseOnEntry

	// the fake driver may now send the first response
	close(s.chanToWaitOnBeforeFirstResponse)
}

func (s *testSnapshotMetadataServerCtxPropagator) synchronizeAfterCancel() {
	// the fake driver can now send the second response
	close(s.chanToWaitOnBeforeSecondResponse)

	// wait for the fake driver method to complete
	<-s.chanToCloseBeforeReturn
}

func (s *testSnapshotMetadataServerCtxPropagator) GetMetadataAllocated(req *csi.GetMetadataAllocatedRequest, stream csi.SnapshotMetadata_GetMetadataAllocatedServer) error {
	var byteOffset int64
	return s.sync(stream.Context(),
		func() error {
			byteOffset += 1024
			return stream.Send(&csi.GetMetadataAllocatedResponse{
				BlockMetadataType:   csi.BlockMetadataType_FIXED_LENGTH,
				VolumeCapacityBytes: 1024 * 1024 * 1024,
				BlockMetadata: []*csi.BlockMetadata{
					{
						ByteOffset: byteOffset,
						SizeBytes:  1024,
					},
				},
			})
		})
}

func (s *testSnapshotMetadataServerCtxPropagator) GetMetadataDelta(req *csi.GetMetadataDeltaRequest, stream csi.SnapshotMetadata_GetMetadataDeltaServer) error {
	var byteOffset int64
	return s.sync(stream.Context(),
		func() error {
			byteOffset += 1024
			return stream.Send(&csi.GetMetadataDeltaResponse{
				BlockMetadataType:   csi.BlockMetadataType_FIXED_LENGTH,
				VolumeCapacityBytes: 1024 * 1024 * 1024,
				BlockMetadata: []*csi.BlockMetadata{
					{
						ByteOffset: byteOffset,
						SizeBytes:  1024,
					},
				},
			})
		})
}
