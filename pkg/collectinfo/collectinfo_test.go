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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/collectinfo"
)

const (
	nodeName             = "test-node"
	scName               = "test-sc"
	pvcName              = "test-pvc"
	stsName              = "test-sts"
	podName              = "test-pod"
	containerName        = "test-container"
	aerospikeClusterName = "test-aerocluster"
)

var (
	clusterScopeDir   = filepath.Join(collectinfo.RootOutputDir, "k8s-cluster")
	namespaceScopeDir = filepath.Join(collectinfo.RootOutputDir, "k8s-namespaces")
)

// key format: RootOutputDir/<k8s-cluster or k8s-namespaces>/ns/<objectKIND>/<objectName>
var filesList = map[string]bool{
	filepath.Join(clusterScopeDir, collectinfo.NodeKind,
		nodeName+collectinfo.FilePrefix): false,
	filepath.Join(clusterScopeDir, collectinfo.SCKind,
		scName+collectinfo.FilePrefix): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.PVCKind,
		pvcName+collectinfo.FilePrefix): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.STSKind,
		stsName+collectinfo.FilePrefix): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.PodKind, "logs",
		podName+"-"+containerName+"-current.log"): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.PodKind,
		podName+collectinfo.FilePrefix): false,
	filepath.Join(namespaceScopeDir, namespace, collectinfo.AerospikeClusterKind,
		aerospikeClusterName+collectinfo.FilePrefix): false,
	filepath.Join(collectinfo.RootOutputDir,
		collectinfo.FileName): false,
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

			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: namespace},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			}
			err = k8sClient.Create(context.TODO(), pvc, createOption)
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

			gvk := schema.GroupVersionKind{
				Group:   "asdb.aerospike.com",
				Version: "v1beta1",
				Kind:    "AerospikeCluster",
			}

			u := &unstructured.Unstructured{}
			u.SetName(aerospikeClusterName)
			u.SetNamespace(namespace)
			u.SetGroupVersionKind(gvk)

			err = k8sClient.Create(context.TODO(), u)
			Expect(err).ToNot(HaveOccurred())

			var nslist = []string{namespace}

			err = os.MkdirAll(collectinfo.RootOutputDir, os.ModePerm)
			Expect(err).ToNot(HaveOccurred())

			logger := collectinfo.InitializeLogger(filepath.Join(collectinfo.RootOutputDir, "logFile.log"))

			err = collectinfo.CollectInfo(logger, k8sClient, k8sClientset, nslist, "", false, true)
			Expect(err).ToNot(HaveOccurred())

			err = validateTar(collectinfo.TarName, filesList)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

func validateTar(srcFile string, filesList map[string]bool) error {
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

	return nil
}
