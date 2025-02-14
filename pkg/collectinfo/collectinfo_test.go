/*
Copyright 2023 The aerospike-operator Authors.
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

package collectinfo_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	v1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/collectinfo"
	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/internal"
	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/testutils"
)

const (
	nodeName                   = "test-node"
	scName                     = "test-sc"
	serviceName                = "test-service"
	pvcName                    = "test-pvc"
	pvName                     = "test-pv"
	stsName                    = "test-sts"
	deployName                 = "test-deploy"
	podName                    = "test-pod"
	containerName              = "test-container"
	aerospikeClusterName       = "test-aerocluster"
	aerospikeBackupServiceName = "test-aerobackupservice"
	aerospikeBackupName        = "test-aerobackup"
	aerospikeRestoreName       = "test-aerorestore"
	pdbName                    = "test-pdb"
	cmName                     = "test-cm"
)

var (
	clusterScopeDir   = filepath.Join(collectinfo.RootOutputDir, collectinfo.ClusterScopedDir)
	namespaceScopeDir = filepath.Join(collectinfo.RootOutputDir, collectinfo.NamespaceScopedDir)
)

// key format: RootOutputDir/<k8s-cluster or k8s-namespaces>/ns/<objectKIND>/<objectName>
var filesList = map[string]bool{
	filepath.Join(clusterScopeDir, collectinfo.KindDirNames[internal.NodeKind],
		nodeName+collectinfo.FileSuffix): false,
	filepath.Join(clusterScopeDir, collectinfo.KindDirNames[internal.SCKind],
		scName+collectinfo.FileSuffix): false,
	filepath.Join(clusterScopeDir, collectinfo.KindDirNames[internal.PVKind],
		pvName+collectinfo.FileSuffix): false,
	filepath.Join(clusterScopeDir, collectinfo.KindDirNames[internal.MutatingWebhookKind],
		collectinfo.MutatingWebhookName+collectinfo.FileSuffix): false,
	filepath.Join(clusterScopeDir, collectinfo.KindDirNames[internal.ValidatingWebhookKind],
		collectinfo.ValidatingWebhookName+collectinfo.FileSuffix): false,
	filepath.Join(clusterScopeDir, collectinfo.SummaryDir,
		collectinfo.SummaryFile): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.KindDirNames[internal.PVCKind],
		pvcName+collectinfo.FileSuffix): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.KindDirNames[internal.STSKind],
		stsName+collectinfo.FileSuffix): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.KindDirNames[internal.DeployKind],
		deployName+collectinfo.FileSuffix): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.KindDirNames[internal.PodKind], podName, "logs",
		containerName+".log"): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.KindDirNames[internal.PodKind], podName, "logs", "previous",
		containerName+".log"): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.KindDirNames[internal.PodKind], podName,
		podName+collectinfo.FileSuffix): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.KindDirNames[internal.ServiceKind],
		serviceName+collectinfo.FileSuffix): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.KindDirNames[internal.AerospikeClusterKind],
		aerospikeClusterName+collectinfo.FileSuffix): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.KindDirNames[internal.AerospikeBackupServiceKind],
		aerospikeBackupServiceName+collectinfo.FileSuffix): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.KindDirNames[internal.AerospikeBackupKind],
		aerospikeBackupName+collectinfo.FileSuffix): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.KindDirNames[internal.AerospikeRestoreKind],
		aerospikeRestoreName+collectinfo.FileSuffix): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.KindDirNames[internal.PodDisruptionBudgetKind],
		pdbName+collectinfo.FileSuffix): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.KindDirNames[internal.ConfigMapKind],
		cmName+collectinfo.FileSuffix): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.SummaryDir,
		collectinfo.SummaryFile): false,
	filepath.Join(collectinfo.RootOutputDir,
		collectinfo.LogFileName): false,
}

var _ = Describe("collectInfo", func() {
	Context("When doing valid operations", func() {

		createOption := &client.CreateOptions{}

		It("Should create a tar file with all logs", func() {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: nodeName},
			}
			err := k8sClient.Create(context.TODO(), node, createOption)
			Expect(err).ToNot(HaveOccurred())

			sc := &v1.StorageClass{
				ObjectMeta:  metav1.ObjectMeta{Name: scName},
				Provisioner: "provisionerPluginName",
			}
			err = k8sClient.Create(context.TODO(), sc, createOption)
			Expect(err).ToNot(HaveOccurred())

			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Port: 3000},
					},
				},
			}
			err = k8sClient.Create(context.TODO(), service, createOption)
			Expect(err).ToNot(HaveOccurred())

			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: namespace},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
					VolumeName: pvName,
				},
			}
			err = k8sClient.Create(context.TODO(), pvc, createOption)
			Expect(err).ToNot(HaveOccurred())

			volumeMode := corev1.PersistentVolumeBlock
			pv := &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{Name: pvName},
				Spec: corev1.PersistentVolumeSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Capacity: map[corev1.ResourceName]resource.Quantity{
						"storage": resource.MustParse("1Gi"),
					},
					ClaimRef: &corev1.ObjectReference{
						Name:      pvcName,
						Namespace: namespace,
					},
					StorageClassName: "",
					VolumeMode:       &volumeMode,
					PersistentVolumeSource: corev1.PersistentVolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/opt/volume/ngnix",
						},
					},
				},
			}
			err = k8sClient.Create(context.TODO(), pv, createOption)
			Expect(err).ToNot(HaveOccurred())

			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: stsName, Namespace: namespace},
				Spec: appsv1.StatefulSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "t1", "s2iBuilder": "t1-s2i-1x55", "version": "v1"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "t1", "s2iBuilder": "t1-s2i-1x55", "version": "v1"},
						},
					},
				},
			}
			err = k8sClient.Create(context.TODO(), sts, createOption)
			Expect(err).ToNot(HaveOccurred())

			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: deployName, Namespace: namespace},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "t1", "s2iBuilder": "t1-s2i-1x55", "version": "v1"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "t1", "s2iBuilder": "t1-s2i-1x55", "version": "v1"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  containerName,
									Image: "nginx:1.12",
								},
							},
						},
					},
				},
			}
			err = k8sClient.Create(context.TODO(), deploy, createOption)
			Expect(err).ToNot(HaveOccurred())

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: namespace},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  containerName,
							Image: "nginx",
						},
					},
				},
			}
			err = k8sClient.Create(context.TODO(), pod, createOption)
			Expect(err).ToNot(HaveOccurred())

			mutatingWebhook := &admissionv1.MutatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: collectinfo.MutatingWebhookName},
			}
			err = k8sClient.Create(context.TODO(), mutatingWebhook, createOption)
			Expect(err).ToNot(HaveOccurred())

			validatingWebhook := &admissionv1.ValidatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: collectinfo.ValidatingWebhookName},
			}
			err = k8sClient.Create(context.TODO(), validatingWebhook, createOption)
			Expect(err).ToNot(HaveOccurred())

			gvk := schema.GroupVersionKind{
				Group:   internal.Group,
				Version: "v1",
				Kind:    internal.AerospikeClusterKind,
			}

			createUnstructuredObject(aerospikeClusterName, namespace, gvk)

			gvk = schema.GroupVersionKind{
				Group:   internal.Group,
				Version: internal.BetaVersion,
				Kind:    internal.AerospikeBackupServiceKind,
			}

			createUnstructuredObject(aerospikeBackupServiceName, namespace, gvk)

			gvk = schema.GroupVersionKind{
				Group:   internal.Group,
				Version: internal.BetaVersion,
				Kind:    internal.AerospikeBackupKind,
			}

			createUnstructuredObject(aerospikeBackupName, namespace, gvk)

			gvk = schema.GroupVersionKind{
				Group:   internal.Group,
				Version: internal.BetaVersion,
				Kind:    internal.AerospikeRestoreKind,
			}

			createUnstructuredObject(aerospikeRestoreName, namespace, gvk)

			maxUnavailable := intstr.FromInt32(1)
			pdb := &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{Name: pdbName, Namespace: namespace},
				Spec: policyv1.PodDisruptionBudgetSpec{
					MaxUnavailable: &maxUnavailable,
				},
			}

			err = k8sClient.Create(context.TODO(), pdb, createOption)
			Expect(err).ToNot(HaveOccurred())

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace},
				Data:       map[string]string{},
			}

			err = k8sClient.Create(context.TODO(), cm, createOption)
			Expect(err).ToNot(HaveOccurred())

			err = os.MkdirAll(collectinfo.RootOutputDir, os.ModePerm)
			Expect(err).ToNot(HaveOccurred())

			params, err := testutils.NewTestParams(testCtx, k8sClient, k8sClientSet, []string{namespace}, false, true)
			Expect(err).ToNot(HaveOccurred())

			params.Logger = collectinfo.AttachFileLogger(params.Logger,
				filepath.Join(collectinfo.RootOutputDir, collectinfo.LogFileName))

			err = collectinfo.CollectInfo(testCtx, params, "")
			Expect(err).ToNot(HaveOccurred())

			err = validateAndDeleteTar(collectinfo.TarName, filesList)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

func validateAndDeleteTar(srcFile string, filesList map[string]bool) error {
	f, err := os.Open(srcFile)
	if err != nil {
		return err
	}
	defer f.Close()

	gzf, err := gzip.NewReader(f)
	if err != nil {
		return err
	}

	tarReader := tar.NewReader(gzf)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		name := header.Name

		switch header.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
			if _, ok := filesList[name]; ok {
				filesList[name] = true
			} else {
				return fmt.Errorf("found unexpected file in tar %s", name)
			}
		default:
			return fmt.Errorf("unable to figure out type : %c in file %s",
				header.Typeflag,
				name,
			)
		}
	}

	var missingFiles []string

	for key, value := range filesList {
		if !value {
			missingFiles = append(missingFiles, key)
		}
	}

	if len(missingFiles) != 0 {
		return fmt.Errorf("certain log files are missing %v", missingFiles)
	}

	return os.Remove(srcFile)
}

func createUnstructuredObject(name, namespace string, gvk schema.GroupVersionKind) {
	u := &unstructured.Unstructured{}
	u.SetName(name)
	u.SetNamespace(namespace)
	u.SetGroupVersionKind(gvk)

	err := k8sClient.Create(context.TODO(), u)
	Expect(err).ToNot(HaveOccurred())
}
