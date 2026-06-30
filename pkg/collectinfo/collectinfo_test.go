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
	rbacv1 "k8s.io/api/rbac/v1"
	v1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/collectinfo"
	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/configuration"
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
	filepath.Join(clusterScopeDir, collectinfo.KindDirNames[internal.CRDKind],
		"aerospikeclusters."+internal.Group+collectinfo.FileSuffix): false,
	filepath.Join(clusterScopeDir, collectinfo.KindDirNames[internal.CRDKind],
		"aerospikebackupservices."+internal.Group+collectinfo.FileSuffix): false,
	filepath.Join(clusterScopeDir, collectinfo.KindDirNames[internal.CRDKind],
		"aerospikebackups."+internal.Group+collectinfo.FileSuffix): false,
	filepath.Join(clusterScopeDir, collectinfo.KindDirNames[internal.CRDKind],
		"aerospikerestores."+internal.Group+collectinfo.FileSuffix): false,
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

// The specs below exercise collectinfo scenario:
//
//	akoctl collectinfo -n <ns> --cluster-scope=false
//
// against a real API server with RBAC enforced, impersonating namespace-restricted
// ServiceAccounts that lack the cluster-wide "list namespaces" permission.
const (
	rbacNS         = "cinfo-rbac-ns"
	rbacNS2        = "cinfo-rbac-ns2"
	restrictedSA   = "akoctl-restricted"    // namespaced reads + get-namespace (no list)
	noNsPermSA     = "akoctl-no-ns-perm"    // namespaced reads only (no namespace perms at all)
	missingReadsSA = "akoctl-missing-reads" // get-namespace only (no namespaced reads)
	nsReaderCR     = "cinfo-ns-reader"
	nsGetterCR     = "cinfo-ns-getter"
)

var _ = Describe("collectinfo under restricted RBAC", Ordered, func() {
	var (
		restrictedClient    client.Client
		restrictedClientSet *kubernetes.Clientset
		noNsPermClient      client.Client
		noNsPermClientSet   *kubernetes.Clientset
		missingReadsClient  client.Client
		missingReadsCS      *kubernetes.Clientset
	)

	BeforeAll(func() {
		By("Creating the target namespaces and seeding namespace-scoped objects")
		Expect(testutils.CreateNamespace(testCtx, k8sClient, rbacNS)).To(Succeed())
		Expect(testutils.CreateNamespace(testCtx, k8sClient, rbacNS2)).To(Succeed())
		seedNamespacedObjects(rbacNS)
		seedNamespacedObjects(rbacNS2)

		By("Creating a ClusterRole that can read all namespace-scoped resources collectinfo captures")
		Expect(k8sClient.Create(testCtx, &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: nsReaderCR},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods", "pods/log", "services", "configmaps", "persistentvolumeclaims"},
					Verbs:     []string{"get", "list"},
				},
				{
					APIGroups: []string{"apps"},
					Resources: []string{"statefulsets", "deployments"},
					Verbs:     []string{"get", "list"},
				},
				{
					APIGroups: []string{"policy"},
					Resources: []string{"poddisruptionbudgets"},
					Verbs:     []string{"get", "list"},
				},
				{
					APIGroups: []string{internal.Group},
					Resources: []string{"aerospikeclusters", "aerospikebackupservices",
						"aerospikebackups", "aerospikerestores"},
					Verbs: []string{"get", "list"},
				},
			},
		})).To(Succeed())

		By("Creating a ClusterRole that can GET a namespace but NOT LIST namespaces")
		Expect(k8sClient.Create(testCtx, &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: nsGetterCR},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"namespaces"},
					Verbs:     []string{"get"},
				},
			},
		})).To(Succeed())

		By("Creating the ServiceAccounts")

		for _, sa := range []string{restrictedSA, noNsPermSA, missingReadsSA} {
			Expect(k8sClient.Create(testCtx, &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: sa, Namespace: rbacNS},
			})).To(Succeed())
		}

		By("Binding the restricted SA: namespaced reads in both namespaces + cluster-wide get-namespace")
		bindClusterRoleInNamespace(nsReaderCR, restrictedSA, rbacNS)
		bindClusterRoleInNamespace(nsReaderCR, restrictedSA, rbacNS2)
		bindClusterRoleClusterWide(nsGetterCR, restrictedSA, "restricted-ns-getter")

		By("Binding the no-namespace-permission SA: namespaced reads only")
		bindClusterRoleInNamespace(nsReaderCR, noNsPermSA, rbacNS)

		By("Binding the missing-reads SA: get-namespace only, no namespaced reads")
		bindClusterRoleClusterWide(nsGetterCR, missingReadsSA, "missing-reads-ns-getter")

		By("Building impersonating clients for each identity")

		restrictedClient, restrictedClientSet = impersonatingClients(rbacNS, restrictedSA)
		noNsPermClient, noNsPermClientSet = impersonatingClients(rbacNS, noNsPermSA)
		missingReadsClient, missingReadsCS = impersonatingClients(rbacNS, missingReadsSA)
	})

	// ---------- Happy paths ----------

	It("lets a namespace-restricted SA run `collectinfo -n <ns> --cluster-scope=false`", func() {
		By("Confirming the SA cannot list namespaces (the permission the old code required)")
		Expect(apierrors.IsForbidden(
			restrictedClient.List(testCtx, &corev1.NamespaceList{}))).To(BeTrue())

		params, err := testutils.NewTestParams(
			testCtx, restrictedClient, restrictedClientSet, []string{rbacNS}, false, false)
		Expect(err).NotTo(HaveOccurred())

		paths := runCollectInfo(params)

		Expect(paths).To(ContainElement(ContainSubstring(
			filepath.Join(collectinfo.NamespaceScopedDir, rbacNS, "configmaps", "rbac-cm.yaml"))))
		Expect(paths).To(ContainElement(ContainSubstring(
			filepath.Join(collectinfo.NamespaceScopedDir, rbacNS, "services", "rbac-svc.yaml"))))
		By("Not collecting any cluster-scoped resources when --cluster-scope=false")
		Expect(paths).NotTo(ContainElement(ContainSubstring(collectinfo.ClusterScopedDir)))
	})

	It("collects multiple namespaces with --cluster-scope=false", func() {
		params, err := testutils.NewTestParams(
			testCtx, restrictedClient, restrictedClientSet, []string{rbacNS, rbacNS2}, false, false)
		Expect(err).NotTo(HaveOccurred())

		paths := runCollectInfo(params)

		Expect(paths).To(ContainElement(ContainSubstring(
			filepath.Join(collectinfo.NamespaceScopedDir, rbacNS, "configmaps"))))
		Expect(paths).To(ContainElement(ContainSubstring(
			filepath.Join(collectinfo.NamespaceScopedDir, rbacNS2, "configmaps"))))
	})

	It("works for a pure namespace-scoped SA that cannot even GET a namespace", func() {
		By("Confirming the SA can neither get nor list namespaces")
		Expect(apierrors.IsForbidden(
			noNsPermClient.List(testCtx, &corev1.NamespaceList{}))).To(BeTrue())
		Expect(apierrors.IsForbidden(
			noNsPermClient.Get(testCtx, client.ObjectKey{Name: rbacNS}, &corev1.Namespace{}))).To(BeTrue())

		// GET is forbidden, so existence validation is skipped and the namespace is
		// used as-is; collection then proceeds with the namespaced reads it does have.
		params, err := testutils.NewTestParams(
			testCtx, noNsPermClient, noNsPermClientSet, []string{rbacNS}, false, false)
		Expect(err).NotTo(HaveOccurred())

		paths := runCollectInfo(params)
		Expect(paths).To(ContainElement(ContainSubstring(
			filepath.Join(collectinfo.NamespaceScopedDir, rbacNS, "configmaps"))))
	})

	// ---------- Dark paths ----------

	It("fails collection when --cluster-scope=true but the SA lacks cluster permissions", func() {
		params, err := testutils.NewTestParams(
			testCtx, restrictedClient, restrictedClientSet, []string{rbacNS}, false, true)
		Expect(err).NotTo(HaveOccurred()) // validation via GET still succeeds

		out := GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(out, collectinfo.RootOutputDir), os.ModePerm)).To(Succeed())

		err = collectinfo.CollectInfo(testCtx, params, out)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsForbidden(err)).To(BeTrue())
	})

	It("fails at parameter creation when -A is used (still needs list-namespaces)", func() {
		_, err := testutils.NewTestParams(
			testCtx, restrictedClient, restrictedClientSet, nil, true, false)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsForbidden(err)).To(BeTrue())
	})

	It("fails when all -n namespaces are missing", func() {
		_, err := testutils.NewTestParams(
			testCtx, restrictedClient, restrictedClientSet, []string{"missing-one", "missing-two"}, false, false)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("all given namespaces are not present"))
	})

	It("drops a missing namespace and collects the existing one", func() {
		params, err := testutils.NewTestParams(
			testCtx, restrictedClient, restrictedClientSet, []string{rbacNS, "does-not-exist"}, false, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(params.Namespaces.UnsortedList()).To(ConsistOf(rbacNS))

		paths := runCollectInfo(params)
		Expect(paths).To(ContainElement(ContainSubstring(
			filepath.Join(collectinfo.NamespaceScopedDir, rbacNS, "configmaps"))))
		Expect(paths).NotTo(ContainElement(ContainSubstring(
			filepath.Join(collectinfo.NamespaceScopedDir, "does-not-exist"))))
	})

	It("fails collection when the SA cannot list namespaced resources", func() {
		// missingReadsSA can validate the namespace (get) but has no read on
		// namespaced resources, so the first namespaced List in CollectInfo is denied.
		params, err := testutils.NewTestParams(
			testCtx, missingReadsClient, missingReadsCS, []string{rbacNS}, false, false)
		Expect(err).NotTo(HaveOccurred())

		out := GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(out, collectinfo.RootOutputDir), os.ModePerm)).To(Succeed())

		err = collectinfo.CollectInfo(testCtx, params, out)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsForbidden(err)).To(BeTrue())
	})
})

// runCollectInfo runs CollectInfo into a fresh temp dir and returns the list of
// regular-file paths inside the produced tar archive.
func runCollectInfo(params *configuration.Parameters) []string {
	out := GinkgoT().TempDir()
	Expect(os.MkdirAll(filepath.Join(out, collectinfo.RootOutputDir), os.ModePerm)).To(Succeed())

	params.Logger = collectinfo.AttachFileLogger(params.Logger,
		filepath.Join(out, collectinfo.RootOutputDir, collectinfo.LogFileName))

	Expect(collectinfo.CollectInfo(testCtx, params, out)).To(Succeed())

	paths, err := tarFilePaths(filepath.Join(out, collectinfo.TarName))
	Expect(err).NotTo(HaveOccurred())

	return paths
}

// tarFilePaths returns the names of all regular files in the gzipped tar archive.
func tarFilePaths(tarPath string) ([]string, error) {
	f, err := os.Open(tarPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gzf, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}

	var paths []string

	tr := tar.NewReader(gzf)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}

		if header.Typeflag == tar.TypeReg {
			paths = append(paths, header.Name)
		}
	}

	return paths, nil
}

// seedNamespacedObjects creates a small set of namespace-scoped objects so that
// collection has something to capture in the given namespace.
func seedNamespacedObjects(ns string) {
	Expect(k8sClient.Create(testCtx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "rbac-cm", Namespace: ns},
		Data:       map[string]string{},
	})).To(Succeed())

	Expect(k8sClient.Create(testCtx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "rbac-svc", Namespace: ns},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3000}}},
	})).To(Succeed())

	Expect(k8sClient.Create(testCtx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "rbac-pod", Namespace: ns},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "c", Image: "nginx"}},
		},
	})).To(Succeed())
}

// bindClusterRoleInNamespace grants a ClusterRole's permissions to a SA, scoped to
// a single namespace, via a RoleBinding.
func bindClusterRoleInNamespace(clusterRole, sa, ns string) {
	Expect(k8sClient.Create(testCtx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: clusterRole + "-" + sa, Namespace: ns},
		Subjects: []rbacv1.Subject{{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      sa,
			Namespace: rbacNS,
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     clusterRole,
		},
	})).To(Succeed())
}

// bindClusterRoleClusterWide grants a ClusterRole's permissions to a SA cluster-wide
// via a ClusterRoleBinding.
func bindClusterRoleClusterWide(clusterRole, sa, bindingName string) {
	Expect(k8sClient.Create(testCtx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: bindingName},
		Subjects: []rbacv1.Subject{{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      sa,
			Namespace: rbacNS,
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     clusterRole,
		},
	})).To(Succeed())
}

// impersonatingClients builds the controller-runtime client and typed clientset
// collectinfo needs, both impersonating the given ServiceAccount so the API server
// enforces that SA's RBAC.
func impersonatingClients(saNS, saName string) (client.Client, *kubernetes.Clientset) {
	impCfg := rest.CopyConfig(cfg)
	impCfg.Impersonate = rest.ImpersonationConfig{
		UserName: fmt.Sprintf("system:serviceaccount:%s:%s", saNS, saName),
	}

	scheme := runtime.NewScheme()
	Expect(clientscheme.AddToScheme(scheme)).To(Succeed())

	c, err := client.New(impCfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())

	cs, err := kubernetes.NewForConfig(impCfg)
	Expect(err).NotTo(HaveOccurred())

	return c, cs
}
