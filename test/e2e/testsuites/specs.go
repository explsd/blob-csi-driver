/*
Copyright 2019 The Kubernetes Authors.

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

package testsuites

import (
	"context"
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/blob-csi-driver/test/e2e/driver"

	"github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	clientset "k8s.io/client-go/kubernetes"
)

type PodDetails struct {
	Cmd     string
	Volumes []VolumeDetails
}

type VolumeDetails struct {
	VolumeType            string
	FSType                string
	Encrypted             bool
	MountOptions          []string
	ClaimSize             string
	ReclaimPolicy         *v1.PersistentVolumeReclaimPolicy
	VolumeBindingMode     *storagev1.VolumeBindingMode
	AllowedTopologyValues []string
	VolumeMode            VolumeMode
	VolumeMount           VolumeMountDetails
	VolumeDevice          VolumeDeviceDetails
	// Optional, used with pre-provisioned volumes
	VolumeID string
	// Optional, used with PVCs created from snapshots
	DataSource *DataSource
	// Optional, used with specified StorageClass
	StorageClass       *storagev1.StorageClass
	NodeStageSecretRef string
	Attrib             map[string]string
	AccessModes        []v1.PersistentVolumeAccessMode
}

type VolumeMode int

const (
	FileSystem VolumeMode = iota
	Block
)

const (
	VolumePVCKind      = "PersistentVolumeClaim"
	APIVersionv1alpha1 = "v1alpha1"
)

var (
	isAzureStackCloud                            = strings.EqualFold(os.Getenv("AZURE_CLOUD_NAME"), "AZURESTACKCLOUD")
	azurePublicCloudSupportedStorageAccountTypes = []string{"Standard_LRS", "Premium_LRS", "Standard_GRS", "Standard_RAGRS"}
	azureStackCloudSupportedStorageAccountTypes  = []string{"Standard_LRS", "Premium_LRS"}
)

type VolumeMountDetails struct {
	NameGenerate      string
	MountPathGenerate string
	ReadOnly          bool
}

type VolumeDeviceDetails struct {
	NameGenerate string
	DevicePath   string
}

type DataSource struct {
	Kind string
	Name string
}

func (pod *PodDetails) SetupWithCSIInlineVolumes(client clientset.Interface, namespace *v1.Namespace, secretName, shareName string, readOnly bool) (*TestPod, []func(context.Context)) {
	tpod := NewTestPod(client, namespace, pod.Cmd)
	cleanupFuncs := make([]func(ctx context.Context), 0)
	for n, v := range pod.Volumes {
		tpod.SetupInlineVolume(fmt.Sprintf("%s%d", v.VolumeMount.NameGenerate, n+1), fmt.Sprintf("%s%d", v.VolumeMount.MountPathGenerate, n+1), secretName, shareName, readOnly)
	}
	return tpod, cleanupFuncs
}

func (pod *PodDetails) SetupWithDynamicVolumes(ctx context.Context, client clientset.Interface, namespace *v1.Namespace, csiDriver driver.DynamicPVTestDriver, storageClassParameters map[string]string) (*TestPod, []func(context.Context)) {
	tpod := NewTestPod(client, namespace, pod.Cmd)
	cleanupFuncs := make([]func(ctx context.Context), 0)
	for n, v := range pod.Volumes {
		tpvc, funcs := v.SetupDynamicPersistentVolumeClaim(ctx, client, namespace, csiDriver, storageClassParameters)
		cleanupFuncs = append(cleanupFuncs, funcs...)
		if v.VolumeMode == Block {
			tpod.SetupRawBlockVolume(tpvc.persistentVolumeClaim, fmt.Sprintf("%s%d", v.VolumeDevice.NameGenerate, n+1), v.VolumeDevice.DevicePath)
		} else {
			tpod.SetupVolume(tpvc.persistentVolumeClaim, fmt.Sprintf("%s%d", v.VolumeMount.NameGenerate, n+1), fmt.Sprintf("%s%d", v.VolumeMount.MountPathGenerate, n+1), v.VolumeMount.ReadOnly)
		}
	}
	return tpod, cleanupFuncs
}

// SetupWithDynamicMultipleVolumes each pod will be mounted with multiple volumes with different storage account types
func (pod *PodDetails) SetupWithDynamicMultipleVolumes(ctx context.Context, client clientset.Interface, namespace *v1.Namespace, csiDriver driver.DynamicPVTestDriver) (*TestPod, []func(context.Context)) {
	tpod := NewTestPod(client, namespace, pod.Cmd)
	cleanupFuncs := make([]func(ctx context.Context), 0)
	supportedStorageAccountTypes := azurePublicCloudSupportedStorageAccountTypes
	if isAzureStackCloud {
		supportedStorageAccountTypes = azureStackCloudSupportedStorageAccountTypes
	}
	accountTypeCount := len(supportedStorageAccountTypes)
	for n, v := range pod.Volumes {
		storageClassParameters := map[string]string{"skuName": supportedStorageAccountTypes[n%accountTypeCount]}
		tpvc, funcs := v.SetupDynamicPersistentVolumeClaim(ctx, client, namespace, csiDriver, storageClassParameters)
		cleanupFuncs = append(cleanupFuncs, funcs...)
		if v.VolumeMode == Block {
			tpod.SetupRawBlockVolume(tpvc.persistentVolumeClaim, fmt.Sprintf("%s%d", v.VolumeDevice.NameGenerate, n+1), v.VolumeDevice.DevicePath)
		} else {
			tpod.SetupVolume(tpvc.persistentVolumeClaim, fmt.Sprintf("%s%d", v.VolumeMount.NameGenerate, n+1), fmt.Sprintf("%s%d", v.VolumeMount.MountPathGenerate, n+1), v.VolumeMount.ReadOnly)
		}
	}
	return tpod, cleanupFuncs
}

func (pod *PodDetails) SetupWithPreProvisionedVolumes(ctx context.Context, client clientset.Interface, namespace *v1.Namespace, csiDriver driver.PreProvisionedVolumeTestDriver) (*TestPod, []func(context.Context)) {
	tpod := NewTestPod(client, namespace, pod.Cmd)
	cleanupFuncs := make([]func(ctx context.Context), 0)
	for n, v := range pod.Volumes {
		tpvc, funcs := v.SetupPreProvisionedPersistentVolumeClaim(ctx, client, namespace, csiDriver)
		cleanupFuncs = append(cleanupFuncs, funcs...)

		if v.VolumeMode == Block {
			tpod.SetupRawBlockVolume(tpvc.persistentVolumeClaim, fmt.Sprintf("%s%d", v.VolumeDevice.NameGenerate, n+1), v.VolumeDevice.DevicePath)
		} else {
			tpod.SetupVolume(tpvc.persistentVolumeClaim, fmt.Sprintf("%s%d", v.VolumeMount.NameGenerate, n+1), fmt.Sprintf("%s%d", v.VolumeMount.MountPathGenerate, n+1), v.VolumeMount.ReadOnly)
		}
	}
	return tpod, cleanupFuncs
}

func (pod *PodDetails) SetupDeployment(ctx context.Context, client clientset.Interface, namespace *v1.Namespace, csiDriver driver.DynamicPVTestDriver, storageClassParameters map[string]string) (*TestDeployment, []func(context.Context), string) {
	cleanupFuncs := make([]func(ctx context.Context), 0)
	volume := pod.Volumes[0]
	ginkgo.By("setting up the StorageClass")
	storageClass := csiDriver.GetProvisionStorageClass(storageClassParameters, volume.MountOptions, volume.ReclaimPolicy, volume.VolumeBindingMode, volume.AllowedTopologyValues, namespace.Name)
	tsc := NewTestStorageClass(client, namespace, storageClass)
	createdStorageClass := tsc.Create(ctx)
	cleanupFuncs = append(cleanupFuncs, tsc.Cleanup)
	ginkgo.By("setting up the PVC")
	tpvc := NewTestPersistentVolumeClaim(client, namespace, volume.ClaimSize, volume.VolumeMode, &createdStorageClass)
	tpvc.Create(ctx)
	tpvc.WaitForBound(ctx)
	tpvc.ValidateProvisionedPersistentVolume(ctx)
	cleanupFuncs = append(cleanupFuncs, tpvc.Cleanup)
	ginkgo.By("setting up the Deployment")
	tDeployment := NewTestDeployment(client, namespace, pod.Cmd, tpvc.persistentVolumeClaim, fmt.Sprintf("%s%d", volume.VolumeMount.NameGenerate, 1), fmt.Sprintf("%s%d", volume.VolumeMount.MountPathGenerate, 1), volume.VolumeMount.ReadOnly)

	cleanupFuncs = append(cleanupFuncs, tDeployment.Cleanup)
	var volumeID string
	if tpvc.persistentVolume != nil && tpvc.persistentVolume.Spec.CSI != nil {
		volumeID = tpvc.persistentVolume.Spec.CSI.VolumeHandle
	}
	return tDeployment, cleanupFuncs, volumeID
}

func (volume *VolumeDetails) SetupDynamicPersistentVolumeClaim(ctx context.Context, client clientset.Interface, namespace *v1.Namespace, csiDriver driver.DynamicPVTestDriver, storageClassParameters map[string]string) (*TestPersistentVolumeClaim, []func(context.Context)) {
	cleanupFuncs := make([]func(ctx context.Context), 0)
	ginkgo.By("setting up the StorageClass")
	storageClass := csiDriver.GetProvisionStorageClass(storageClassParameters, volume.MountOptions, volume.ReclaimPolicy, volume.VolumeBindingMode, volume.AllowedTopologyValues, namespace.Name)
	tsc := NewTestStorageClass(client, namespace, storageClass)
	createdStorageClass := tsc.Create(ctx)
	cleanupFuncs = append(cleanupFuncs, tsc.Cleanup)
	ginkgo.By("setting up the PVC and PV")
	var tpvc *TestPersistentVolumeClaim
	if volume.DataSource != nil {
		dataSource := &v1.TypedLocalObjectReference{
			Name: volume.DataSource.Name,
			Kind: volume.DataSource.Kind,
		}
		tpvc = NewTestPersistentVolumeClaimWithDataSource(client, namespace, volume.ClaimSize, volume.VolumeMode, &createdStorageClass, dataSource)
	} else {
		tpvc = NewTestPersistentVolumeClaim(client, namespace, volume.ClaimSize, volume.VolumeMode, &createdStorageClass)
	}

	if len(volume.AccessModes) > 0 {
		ginkgo.By("setting up the PVC with access modes")
		tpvc.AccessModes = volume.AccessModes
	}
	tpvc.Create(ctx)
	cleanupFuncs = append(cleanupFuncs, tpvc.Cleanup)
	// PV will not be ready until PVC is used in a pod when volumeBindingMode: WaitForFirstConsumer
	if volume.VolumeBindingMode == nil || *volume.VolumeBindingMode == storagev1.VolumeBindingImmediate {
		tpvc.WaitForBound(ctx)
		tpvc.ValidateProvisionedPersistentVolume(ctx)
	}

	return tpvc, cleanupFuncs
}

func (volume *VolumeDetails) SetupPreProvisionedPersistentVolumeClaim(ctx context.Context, client clientset.Interface, namespace *v1.Namespace, csiDriver driver.PreProvisionedVolumeTestDriver) (*TestPersistentVolumeClaim, []func(ctx context.Context)) {
	cleanupFuncs := make([]func(ctx context.Context), 0)
	ginkgo.By("setting up the PV")
	attrib := volume.Attrib
	nodeStageSecretRef := volume.NodeStageSecretRef
	pv := csiDriver.GetPersistentVolume(volume.VolumeID, volume.FSType, volume.ClaimSize, volume.ReclaimPolicy, namespace.Name, attrib, nodeStageSecretRef)
	tpv := NewTestPreProvisionedPersistentVolume(client, pv)
	tpv.Create(ctx)
	ginkgo.By("setting up the PVC")
	tpvc := NewTestPersistentVolumeClaim(client, namespace, volume.ClaimSize, volume.VolumeMode, nil)
	tpvc.Create(ctx)
	cleanupFuncs = append(cleanupFuncs, tpvc.DeleteBoundPersistentVolume)
	cleanupFuncs = append(cleanupFuncs, tpvc.Cleanup)
	tpvc.WaitForBound(ctx)
	tpvc.ValidateProvisionedPersistentVolume(ctx)

	return tpvc, cleanupFuncs
}

func (volume *VolumeDetails) CreateStorageClass(ctx context.Context, client clientset.Interface, namespace *v1.Namespace, csiDriver driver.DynamicPVTestDriver, storageClassParameters map[string]string) (*TestStorageClass, func(ctx context.Context)) {
	ginkgo.By("setting up the StorageClass")
	storageClass := csiDriver.GetProvisionStorageClass(storageClassParameters, volume.MountOptions, volume.ReclaimPolicy, volume.VolumeBindingMode, volume.AllowedTopologyValues, namespace.Name)
	tsc := NewTestStorageClass(client, namespace, storageClass)
	tsc.Create(ctx)
	return tsc, tsc.Cleanup
}
