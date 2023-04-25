package collectinfo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
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
	Logger          *zap.Logger
	CurrentTime     = time.Now().Format("01-02-2006")
	tarName         = RootOutputDir + "-" + CurrentTime + ".tar.gzip"
	gvkListNSScoped = []schema.GroupVersionKind{
		corev1.SchemeGroupVersion.WithKind("PersistentVolumeClaim"),
		appsv1.SchemeGroupVersion.WithKind("StatefulSet"),
		{
			Group:   "asdb.aerospike.com",
			Version: "v1beta1",
			Kind:    "AerospikeCluster",
		},
	}
	gvkListClusterScoped = []schema.GroupVersionKind{
		corev1.SchemeGroupVersion.WithKind("Node"),
		corev1.SchemeGroupVersion.WithKind("Event"),
		{
			Group:   "storage.k8s.io",
			Version: "v1",
			Kind:    "StorageClass",
		},
	}
)

func CollectInfoUtil(namespaces []string, path string) {
	k8sClient, clientSet, err := createKubeClients(config.GetConfigOrDie())
	if err != nil {
		Logger.Error("Not able to create kube clients", zap.Error(err))
	}

	if err := CollectInfo(k8sClient, clientSet, namespaces, path); err != nil {
		Logger.Error("Not able to collect object info", zap.Error(err))
	}
}

func initializeLogger(logFilePath string) {
	cfg := zap.NewProductionEncoderConfig()
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewJSONEncoder(cfg)
	consoleEncoder := zapcore.NewConsoleEncoder(cfg)
	logFile, _ := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	writer := zapcore.AddSync(logFile)
	defaultLogLevel := zapcore.DebugLevel
	core := zapcore.NewTee(
		zapcore.NewCore(fileEncoder, writer, defaultLogLevel),
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), defaultLogLevel),
	)
	Logger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
}

func CollectInfo(k8sClient client.Client, clientSet *kubernetes.Clientset, namespaces []string,
	path string) error {
	rootOutputPath := filepath.Join(path, RootOutputDir)
	if err := os.MkdirAll(rootOutputPath, os.ModePerm); err != nil {
		return err
	}

	initializeLogger(filepath.Join(rootOutputPath, fileName))

	nsList := namespaces
	if len(nsList) == 0 {
		Logger.Info("Namespace list is not provided, hence capturing for all namespaces")

		namespaceObjs := &corev1.NamespaceList{}
		if err := k8sClient.List(context.TODO(), namespaceObjs); err != nil {
			return err
		}

		for idx := range namespaceObjs.Items {
			nsList = append(nsList, namespaceObjs.Items[idx].Name)
		}
	}

	for _, ns := range nsList {
		if err := capturePodLogs(clientSet, ns, rootOutputPath); err != nil {
			return err
		}

		for _, gvk := range gvkListNSScoped {
			if err := captureObject(k8sClient, gvk, ns, rootOutputPath); err != nil {
				return err
			}
		}
	}

	for _, gvk := range gvkListClusterScoped {
		if err := captureObject(k8sClient, gvk, "", rootOutputPath); err != nil {
			return err
		}
	}

	Logger.Info("Compressing and deleting all logs and created ", zap.String("tar file", tarName))

	return makeTarAndClean(path)
}

func createKubeClients(cfg *rest.Config) (client.Client, *kubernetes.Clientset, error) {
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

func captureObject(k8sClient client.Client, gvk schema.GroupVersionKind, ns, rootOutputPath string) error {
	listOps := &client.ListOptions{Namespace: ns}
	u := &unstructured.UnstructuredList{}

	nsPrefix := ns
	if ns != "" {
		nsPrefix = ns + "-"
	}

	u.SetGroupVersionKind(gvk)

	if err := k8sClient.List(context.TODO(), u, listOps); err != nil {
		Logger.Error("Not able to list ", zap.String("object", gvk.Kind), zap.Error(err))
		return nil
	}

	objOutputDir := filepath.Join(rootOutputPath, gvk.Kind)
	if err := os.MkdirAll(objOutputDir, os.ModePerm); err != nil {
		return err
	}

	for idx := range u.Items {
		clusterData, err := yaml.Marshal(u.Items[idx])
		if err != nil {
			return err
		}

		fileName := filepath.Join(objOutputDir,
			nsPrefix+u.Items[idx].GetName()+".yaml")

		if err := populateScraperDir(clusterData, fileName); err != nil {
			return err
		}
	}

	Logger.Info("Successfully saved ", zap.String("object", gvk.Kind),
		zap.Int("no of objects", len(u.Items)), zap.String("namespace", ns))

	return nil
}

func makeTarAndClean(pathToStore string) error {
	var buf bytes.Buffer

	if err := compress(pathToStore, &buf); err != nil {
		return err
	}

	// write the .tar.gzip
	fileToWrite, err := os.OpenFile(filepath.Join(pathToStore, tarName),
		os.O_CREATE|os.O_RDWR, 0650) //nolint:gocritic // file permission
	if err != nil {
		return err
	}

	if _, err := io.Copy(fileToWrite, &buf); err != nil {
		return err
	}

	return os.RemoveAll(filepath.Join(pathToStore, RootOutputDir))
}

func capturePodLogs(clientSet *kubernetes.Clientset, ns, rootOutputPath string) error {
	pods, err := clientSet.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		Logger.Error("Not able to list ", zap.String("object", "Pod"), zap.Error(err))
		return nil
	}

	podLogsDir := filepath.Join(rootOutputPath, "Pod", "logs")
	if err := os.MkdirAll(podLogsDir, os.ModePerm); err != nil {
		return err
	}

	for podIndex := range pods.Items {
		podData, err := yaml.Marshal(pods.Items[podIndex])
		if err != nil {
			return err
		}

		fileName := filepath.Join(podLogsDir, "..", ns+"-"+pods.Items[podIndex].Name+".yaml")

		if err := populateScraperDir(podData, fileName); err != nil {
			return err
		}

		for containerIndex := range pods.Items[podIndex].Spec.Containers {
			containerName := pods.Items[podIndex].Spec.Containers[containerIndex].Name
			if err := captureContainerLogs(clientSet, pods.Items[podIndex].Name, containerName, ns, podLogsDir); err != nil {
				return err
			}
		}

		for initContainerIndex := range pods.Items[podIndex].Spec.InitContainers {
			initContainerName := pods.Items[podIndex].Spec.InitContainers[initContainerIndex].Name
			if err := captureContainerLogs(clientSet, pods.Items[podIndex].Name, initContainerName, ns, podLogsDir); err != nil {
				return err
			}
		}
	}

	Logger.Info("Successfully saved ", zap.String("object", "Pod"),
		zap.Int("no of objects", len(pods.Items)), zap.String("namespace", ns))

	return nil
}

func captureContainerLogs(clientSet *kubernetes.Clientset, podName, containerName, ns, podLogsDir string) error {
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

	fileName := filepath.Join(podLogsDir, ns+"-"+podName+"-"+containerName+"-current.log")

	return populateScraperDir(buf.Bytes(), fileName)
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