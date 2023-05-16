package collectinfo

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

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/duration"
	runtimeresource "k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/kube-openapi/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	RootOutputDir           = "akoctl_collectinfo"
	NamespaceScopedDir      = "k8s_namespaces"
	ClusterScopedDir        = "k8s_cluster"
	LogFileName             = "akoctl.log"
	FileSuffix              = ".yaml"
	MutatingWebhookPrefix   = "maerospikecluster.kb.io-"
	ValidatingWebhookPrefix = "vaerospikecluster.kb.io-"
	MutatingWebhookName     = "aerospike-operator-mutating-webhook-configuration"
	ValidatingWebhookName   = "aerospike-operator-validating-webhook-configuration"
)

var (
	currentTime = time.Now().Format("20060102_150405")
	TarName     = RootOutputDir + "_" + currentTime + ".tar.gzip"
)

func RunCollectInfo(namespaces []string, path string, allNamespaces, clusterScope bool) error {
	rootOutputPath := filepath.Join(path, RootOutputDir)
	if err := os.Mkdir(rootOutputPath, os.ModePerm); err != nil {
		return err
	}

	logger := InitializeLogger(filepath.Join(rootOutputPath, LogFileName))

	if len(namespaces) == 0 && !allNamespaces {
		logger.Error("Either `namespaces` or `all-namespaces` argument must be provided")
		return nil
	}

	k8sClient, clientSet, err := createKubeClients(config.GetConfigOrDie())
	if err != nil {
		logger.Error("Not able to create kube clients", zap.Error(err))
		return err
	}

	if err := CollectInfo(logger, k8sClient, clientSet, namespaces, path, allNamespaces, clusterScope); err != nil {
		logger.Error("Not able to collect object info", zap.String("err", err.Error()))
	}

	return nil
}

func InitializeLogger(logFilePath string) *zap.Logger {
	cfg := zap.NewProductionEncoderConfig()
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewJSONEncoder(cfg)
	consoleEncoder := zapcore.NewConsoleEncoder(cfg)
	logFile, _ := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gocritic // file permission
	writer := zapcore.AddSync(logFile)
	defaultLogLevel := zapcore.DebugLevel
	core := zapcore.NewTee(
		zapcore.NewCore(fileEncoder, writer, defaultLogLevel),
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), defaultLogLevel),
	)

	return zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.DPanicLevel))
}

func CollectInfo(logger *zap.Logger, k8sClient client.Client, clientSet *kubernetes.Clientset, namespaces []string,
	path string, allNamespaces, clusterScope bool) error {
	rootOutputPath := filepath.Join(path, RootOutputDir)
	ctx := context.TODO()
	nsList := sets.String{}
	nsList.Insert(namespaces...)

	if allNamespaces {
		logger.Info("Capturing for all namespaces")

		namespaceObjs := &corev1.NamespaceList{}
		if err := k8sClient.List(ctx, namespaceObjs); err != nil {
			return err
		}

		for idx := range namespaceObjs.Items {
			nsList.Insert(namespaceObjs.Items[idx].Name)
		}
	}

	for ns := range nsList {
		objOutputDir := filepath.Join(rootOutputPath, NamespaceScopedDir, ns)
		if err := os.MkdirAll(objOutputDir, os.ModePerm); err != nil {
			return err
		}

		if err := capturePodLogs(ctx, logger, clientSet, ns, objOutputDir); err != nil {
			return err
		}

		if err := captureEvents(ctx, logger, clientSet, ns, objOutputDir); err != nil {
			return err
		}

		for _, gvk := range gvkListNSScoped {
			if err := captureObject(logger, k8sClient, gvk, ns, objOutputDir); err != nil {
				return err
			}
		}
	}

	if clusterScope {
		logger.Info("Capturing cluster scoped objects info")

		objOutputDir := filepath.Join(rootOutputPath, ClusterScopedDir)
		if err := os.MkdirAll(objOutputDir, os.ModePerm); err != nil {
			return err
		}

		for _, gvk := range gvkListClusterScoped {
			if err := captureObject(logger, k8sClient, gvk, "", objOutputDir); err != nil {
				return err
			}
		}

		for _, webhooksGVK := range gvkListWebhooks {
			if err := captureWebhooks(logger, k8sClient, webhooksGVK, objOutputDir); err != nil {
				return err
			}
		}
	}

	logger.Info("Compressing and deleting all logs and created ", zap.String("tar file", TarName))

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

func captureObject(logger *zap.Logger, k8sClient client.Client, gvk schema.GroupVersionKind,
	ns, rootOutputPath string) error {
	listOps := &client.ListOptions{Namespace: ns}
	u := &unstructured.UnstructuredList{}

	u.SetGroupVersionKind(gvk)

	if err := k8sClient.List(context.TODO(), u, listOps); err != nil {
		logger.Error("Not able to list ", zap.String("object", gvk.Kind), zap.Error(err))
		return err
	}

	objOutputDir := filepath.Join(rootOutputPath, KindDirNames[gvk.Kind])
	if err := os.MkdirAll(objOutputDir, os.ModePerm); err != nil {
		return err
	}

	for idx := range u.Items {
		clusterData, err := yaml.Marshal(u.Items[idx])
		if err != nil {
			return err
		}

		fileName := filepath.Join(objOutputDir,
			u.Items[idx].GetName()+FileSuffix)

		if err := populateScraperDir(clusterData, fileName); err != nil {
			return err
		}
	}

	logger.Info("Successfully saved ", zap.String("object", gvk.Kind),
		zap.Int("no of objects", len(u.Items)), zap.String("namespace", ns))

	return nil
}

func makeTarAndClean(pathToStore string) error {
	var buf bytes.Buffer

	if err := compress(pathToStore, &buf); err != nil {
		return err
	}

	// write the .tar.gzip
	fileToWrite, err := os.OpenFile(filepath.Join(pathToStore, TarName),
		os.O_CREATE|os.O_RDWR, 0650) //nolint:gocritic // file permission
	if err != nil {
		return err
	}

	if _, err := io.Copy(fileToWrite, &buf); err != nil {
		return err
	}

	return os.RemoveAll(filepath.Join(pathToStore, RootOutputDir))
}

func capturePodLogs(ctx context.Context, logger *zap.Logger, clientSet *kubernetes.Clientset, ns,
	rootOutputPath string) error {
	pods, err := clientSet.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Error("Not able to list ", zap.String("object", PodKind), zap.Error(err))
		return err
	}

	for podIndex := range pods.Items {
		podData, err := yaml.Marshal(pods.Items[podIndex])
		if err != nil {
			return err
		}

		podLogsDir := filepath.Join(rootOutputPath, KindDirNames[PodKind], pods.Items[podIndex].Name, "logs")
		if err := os.MkdirAll(podLogsDir, os.ModePerm); err != nil {
			return err
		}

		fileName := filepath.Join(podLogsDir, "..", pods.Items[podIndex].Name+FileSuffix)

		if err := populateScraperDir(podData, fileName); err != nil {
			return err
		}

		for containerIndex := range pods.Items[podIndex].Spec.Containers {
			containerName := pods.Items[podIndex].Spec.Containers[containerIndex].Name
			if err := captureContainerLogs(logger, clientSet, pods.Items[podIndex].Name, containerName, ns,
				podLogsDir, false); err != nil {
				return err
			}

			if err := captureContainerLogs(logger, clientSet, pods.Items[podIndex].Name, containerName, ns,
				podLogsDir, true); err != nil {
				return err
			}
		}

		for initContainerIndex := range pods.Items[podIndex].Spec.InitContainers {
			initContainerName := pods.Items[podIndex].Spec.InitContainers[initContainerIndex].Name
			if err := captureContainerLogs(logger, clientSet, pods.Items[podIndex].Name, initContainerName, ns,
				podLogsDir, false); err != nil {
				return err
			}

			if err := captureContainerLogs(logger, clientSet, pods.Items[podIndex].Name, initContainerName, ns,
				podLogsDir, true); err != nil {
				return err
			}
		}
	}

	logger.Info("Successfully saved ", zap.String("object", PodKind),
		zap.Int("no of objects", len(pods.Items)), zap.String("namespace", ns))

	return nil
}

func captureContainerLogs(logger *zap.Logger, clientSet *kubernetes.Clientset, podName, containerName, ns,
	podLogsDir string, previous bool) error {
	podLogOpts := corev1.PodLogOptions{
		Container: containerName,
		Previous:  previous,
	}
	req := clientSet.CoreV1().Pods(ns).GetLogs(podName, &podLogOpts)

	podLogs, reqErr := req.Stream(context.TODO())
	if reqErr != nil {
		logger.Error("Container's logs not found ", zap.String("container", containerName),
			zap.Bool("previous", previous), zap.Error(reqErr))
		return nil
	}

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, podLogs); err != nil {
		return err
	}

	if err := podLogs.Close(); err != nil {
		return err
	}

	if previous {
		podLogsDir = filepath.Join(podLogsDir, "previous")
		if err := os.MkdirAll(podLogsDir, os.ModePerm); err != nil {
			return err
		}
	}

	fileName := filepath.Join(podLogsDir, containerName+".log")

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

func appendOneEvent(data *[]byte, e *corev1.Event) {
	event := fmt.Sprintf("%s\t%s\t%s\t%s/%s\t%v\n", getInterval(e), e.Type, e.Reason, e.InvolvedObject.Kind,
		e.InvolvedObject.Name, strings.TrimSpace(e.Message))
	*data = append(*data, event...)
}

func captureEvents(ctx context.Context, logger *zap.Logger, clientSet *kubernetes.Clientset, namespace,
	rootOutputPath string) error {
	listOptions := metav1.ListOptions{Limit: 500}

	e := clientSet.CoreV1().Events(namespace)
	el := &corev1.EventList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "EventList",
			APIVersion: "v1",
		},
	}
	err := runtimeresource.FollowContinue(&listOptions,
		func(options metav1.ListOptions) (runtime.Object, error) {
			newEvents, err := e.List(ctx, options)
			if err != nil {
				return nil, runtimeresource.EnhanceListError(err, options, "events")
			}
			el.Items = append(el.Items, newEvents.Items...)
			return newEvents, nil
		})

	if err != nil {
		return err
	}

	if len(el.Items) == 0 {
		logger.Info("No events found in namespace", zap.String("namespace", namespace))
		return nil
	}

	data := []byte("LAST SEEN\tTYPE\tREASON\tOBJECT\tMESSAGE\n")
	for idx := range el.Items {
		appendOneEvent(&data, &el.Items[idx])
	}

	objOutputDir := filepath.Join(rootOutputPath, KindDirNames[EventKind])
	if err := os.MkdirAll(objOutputDir, os.ModePerm); err != nil {
		return err
	}

	fileName := filepath.Join(objOutputDir, KindDirNames[EventKind]+".log")

	if err := populateScraperDir(data, fileName); err != nil {
		return err
	}

	logger.Info("Successfully saved ", zap.String("object", "Events"),
		zap.Int("no of objects", len(el.Items)), zap.String("namespace", namespace))

	return nil
}

func getInterval(e *corev1.Event) string {
	var interval string

	firstTimestampSince := translateMicroTimestampSince(e.EventTime)
	if e.EventTime.IsZero() {
		firstTimestampSince = translateTimestampSince(e.FirstTimestamp)
	}

	switch {
	case e.Series != nil:
		interval = fmt.Sprintf("%s (x%d over %s)", translateMicroTimestampSince(e.Series.LastObservedTime),
			e.Series.Count, firstTimestampSince)

	case e.Count > 1:
		interval = fmt.Sprintf("%s (x%d over %s)", translateTimestampSince(e.LastTimestamp), e.Count, firstTimestampSince)

	default:
		interval = firstTimestampSince
	}

	return interval
}

// translateMicroTimestampSince returns the elapsed time since timestamp in
// human-readable approximation.
func translateMicroTimestampSince(timestamp metav1.MicroTime) string {
	if timestamp.IsZero() {
		return "<unknown>"
	}

	return duration.HumanDuration(time.Since(timestamp.Time))
}

// translateTimestampSince returns the elapsed time since timestamp in
// human-readable approximation.
func translateTimestampSince(timestamp metav1.Time) string {
	if timestamp.IsZero() {
		return "<unknown>"
	}

	return duration.HumanDuration(time.Since(timestamp.Time))
}

func captureWebhooks(logger *zap.Logger, k8sClient client.Client, gvk schema.GroupVersionKind,
	rootOutputPath string) error {
	listOps := &client.ListOptions{}
	u := &unstructured.UnstructuredList{}

	u.SetGroupVersionKind(gvk)

	if err := k8sClient.List(context.TODO(), u, listOps); err != nil {
		logger.Error("Not able to list ", zap.String("object", gvk.Kind), zap.Error(err))
		return err
	}

	objOutputDir := filepath.Join(rootOutputPath, KindDirNames[gvk.Kind])
	if err := os.MkdirAll(objOutputDir, os.ModePerm); err != nil {
		return err
	}

	idx := -1
	captureWebhook := false

	switch gvk.Kind {
	case MutatingWebhookKind:
		for idx = range u.Items {
			name := u.Items[idx].GetName()
			if strings.HasPrefix(name, MutatingWebhookPrefix) || name == MutatingWebhookName {
				captureWebhook = true
				break
			}
		}
	case ValidatingWebhookKind:
		for idx = range u.Items {
			name := u.Items[idx].GetName()
			if strings.HasPrefix(name, ValidatingWebhookPrefix) || name == ValidatingWebhookName {
				captureWebhook = true
				break
			}
		}
	}

	if captureWebhook {
		clusterData, err := yaml.Marshal(u.Items[idx])
		if err != nil {
			return err
		}

		fileName := filepath.Join(objOutputDir,
			u.Items[idx].GetName()+FileSuffix)

		if err := populateScraperDir(clusterData, fileName); err != nil {
			return err
		}
	}

	return nil
}
