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

package e2e

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"sigs.k8s.io/blob-csi-driver/test/e2e/driver"
	"sigs.k8s.io/blob-csi-driver/test/e2e/testsuites"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

var _ = ginkgo.Describe("[blob-csi-e2e] Dynamic Provisioning", func() {
	f := framework.NewDefaultFramework("blob")

	var (
		cs         clientset.Interface
		ns         *v1.Namespace
		testDriver driver.PVTestDriver
	)

	ginkgo.BeforeEach(func() {
		checkPodsRestart := testCmd{
			command:  "sh",
			args:     []string{"test/utils/check_driver_pods_restart.sh"},
			startLog: "Check driver pods if restarts ...",
			endLog:   "Check successfully",
		}
		execTestCmd([]testCmd{checkPodsRestart})

		cs = f.ClientSet
		ns = f.Namespace
	})

	testDriver = driver.InitBlobCSIDriver()
	ginkgo.It("should create a volume on demand", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						MountOptions: []string{
							"-o allow_other",
							"--file-cache-timeout-in-seconds=120",
						},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver:              testDriver,
			Pods:                   pods,
			StorageClassParameters: map[string]string{"skuName": "Standard_LRS"},
		}
		test.Run(cs, ns)
	})

	ginkgo.It("should create a deployment object, write and read to it, delete the pod and write and read to it again", func() {
		pod := testsuites.PodDetails{
			Cmd: "echo 'hello world' >> /mnt/test-1/data && while true; do sleep 1; done",
			Volumes: []testsuites.VolumeDetails{
				{
					FSType:    "ext3",
					ClaimSize: "10Gi",
					MountOptions: []string{
						"-o allow_other",
						"--file-cache-timeout-in-seconds=120",
					},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedDeletePodTest{
			CSIDriver: testDriver,
			Pod:       pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:            []string{"cat", "/mnt/test-1/data"},
				ExpectedString: "hello world\nhello world\n", // pod will be restarted so expect to see 2 instances of string
			},
			StorageClassParameters: map[string]string{"skuName": "Premium_LRS"},
		}
		test.Run(cs, ns)
	})

	// Track issue https://github.com/kubernetes/kubernetes/issues/70505
	ginkgo.It("should create a volume on demand and mount it as readOnly in a pod", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: "touch /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						FSType:    "ext4",
						ClaimSize: "10Gi",
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
							ReadOnly:          true,
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedReadOnlyVolumeTest{
			CSIDriver:              testDriver,
			Pods:                   pods,
			StorageClassParameters: map[string]string{"skuName": "Standard_GRS"},
		}
		test.Run(cs, ns)
	})

	ginkgo.It("should create multiple PV objects, bind to PVCs and attach all to different pods on the same node", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: "while true; do echo $(date -u) >> /mnt/test-1/data; sleep 1; done",
				Volumes: []testsuites.VolumeDetails{
					{
						FSType:    "ext3",
						ClaimSize: "10Gi",
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
			{
				Cmd: "while true; do echo $(date -u) >> /mnt/test-1/data; sleep 1; done",
				Volumes: []testsuites.VolumeDetails{
					{
						FSType:    "ext4",
						ClaimSize: "10Gi",
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCollocatedPodTest{
			CSIDriver:              testDriver,
			Pods:                   pods,
			ColocatePods:           true,
			StorageClassParameters: map[string]string{"skuName": "Standard_RAGRS"},
		}
		test.Run(cs, ns)
	})

	ginkgo.It(fmt.Sprintf("should delete PV with reclaimPolicy %q", v1.PersistentVolumeReclaimDelete), func() {
		reclaimPolicy := v1.PersistentVolumeReclaimDelete
		volumes := []testsuites.VolumeDetails{
			{
				FSType:        "ext4",
				ClaimSize:     "10Gi",
				ReclaimPolicy: &reclaimPolicy,
			},
		}
		test := testsuites.DynamicallyProvisionedReclaimPolicyTest{
			CSIDriver:              testDriver,
			Volumes:                volumes,
			StorageClassParameters: map[string]string{"skuName": "Standard_LRS"},
		}
		test.Run(cs, ns)
	})

	ginkgo.It(fmt.Sprintf("[env] should retain PV with reclaimPolicy %q", v1.PersistentVolumeReclaimRetain), func() {
		reclaimPolicy := v1.PersistentVolumeReclaimRetain
		volumes := []testsuites.VolumeDetails{
			{
				FSType:        "ext4",
				ClaimSize:     "10Gi",
				ReclaimPolicy: &reclaimPolicy,
			},
		}
		test := testsuites.DynamicallyProvisionedReclaimPolicyTest{
			CSIDriver:              testDriver,
			Volumes:                volumes,
			Driver:                 blobDriver,
			StorageClassParameters: map[string]string{"skuName": "Standard_GRS"},
		}
		test.Run(cs, ns)
	})

	ginkgo.It("should create a pod with multiple volumes", func() {
		volumes := []testsuites.VolumeDetails{}
		for i := 1; i <= 6; i++ {
			volume := testsuites.VolumeDetails{
				ClaimSize: "10Gi",
				VolumeMount: testsuites.VolumeMountDetails{
					NameGenerate:      "test-volume-",
					MountPathGenerate: "/mnt/test-",
				},
			}
			volumes = append(volumes, volume)
		}

		pods := []testsuites.PodDetails{
			{
				Cmd:     "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: volumes,
			},
		}
		test := testsuites.DynamicallyProvisionedPodWithMultiplePVsTest{
			CSIDriver: testDriver,
			Pods:      pods,
		}
		test.Run(cs, ns)
	})

	ginkgo.It("should receive FailedMount event with invalid mount options", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						MountOptions: []string{
							"invalid",
							"mount",
							"options",
						},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedInvalidMountOptions{
			CSIDriver:              testDriver,
			Pods:                   pods,
			StorageClassParameters: map[string]string{"skuName": "Standard_LRS"},
		}
		test.Run(cs, ns)
	})

	ginkgo.It("should create a volume on demand (Bring Your Own Key)", func() {
		// get storage account secret name
		err := os.Chdir("../..")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		defer func() {
			err := os.Chdir("test/e2e")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}()

		getSecretNameScript := "test/utils/get_storage_account_secret_name.sh"
		log.Printf("run script: %s\n", getSecretNameScript)

		cmd := exec.Command("bash", getSecretNameScript)
		output, err := cmd.CombinedOutput()
		log.Printf("got output: %v, error: %v\n", string(output), err)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		secretName := strings.TrimSuffix(string(output), "\n")
		log.Printf("got storage account secret name: %v\n", secretName)
		bringKeyStorageClassParameters["csi.storage.k8s.io/provisioner-secret-name"] = secretName
		bringKeyStorageClassParameters["csi.storage.k8s.io/node-stage-secret-name"] = secretName

		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						MountOptions: []string{
							"-o allow_other",
							"--file-cache-timeout-in-seconds=120",
						},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver:              testDriver,
			Pods:                   pods,
			StorageClassParameters: bringKeyStorageClassParameters,
		}
		test.Run(cs, ns)
	})

	ginkgo.It("should create a volume on demand and resize it [kubernetes.io/blob-csi] [blob.csi.azure.com]", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						ClaimSize: "10Gi",
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedResizeVolumeTest{
			CSIDriver:              testDriver,
			Pods:                   pods,
			StorageClassParameters: map[string]string{"skuName": "Standard_LRS"},
		}
		test.Run(cs, ns)
	})
})
