package pkg

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	RootOutputDir = "scraperlogs"
	fileName      = "logFile.log"
)

var (
	CurrentTime = time.Now().Format("01-02-2006")
	// Key of this map should be the Kind of the object
	objKindDirMap = map[string]string{
		"Pod":                   "",
		"Event":                 "",
		"StatefulSet":           "",
		"AerospikeCluster":      "",
		"PersistentVolumeClaim": "",
		"Node":                  "",
		"StorageClass":          "",
	}
)

func CollectInfoUtil(namespaces []string, pathToStore string) error {
	k8sClient, clientSet, err := CreateKubeClients(config.GetConfigOrDie())
	if err != nil {
		logrus.Error(err)
		return err
	}

	if err := CollectInfo(k8sClient, clientSet, namespaces, pathToStore); err != nil {
		logrus.Error(err)
		return err
	}

	return nil
}

func CollectInfo(k8sClient client.Client, clientSet *kubernetes.Clientset, namespaces []string,
	pathToStore string) error {
	rootOutputPath := filepath.Join(pathToStore, RootOutputDir)
	if err := os.MkdirAll(rootOutputPath, os.ModePerm); err != nil {
		return err
	}

	// open log file
	logFile, fileErr := os.OpenFile(filepath.Join(rootOutputPath, fileName),
		os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644) //nolint:gocritic // file permission
	if fileErr != nil {
		return fileErr
	}

	logrus.SetOutput(logFile)

	if err := createDirStructure(rootOutputPath); err != nil {
		return err
	}

	logrus.Info("Directory structure created at ", rootOutputPath)

	nsList := namespaces
	if len(nsList) == 0 {
		logrus.Info("namespace list is not provided, hence capturing for all namespaces")

		namespaceObjs := &corev1.NamespaceList{}
		if err := k8sClient.List(context.TODO(), namespaceObjs); err != nil {
			logrus.Error(err)
			return err
		}

		for idx := range namespaceObjs.Items {
			nsList = append(nsList, namespaceObjs.Items[idx].Name)
		}
	}

	for _, ns := range nsList {
		if err := capturePodLogs(clientSet, ns); err != nil {
			return err
		}

		if err := captureSTSConfig(k8sClient, ns); err != nil {
			return err
		}

		if err := captureAeroClusterConfig(k8sClient, ns); err != nil {
			return err
		}

		if err := capturePVCConfig(k8sClient, ns); err != nil {
			return err
		}

		if err := captureEvents(k8sClient, ns); err != nil {
			return err
		}
	}

	if err := captureNodesConfig(k8sClient); err != nil {
		return err
	}

	if err := captureSCConfig(k8sClient); err != nil {
		return err
	}

	logrus.Info("Compressing and deleting all logs and created ", RootOutputDir+CurrentTime+".tar.gzip")

	return makeTarAndClean(pathToStore)
}

func CreateKubeClients(cfg *rest.Config) (client.Client, *kubernetes.Clientset, error) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	k8sClient, err := client.New(
		cfg, client.Options{Scheme: scheme},
	)
	if err != nil {
		return nil, nil, err
	}

	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, nil, err
	}

	return k8sClient, clientSet, nil
}

func captureSCConfig(k8sClient client.Client) error {
	gvk := schema.GroupVersionKind{
		Group:   "storage.k8s.io",
		Version: "v1",
		Kind:    "StorageClass",
	}

	return captureObject(k8sClient, gvk, "")
}

func captureNodesConfig(k8sClient client.Client) error {
	gvk := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Node",
	}

	return captureObject(k8sClient, gvk, "")
}

func captureEvents(k8sClient client.Client, ns string) error {
	gvk := schema.GroupVersionKind{
		Group:   "events.k8s.io",
		Version: "v1",
		Kind:    "Event",
	}

	return captureObject(k8sClient, gvk, ns)
}

func captureObject(k8sClient client.Client, gvk schema.GroupVersionKind, ns string) error {
	listOps := &client.ListOptions{Namespace: ns}
	u := &unstructured.UnstructuredList{}

	nsPrefix := ns
	if ns != "" {
		nsPrefix = ns + "-"
	}

	u.SetGroupVersionKind(gvk)

	if err := k8sClient.List(context.TODO(), u, listOps); err != nil {
		return err
	}

	for idx := range u.Items {
		clusterData, err := yaml.Marshal(u.Items[idx])
		if err != nil {
			return err
		}

		fileName := filepath.Join(objKindDirMap[gvk.Kind],
			nsPrefix+fmt.Sprintf("%v", u.Items[idx].Object["metadata"].(map[string]interface{})["name"])+".yaml")

		if err := populateScraperDir(clusterData, fileName); err != nil {
			return err
		}
	}

	logrus.Info("Successfully saved ", gvk.Kind, " namespace ", ns)

	return nil
}

func makeTarAndClean(pathToStore string) error {
	var buf bytes.Buffer

	if err := compress(pathToStore, &buf); err != nil {
		return err
	}

	// write the .tar.gzip
	fileToWrite, err := os.OpenFile(filepath.Join(pathToStore, RootOutputDir+"-"+CurrentTime+".tar.gzip"),
		os.O_CREATE|os.O_RDWR, 0650) //nolint:gocritic // file permission
	if err != nil {
		return err
	}

	if _, err := io.Copy(fileToWrite, &buf); err != nil {
		return err
	}

	if err := os.RemoveAll(filepath.Join(pathToStore, RootOutputDir)); err != nil {
		return err
	}

	return nil
}

func capturePVCConfig(k8sClient client.Client, ns string) error {
	gvk := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "PersistentVolumeClaim",
	}

	return captureObject(k8sClient, gvk, ns)
}

func captureAeroClusterConfig(k8sClient client.Client, ns string) error {
	gvk := schema.GroupVersionKind{
		Group:   "asdb.aerospike.com",
		Version: "v1beta1",
		Kind:    "AerospikeCluster",
	}

	return captureObject(k8sClient, gvk, ns)
}

func captureSTSConfig(k8sClient client.Client, ns string) error {
	gvk := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "StatefulSet",
	}

	return captureObject(k8sClient, gvk, ns)
}

func capturePodLogs(clientSet *kubernetes.Clientset, ns string) error {
	pods, err := clientSet.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for podIndex := range pods.Items {
		podData, err := yaml.Marshal(pods.Items[podIndex])
		if err != nil {
			return err
		}

		fileName := filepath.Join(objKindDirMap["Pod"], "..", ns+"-"+pods.Items[podIndex].Name+".yaml")

		if err := populateScraperDir(podData, fileName); err != nil {
			return err
		}

		for containerIndex := range pods.Items[podIndex].Spec.Containers {
			containerName := pods.Items[podIndex].Spec.Containers[containerIndex].Name
			if err := captureContainerLogs(clientSet, pods.Items[podIndex].Name, containerName, ns); err != nil {
				return err
			}
		}

		for initContainerIndex := range pods.Items[podIndex].Spec.InitContainers {
			initContainerName := pods.Items[podIndex].Spec.InitContainers[initContainerIndex].Name
			if err := captureContainerLogs(clientSet, pods.Items[podIndex].Name, initContainerName, ns); err != nil {
				return err
			}
		}
	}

	logrus.Info("Captured pods logs for namespace ", ns)

	return nil
}

func captureContainerLogs(clientSet *kubernetes.Clientset, podName, containerName, ns string) error {
	podLogOpts := corev1.PodLogOptions{Container: containerName}
	req := clientSet.CoreV1().Pods(ns).GetLogs(podName, &podLogOpts)

	podLogs, err := req.Stream(context.TODO())
	if err != nil {
		return err
	}

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, podLogs); err != nil {
		return err
	}

	if err := podLogs.Close(); err != nil {
		return err
	}

	fileName := filepath.Join(objKindDirMap["Pod"], ns+"-"+podName+"-"+containerName+"-current.log")

	return populateScraperDir(buf.Bytes(), fileName)
}

func createDirStructure(rootOutputPath string) error {
	for obj := range objKindDirMap {
		if obj == "Pod" {
			objKindDirMap[obj] = filepath.Join(rootOutputPath, obj, "logs")
		} else {
			objKindDirMap[obj] = filepath.Join(rootOutputPath, obj)
		}

		if err := os.MkdirAll(objKindDirMap[obj], os.ModePerm); err != nil {
			return err
		}
	}

	return nil
}

func populateScraperDir(data []byte, fileName string) error {
	fileName = filepath.Clean(fileName)

	err := os.WriteFile(fileName, data, 0600) //nolint:gocritic // file permission
	if err != nil {
		return err
	}

	return nil
}

func compress(src string, buf io.Writer) error {
	// tar > gzip > buf
	zr := gzip.NewWriter(buf)
	tw := tar.NewWriter(zr)
	// walk through every file in the folder
	rootOutputPath := filepath.Join(src, RootOutputDir)
	err := filepath.Walk(rootOutputPath, func(file string, fi os.FileInfo, err error) error {
		// generate tar header
		header, fileErr := tar.FileInfoHeader(fi, file)
		if fileErr != nil {
			return fileErr
		}

		// must provide real name
		// (see https://golang.org/src/archive/tar/common.go?#L626)

		header.Name = strings.TrimPrefix(file, src)
		// write header
		if fileErr := tw.WriteHeader(header); fileErr != nil {
			return fileErr
		}
		// if not a dir, write file content
		if !fi.IsDir() {
			data, fileErr := os.Open(file)
			if fileErr != nil {
				return fileErr
			}
			if _, fileErr := io.Copy(tw, data); fileErr != nil {
				return fileErr
			}
		}

		return nil
	})

	if err != nil {
		return err
	}
	// produce tar
	if err := tw.Close(); err != nil {
		return err
	}
	// produce gzip
	return zr.Close()
}
