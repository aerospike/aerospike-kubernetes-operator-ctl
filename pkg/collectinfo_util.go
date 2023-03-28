package pkg

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"github.com/sirupsen/logrus"
	"io"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	asdbv1beta1 "github.com/aerospike/aerospike-kubernetes-operator/api/v1beta1"
)

var (
	rootOutputDir         = "scraperlogs"
	currentTime           = time.Now().Format("01-02-2006")
	logsDirectoryPod      string
	eventLogsDirectory    string
	describeDirectorySTS  string
	describeDirectoryAero string
	describeDirectoryPVC  string
	describeDirectoryNode string
	describeDirectorySC   string
	k8sClient             client.Client
)

func CollectInfo(namespaces []string, kubeconfig, pathToStore *string) {
	fileName := "logFile.log"

	if err := os.MkdirAll(rootOutputDir, os.ModePerm); err != nil {
		panic(err.Error())
	}

	// open log file
	logFile, err := os.OpenFile(filepath.Join(*pathToStore, rootOutputDir, fileName), os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		panic(err)
	}
	defer logFile.Close()

	logrus.SetOutput(logFile)

	cfg, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		panic(err.Error())
	}

	if err := asdbv1beta1.AddToScheme(scheme); err != nil {
		panic(err.Error())
	}

	k8sClient, err = client.New(
		cfg, client.Options{Scheme: scheme},
	)
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err.Error())
	}

	if err = createDirStructure(pathToStore); err != nil {
		panic(err.Error())
	}

	logrus.Info("Directory structure created at ", *pathToStore)

	nsList := namespaces
	if len(nsList) == 0 {
		logrus.Info("namespace list is not provided, hence capturing for all namespaces")

		namespaceObjs := &corev1.NamespaceList{}
		if err := k8sClient.List(context.TODO(), namespaceObjs); err != nil {
			panic(err.Error())
		}

		for _, ns := range namespaceObjs.Items {
			nsList = append(nsList, ns.Name)
		}
	}

	for _, ns := range nsList {
		if err := capturePodLogs(ns, clientset); err != nil {
			panic(err.Error())
		}

		if err := captureSTSConfig(ns); err != nil {
			panic(err.Error())
		}

		if err := captureAeroclusterConfig(ns); err != nil {
			panic(err.Error())
		}

		if err := capturePVCConfig(ns); err != nil {
			panic(err.Error())
		}

		if err := captureEvents(ns); err != nil {
			panic(err.Error())
		}
	}

	if err := captureNodesConfig(); err != nil {
		logrus.Debug("could not capture nodes config ", err)
	}

	if err := captureSCConfig(); err != nil {
		logrus.Debug("could not capture storage-class config ", err)
	}

	if err := makeTarAndClean(pathToStore); err != nil {
		panic(err.Error())
	}
}

func captureSCConfig() error {
	scList := &v1.StorageClassList{}
	listOps := &client.ListOptions{}

	if err := k8sClient.List(context.TODO(), scList, listOps); err != nil {
		return err
	}

	for scIndex := range scList.Items {
		scData, err := json.MarshalIndent(scList.Items[scIndex], "", "	")
		if err != nil {
			panic(err.Error())
		}

		fileName := filepath.Join(describeDirectorySC, scList.Items[scIndex].Name)

		err = populateScraperDir(scData, fileName)
		if err != nil {
			panic(err.Error())
		}
	}

	logrus.Info("Captured storage-class configs")

	return nil
}

func captureNodesConfig() error {
	nodeList := &corev1.NodeList{}
	listOps := &client.ListOptions{}

	if err := k8sClient.List(context.TODO(), nodeList, listOps); err != nil {
		return err
	}

	for nodeIndex := range nodeList.Items {
		nodeData, err := json.MarshalIndent(nodeList.Items[nodeIndex], "", "	")
		if err != nil {
			panic(err.Error())
		}

		fileName := filepath.Join(describeDirectoryNode, nodeList.Items[nodeIndex].Name)

		err = populateScraperDir(nodeData, fileName)
		if err != nil {
			panic(err.Error())
		}
	}

	logrus.Info("Captured nodes config")

	return nil
}

func makeTarAndClean(pathToStore *string) error {
	var buf bytes.Buffer

	if err := compress(rootOutputDir, &buf); err != nil {
		return err
	}
	// write the .tar.gzip
	fileToWrite, err := os.OpenFile(filepath.Join(*pathToStore, rootOutputDir+currentTime+".tar.gzip"), os.O_CREATE|os.O_RDWR, 0650)
	if err != nil {
		return err
	}

	if _, err := io.Copy(fileToWrite, &buf); err != nil {
		return err
	}

	if err = os.RemoveAll(rootOutputDir); err != nil {
		return err
	}

	logrus.Info("Compressed and deleted all logs and created ", rootOutputDir+currentTime+".tar.gzip")

	return nil
}

func captureEvents(ns string) error {
	eventList := &corev1.EventList{}
	listOps := &client.ListOptions{Namespace: ns}

	if err := k8sClient.List(context.TODO(), eventList, listOps); err != nil {
		panic(err.Error())
	}

	eventData, err := json.MarshalIndent(eventList, "", "	")
	if err != nil {
		return err
	}

	fileName := filepath.Join(eventLogsDirectory, ns+"-events")

	if err = populateScraperDir(eventData, fileName); err != nil {
		return err
	}

	logrus.Info("Captured events for namespace ", ns)

	return nil
}

func capturePVCConfig(ns string) error {
	pvcList := &corev1.PersistentVolumeClaimList{}
	listOps := &client.ListOptions{Namespace: ns}

	if err := k8sClient.List(context.TODO(), pvcList, listOps); err != nil {
		panic(err.Error())
	}

	for pvcIndex := range pvcList.Items {
		pvcData, err := json.MarshalIndent(pvcList.Items[pvcIndex], "", "	")
		if err != nil {
			return err
		}

		fileName := filepath.Join(describeDirectoryPVC, ns+"-"+pvcList.Items[pvcIndex].Name)

		err = populateScraperDir(pvcData, fileName)
		if err != nil {
			return err
		}
	}

	logrus.Info("Captured PVC for namespace ", ns)

	return nil
}

func captureAeroclusterConfig(ns string) error {
	listOps := &client.ListOptions{
		Namespace: ns,
	}
	aeroClusterList := &asdbv1beta1.AerospikeClusterList{}

	if err := k8sClient.List(context.TODO(), aeroClusterList, listOps); err != nil {
		return err
	}

	for clusterIndex := range aeroClusterList.Items {
		clusterData, err := json.MarshalIndent(aeroClusterList.Items[clusterIndex], "", "	")
		if err != nil {
			return err
		}

		fileName := filepath.Join(describeDirectoryAero, ns+"-"+aeroClusterList.Items[clusterIndex].Name)

		if err = populateScraperDir(clusterData, fileName); err != nil {
			return err
		}
	}

	logrus.Info("Captured aerospike clusters for namespace ", ns)

	return nil
}

func captureSTSConfig(ns string) error {
	stsList := &appsv1.StatefulSetList{}
	listOps := &client.ListOptions{Namespace: ns}

	if err := k8sClient.List(context.TODO(), stsList, listOps); err != nil {
		panic(err.Error())
	}

	for stsIndex := range stsList.Items {
		stsData, err := json.MarshalIndent(stsList.Items[stsIndex], "", "	")
		if err != nil {
			return err
		}

		fileName := filepath.Join(describeDirectorySTS, ns+"-"+stsList.Items[stsIndex].Name)

		if err = populateScraperDir(stsData, fileName); err != nil {
			return err
		}
	}

	logrus.Info("Captured statefulsets for namespace ", ns)

	return nil
}

func capturePodLogs(ns string, clientSet *kubernetes.Clientset) error {
	pods, err := clientSet.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for podIndex := range pods.Items {
		podData, err := json.MarshalIndent(pods.Items[podIndex], "", "	")
		if err != nil {
			return err
		}

		fileName := filepath.Join(logsDirectoryPod, "..", ns+"-"+pods.Items[podIndex].Name)

		if err = populateScraperDir(podData, fileName); err != nil {
			return err
		}

		for containerIndex := range pods.Items[podIndex].Spec.Containers {
			containerName := pods.Items[podIndex].Spec.Containers[containerIndex].Name
			podLogOpts := corev1.PodLogOptions{Container: containerName}
			req := clientSet.CoreV1().Pods(ns).GetLogs(pods.Items[podIndex].Name, &podLogOpts)

			podLogs, err := req.Stream(context.TODO())
			if err != nil {
				return err
			}

			buf := new(bytes.Buffer)
			if _, err = io.Copy(buf, podLogs); err != nil {
				return err
			}

			if err = podLogs.Close(); err != nil {
				return err
			}

			fileName := filepath.Join(logsDirectoryPod, ns+"-"+pods.Items[podIndex].Name+"-"+containerName+"-current.log")

			if err = populateScraperDir(buf.Bytes(), fileName); err != nil {
				return err
			}
		}

		for initContainerIndex := range pods.Items[podIndex].Spec.InitContainers {
			initContainerName := pods.Items[podIndex].Spec.InitContainers[initContainerIndex].Name
			podLogOpts := corev1.PodLogOptions{Container: initContainerName}
			req := clientSet.CoreV1().Pods(ns).GetLogs(pods.Items[podIndex].Name, &podLogOpts)

			podLogs, err := req.Stream(context.TODO())
			if err != nil {
				return err
			}

			buf := new(bytes.Buffer)
			if _, err = io.Copy(buf, podLogs); err != nil {
				return err
			}

			if err = podLogs.Close(); err != nil {
				return err
			}

			fileName := filepath.Join(logsDirectoryPod, ns+"-"+pods.Items[podIndex].Name+"-"+initContainerName+"-current.log")

			if err = populateScraperDir(buf.Bytes(), fileName); err != nil {
				return err
			}
		}
	}

	logrus.Info("Captured pods logs for namespace ", ns)

	return nil
}

func createDirStructure(pathToStore *string) error {
	rootOutputDir = filepath.Join(*pathToStore, rootOutputDir)
	logsDirectoryPod = filepath.Join(rootOutputDir, "Pod", "logs")
	eventLogsDirectory = filepath.Join(rootOutputDir, "Events")
	describeDirectorySTS = filepath.Join(rootOutputDir, "STS")
	describeDirectoryAero = filepath.Join(rootOutputDir, "AeroCluster")
	describeDirectoryPVC = filepath.Join(rootOutputDir, "PVC")
	describeDirectoryNode = filepath.Join(rootOutputDir, "Nodes")
	describeDirectorySC = filepath.Join(rootOutputDir, "StorageClasses")

	if err := os.MkdirAll(logsDirectoryPod, os.ModePerm); err != nil {
		return err
	}

	if err := os.MkdirAll(eventLogsDirectory, os.ModePerm); err != nil {
		return err
	}

	if err := os.MkdirAll(describeDirectorySTS, os.ModePerm); err != nil {
		return err
	}

	if err := os.MkdirAll(describeDirectoryAero, os.ModePerm); err != nil {
		return err
	}

	if err := os.MkdirAll(describeDirectoryPVC, os.ModePerm); err != nil {
		return err
	}

	if err := os.MkdirAll(describeDirectoryNode, os.ModePerm); err != nil {
		return err
	}

	if err := os.MkdirAll(describeDirectorySC, os.ModePerm); err != nil {
		return err
	}

	return nil
}

func populateScraperDir(data []byte, fileName string) error {
	fileName = filepath.Clean(fileName)

	filePtr, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}

	bufferedWriter := bufio.NewWriter(filePtr)

	if _, err = bufferedWriter.Write(data); err != nil {
		return err
	}

	if err = bufferedWriter.Flush(); err != nil {
		return err
	}

	if err = filePtr.Close(); err != nil {
		return err
	}

	return nil
}

func compress(src string, buf io.Writer) error {
	// tar > gzip > buf
	zr := gzip.NewWriter(buf)
	tw := tar.NewWriter(zr)
	// walk through every file in the folder
	err := filepath.Walk(src, func(file string, fi os.FileInfo, err error) error {
		// generate tar header
		header, err := tar.FileInfoHeader(fi, file)
		if err != nil {
			return err
		}

		// must provide real name
		// (see https://golang.org/src/archive/tar/common.go?#L626)

		header.Name = filepath.ToSlash(file)
		// write header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		// if not a dir, write file content
		if !fi.IsDir() {
			data, err := os.Open(file)
			if err != nil {
				return err
			}
			if _, err := io.Copy(tw, data); err != nil {
				return err
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
	if err := zr.Close(); err != nil {
		return err
	}

	return nil
}

/*func getOLMOrHelmVersion() string {
	csvList := &v1alpha1.ClusterServiceVersionList{}
	fieldSelector := fields.OneTermEqualSelector("spec.customresourcedefinitions.owned.kind", "AerospikeCluster")

	listOps := &client.ListOptions{FieldSelector: fieldSelector}

	if err := k8sClient.List(context.TODO(), csvList, listOps); err != nil {
		panic(err.Error())
	}

	if len(csvList.Items) != 0 {
		logrus.Info("operator is installed by olm")

	}
}*/
