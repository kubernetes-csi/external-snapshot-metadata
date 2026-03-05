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
	"errors"
	"testing"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	snapshotutils "github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	"github.com/stretchr/testify/assert"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	clientgotesting "k8s.io/client-go/testing"
)

const (
	PrefixedSnapshotterSecretNameKey      = snapshotutils.PrefixedSnapshotterSecretNameKey
	PrefixedSnapshotterSecretNamespaceKey = snapshotutils.PrefixedSnapshotterSecretNamespaceKey
)

func TestGetVolumeSnapshotClass(t *testing.T) {
	th := newTestHarness().WithFakeClientAPIs()
	s := th.ServerWithRuntime(t, th.Runtime())

	t.Run("error", func(t *testing.T) {
		invalidVSCName := th.VolumeSnapshotClassName + "foo"

		volSnapClass, err := s.getVolumeSnapshotClass(context.Background(), invalidVSCName)
		assert.Error(t, err)
		assert.ErrorContains(t, err, msgUnavailableFailedToGetVolumeSnapshotClass)
		assert.Nil(t, volSnapClass)

		// Indirect via getSnapshotterSecretName
		vsi := &volSnapshotInfo{
			DriverName: th.DriverName,
			VolumeSnapshot: &snapshotv1.VolumeSnapshot{
				Spec: snapshotv1.VolumeSnapshotSpec{
					VolumeSnapshotClassName: &invalidVSCName,
				},
			},
		}
		secretRef, err := s.getSnapshotterSecretRef(context.Background(), vsi)
		assert.Error(t, err)
		assert.ErrorContains(t, err, msgUnavailableFailedToGetVolumeSnapshotClass)
		assert.Nil(t, secretRef)

		// Indirect via getSnapshotterCredentials
		secretMap, err := s.getSnapshotterCredentials(context.Background(), vsi)
		assert.Error(t, err)
		assert.ErrorContains(t, err, msgUnavailableFailedToGetVolumeSnapshotClass)
		assert.Nil(t, secretMap)
	})

	t.Run("success", func(t *testing.T) {
		volSnapClass, err := s.getVolumeSnapshotClass(context.Background(), th.VolumeSnapshotClassName)
		assert.NoError(t, err)
		assert.NotNil(t, volSnapClass)
		assert.Equal(t, th.VSCParams(), volSnapClass.Parameters)
	})
}

func TestGetDefaultVolumeSnapshotClassForDriver(t *testing.T) {
	th := newTestHarness().WithFakeClientAPIs()

	t.Run("list-error", func(t *testing.T) {
		defer th.FakePopPrependedReactors()

		th.FakeSnapshotClient.PrependReactor("list", "volumesnapshotclasses", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
			return true, nil, errors.New("fake-error")
		})
		s := th.ServerWithRuntime(t, th.Runtime())

		volSnapClass, err := s.getDefaultVolumeSnapshotClassForDriver(context.Background(), "")
		assert.Error(t, err)
		assert.ErrorContains(t, err, msgUnavailableFailedToListVolumeSnapshotClasses)
		assert.Nil(t, volSnapClass)

		// Indirect via getSnapshotterSecretName
		vsi := &volSnapshotInfo{
			DriverName:     th.DriverName,
			VolumeSnapshot: &snapshotv1.VolumeSnapshot{}, // nil VolumeSnapshotClassName
		}
		secretRef, err := s.getSnapshotterSecretRef(context.Background(), vsi)
		assert.Error(t, err)
		assert.ErrorContains(t, err, msgUnavailableFailedToListVolumeSnapshotClasses)
		assert.Nil(t, secretRef)
	})

	t.Run("no-default", func(t *testing.T) {
		defer th.FakePopPrependedReactors()

		th.FakeSnapshotClient.PrependReactor("list", "volumesnapshotclasses", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
			vscList := &snapshotv1.VolumeSnapshotClassList{
				Items: []snapshotv1.VolumeSnapshotClass{
					*th.VSCIsDefaultFalse(),
					*th.VSCNoAnn(),
				},
			}
			return true, vscList, nil
		})
		s := th.ServerWithRuntime(t, th.Runtime())

		volSnapClass, err := s.getDefaultVolumeSnapshotClassForDriver(context.Background(), th.DriverName)
		assert.Error(t, err)
		assert.ErrorContains(t, err, msgUnavailableNoDefaultVolumeSnapshotClassForDriver)
		assert.Nil(t, volSnapClass)
	})

	t.Run("multiple-defaults", func(t *testing.T) {
		defer th.FakePopPrependedReactors()

		dupDefault := th.VSCIsDefaultTrue().DeepCopy()
		dupDefault.Name = "dup-of-" + dupDefault.Name
		th.FakeSnapshotClient.PrependReactor("list", "volumesnapshotclasses", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
			vscList := &snapshotv1.VolumeSnapshotClassList{
				Items: []snapshotv1.VolumeSnapshotClass{
					*th.VSCIsDefaultFalse(),
					*th.VSCIsDefaultTrue(),
					*dupDefault,
				},
			}
			return true, vscList, nil
		})
		s := th.ServerWithRuntime(t, th.Runtime())

		volSnapClass, err := s.getDefaultVolumeSnapshotClassForDriver(context.Background(), th.DriverName)
		assert.Error(t, err)
		assert.ErrorContains(t, err, msgUnavailableMultipleDefaultVolumeSnapshotClassesForDriver)
		assert.Nil(t, volSnapClass)
	})

	t.Run("success", func(t *testing.T) {
		defer th.FakePopPrependedReactors()

		defaultVSC := th.VSCIsDefaultTrue()
		th.FakeSnapshotClient.PrependReactor("list", "volumesnapshotclasses", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
			vscList := &snapshotv1.VolumeSnapshotClassList{
				Items: []snapshotv1.VolumeSnapshotClass{
					*th.VSCIsDefaultFalse(),
					*th.VSCOtherAnn(),
					*th.VSCOtherDriverDefault(),
					*defaultVSC,
					*th.VSCNoAnn(),
				},
			}
			return true, vscList, nil
		})
		s := th.ServerWithRuntime(t, th.Runtime())

		volSnapClass, err := s.getDefaultVolumeSnapshotClassForDriver(context.Background(), th.DriverName)
		assert.NoError(t, err)
		assert.NotNil(t, volSnapClass)
		assert.Equal(t, defaultVSC.Name, volSnapClass.Name)
	})
}

func TestGetSnapshotterSecretRef(t *testing.T) {
	// error via getVolumeSnapshotClass tested in TestGetVolumeSnapshotClass.
	// error via getDefaultVolumeSnapshotClassForDriver tested in TestGetDefaultVolumeSnapshotClassForDriver.

	th := newTestHarness().WithFakeClientAPIs()

	t.Run("vsc-secret-invalid", func(t *testing.T) {
		vsc := th.VSCNoAnn()
		vscNoNs := vsc.DeepCopy()
		delete(vscNoNs.Parameters, PrefixedSnapshotterSecretNameKey)
		vscNoName := vsc.DeepCopy()
		delete(vscNoName.Parameters, PrefixedSnapshotterSecretNamespaceKey)

		vsi := &volSnapshotInfo{
			DriverName: th.DriverName,
			VolumeSnapshot: &snapshotv1.VolumeSnapshot{
				Spec: snapshotv1.VolumeSnapshotSpec{
					VolumeSnapshotClassName: &vsc.Name,
				},
			},
		}

		// sub-cases
		tcs := []struct {
			name string
			vsc  *snapshotv1.VolumeSnapshotClass
		}{
			{
				name: "params-ns-missing",
				vsc:  vscNoNs,
			},
			{
				name: "params-name-missing",
				vsc:  vscNoName,
			},
		}
		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				defer th.FakePopPrependedReactors()

				th.FakeSnapshotClient.PrependReactor("get", "volumesnapshotclasses", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
					ga := action.(clientgotesting.GetAction)
					if ga.GetName() != tc.vsc.Name {
						return false, nil, nil
					}
					return true, tc.vsc, nil
				})
				s := th.ServerWithRuntime(t, th.Runtime())

				secretRef, err := s.getSnapshotterSecretRef(context.Background(), vsi)
				assert.Error(t, err)
				assert.ErrorContains(t, err, msgUnavailableInvalidSecretInVolumeSnapshotClass)
				assert.Nil(t, secretRef)
			})
		}
	})

	t.Run("vsc-no-secret", func(t *testing.T) {
		vsc := th.VSCNoAnn()
		vscParamsNil := vsc.DeepCopy()
		vscParamsNil.Parameters = nil
		vscParamsNoSecret := vsc.DeepCopy()
		delete(vscParamsNoSecret.Parameters, PrefixedSnapshotterSecretNameKey)
		delete(vscParamsNoSecret.Parameters, PrefixedSnapshotterSecretNamespaceKey)

		vsi := &volSnapshotInfo{
			DriverName: th.DriverName,
			VolumeSnapshot: &snapshotv1.VolumeSnapshot{
				Spec: snapshotv1.VolumeSnapshotSpec{
					VolumeSnapshotClassName: &vsc.Name,
				},
			},
		}

		// sub-cases
		tcs := []struct {
			name string
			vsc  *snapshotv1.VolumeSnapshotClass
		}{
			{
				name: "params-nil",
				vsc:  vscParamsNil,
			},
			{
				name: "params-have-no-reference",
				vsc:  vscParamsNoSecret,
			},
		}
		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				defer th.FakePopPrependedReactors()

				th.FakeSnapshotClient.PrependReactor("get", "volumesnapshotclasses", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
					ga := action.(clientgotesting.GetAction)
					if ga.GetName() != tc.vsc.Name {
						return false, nil, nil
					}
					return true, tc.vsc, nil
				})
				s := th.ServerWithRuntime(t, th.Runtime())

				secretRef, err := s.getSnapshotterSecretRef(context.Background(), vsi)
				assert.NoError(t, err)
				assert.Nil(t, secretRef)

				// Indirect via getSnapshotterCredentials
				secretMap, err := s.getSnapshotterCredentials(context.Background(), vsi)
				assert.NoError(t, err)
				assert.Nil(t, secretMap)
			})
		}
	})

	t.Run("vsc-with-secret", func(t *testing.T) {
		vs := th.VolumeSnapshot("test-vs", "test-ns")
		assert.NotNil(t, vs)
		assert.NotNil(t, vs.Spec.VolumeSnapshotClassName)
		assert.NotNil(t, vs.Status.BoundVolumeSnapshotContentName)

		vscName := *vs.Spec.VolumeSnapshotClassName
		contentName := *vs.Status.BoundVolumeSnapshotContentName

		vsc := th.VSCNoAnn()
		vsc.Name = vscName

		vsi := &volSnapshotInfo{
			DriverName:                th.DriverName,
			VolumeSnapshot:            vs,
			VolumeSnapshotContentName: contentName,
		}

		// sub-cases
		tcs := []struct {
			name    string
			params  map[string]string
			expNs   string
			expName string
		}{
			{
				name:    "no-templating",
				params:  th.VSCParams(),
				expNs:   th.SecretNs,
				expName: th.SecretName,
			},
			{
				name: "content:(name,name)",
				params: map[string]string{
					PrefixedSnapshotterSecretNamespaceKey: "ns-${volumesnapshotcontent.name}",
					PrefixedSnapshotterSecretNameKey:      "name-${volumesnapshotcontent.name}",
				},
				expNs:   "ns-" + contentName,
				expName: "name-" + contentName,
			},
			{
				name: "snapshot:(namespace,name)",
				params: map[string]string{
					PrefixedSnapshotterSecretNamespaceKey: "ns-${volumesnapshot.namespace}",
					PrefixedSnapshotterSecretNameKey:      "name-${volumesnapshot.name}",
				},
				expNs:   "ns-" + vs.Namespace,
				expName: "name-" + vs.Name,
			},
			{
				name: "snapshot:(namespace,namespace)",
				params: map[string]string{
					PrefixedSnapshotterSecretNamespaceKey: "ns-${volumesnapshot.namespace}",
					PrefixedSnapshotterSecretNameKey:      "name-${volumesnapshot.namespace}",
				},
				expNs:   "ns-" + vs.Namespace,
				expName: "name-" + vs.Namespace,
			},
		}
		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				defer th.FakePopPrependedReactors()

				th.FakeSnapshotClient.PrependReactor("get", "volumesnapshotclasses", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
					ga := action.(clientgotesting.GetAction)
					if ga.GetName() != vsc.Name {
						return false, nil, nil
					}
					vsc.Parameters = tc.params
					return true, vsc, nil
				})
				s := th.ServerWithRuntime(t, th.Runtime())

				secretRef, err := s.getSnapshotterSecretRef(context.Background(), vsi)
				assert.NoError(t, err)
				assert.NotNil(t, secretRef)
				assert.Equal(t, tc.expNs, secretRef.Namespace)
				assert.Equal(t, tc.expName, secretRef.Name)
			})
		}
	})
}

func TestGetSnapshotterCredentials(t *testing.T) {
	// error via getSnapshotterSecretName tested in TestGetVolumeSnapshotClass.
	// no-secret case tested in TestGetSnapshotterSecretRef.

	th := newTestHarness().WithFakeClientAPIs()

	t.Run("get-secret-error", func(t *testing.T) {
		defer th.FakePopPrependedReactors()

		th.FakeKubeClient.PrependReactor("get", "secrets", func(action clientgotesting.Action) (handled bool, ret apiruntime.Object, err error) {
			ga := action.(clientgotesting.GetAction)
			if ga.GetNamespace() != th.SecretNs || ga.GetName() != th.SecretName {
				return false, nil, nil
			}
			return true, nil, errors.New("test-error")
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
		secretMap, err := s.getSnapshotterCredentials(context.Background(), vsi)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "test-error")
		assert.ErrorContains(t, err, msgUnavailableFailedToGetCredentials)
		assert.Nil(t, secretMap)
	})

	t.Run("success", func(t *testing.T) {
		s := th.ServerWithRuntime(t, th.Runtime())

		vsi := &volSnapshotInfo{
			DriverName: th.DriverName,
			VolumeSnapshot: &snapshotv1.VolumeSnapshot{
				Spec: snapshotv1.VolumeSnapshotSpec{
					VolumeSnapshotClassName: &th.VolumeSnapshotClassName,
				},
			},
		}
		secretMap, err := s.getSnapshotterCredentials(context.Background(), vsi)
		assert.NoError(t, err)
		assert.NotNil(t, secretMap)

		expMap := convStringByteMapToStringStringMap(th.SecretData())
		assert.Equal(t, expMap, secretMap)
	})
}
