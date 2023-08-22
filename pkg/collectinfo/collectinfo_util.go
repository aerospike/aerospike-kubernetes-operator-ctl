package collectinfo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	MutatingWebhookPrefix   = "maerospikecluster.kb.io"
	ValidatingWebhookPrefix = "vaerospikecluster.kb.io"
	MutatingWebhookName     = "aerospike-operator-mutating-webhook-configuration"
	ValidatingWebhookName   = "aerospike-operator-validating-webhook-configuration"
	SummaryDir              = "summary"
	SummaryFile             = "summary.txt"
	EventsFile              = "events.txt"
	kubectlCMD              = "kubectl"
)

var (
	currentTime = time.Now().Format("20060102_150405")
	TarName     = RootOutputDir + "_" + currentTime + ".tar.gzip"
	pvcNameSet  = sets.String{}
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

		for _, gvk := range gvkListNSScoped {
			if gvk.Kind == PodKind {
				if err := capturePodLogs(ctx, logger, clientSet, ns, objOutputDir); err != nil {
					return err
				}
			} else {
				if err := captureObject(logger, k8sClient, gvk, ns, objOutputDir); err != nil {
					return err
				}
			}
		}

		if err := captureSummary(logger, ns, objOutputDir); err != nil {
			return err
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

		if err := captureSummary(logger, "", objOutputDir); err != nil {
			return err
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
		if gvk.Kind == AerospikeClusterKind && errors.Is(err, &meta.NoKindMatchError{}) {
			gvk.Version = "v1beta1"
			u.SetGroupVersionKind(gvk)

			if listErr := k8sClient.List(context.TODO(), u, listOps); listErr != nil {
				logger.Error("Not able to list ",
					zap.String("object", gvk.Kind), zap.String("version", gvk.Version), zap.Error(listErr))
				return err
			}
		} else {
			logger.Error("Not able to list ", zap.String("object", gvk.Kind), zap.Error(err))
			return err
		}
	}

	objOutputDir := filepath.Join(rootOutputPath, KindDirNames[gvk.Kind])
	if err := os.MkdirAll(objOutputDir, os.ModePerm); err != nil {
		return err
	}

	count := 0

	for idx := range u.Items {
		switch gvk.Kind {
		case PVCKind:
			obj := u.Items[idx].Object
			volumeName := obj["spec"].(map[string]interface{})["volumeName"].(string)
			pvcNameSet.Insert(volumeName)
		case PVKind:
			if !pvcNameSet.Has(u.Items[idx].GetName()) {
				continue
			}
		case ValidatingWebhookKind:
			name := u.Items[idx].GetName()
			if !(strings.HasPrefix(name, ValidatingWebhookPrefix) || name == ValidatingWebhookName) {
				continue
			}
		case MutatingWebhookKind:
			name := u.Items[idx].GetName()
			if !(strings.HasPrefix(name, MutatingWebhookPrefix) || name == MutatingWebhookName) {
				continue
			}
		}

		if err := serializeAndWrite(u.Items[idx], objOutputDir); err != nil {
			return err
		}

		count++
	}

	logger.Info("Successfully saved ", zap.String("object", gvk.Kind),
		zap.Int("no of objects", count), zap.String("namespace", ns))

	return nil
}

func captureSummary(logger *zap.Logger, ns, rootOutputPath string) error {
	_, err := exec.LookPath(kubectlCMD)
	if err != nil {
		logger.Error("not able to collect cluster summary", zap.Error(err))
		return nil
	}

	cmdMap := make(map[string]*exec.Cmd)

	if ns != "" {
		for _, gvk := range gvkListNSScoped {
			cmd := exec.Command(kubectlCMD, "get", gvk.Kind, "-n", ns) //nolint:gosec // kind is constant
			cmdMap[gvk.Kind] = cmd
		}

		cmd := exec.Command(kubectlCMD, "get", EventKind, "-n", ns, "--sort-by=.metadata.creationTimestamp")
		cmdMap[EventKind] = cmd
	} else {
		for _, gvk := range gvkListClusterScoped {
			cmd := exec.Command(kubectlCMD, "get", gvk.Kind) //nolint:gosec // kind is constant
			cmdMap[gvk.Kind] = cmd
		}
	}

	var (
		finalSummary []byte
		events       []byte
	)

	for kind, cmd := range cmdMap {
		divider := fmt.Sprintf("\n%s\n%s%s\n%s\n",
			strings.Repeat("-", 100), strings.Repeat(" ", 50-len(kind)/2), kind, strings.Repeat("-", 100))

		out, err := cmd.Output()
		if err != nil {
			logger.Error("could not run command: ", zap.Error(err))
			continue
		}

		switch kind {
		case PVKind:
			out = filterPersistentVolumes(out)
		case MutatingWebhookKind:
			out = filterWebhooks(out)
		case ValidatingWebhookKind:
			out = filterWebhooks(out)
		case EventKind:
			events = out
			continue
		}

		if len(out) > 0 {
			finalSummary = append(finalSummary, []byte(divider)...)
			finalSummary = append(finalSummary, out...)
		}
	}

	objOutputDir := filepath.Join(rootOutputPath, SummaryDir)
	if err := os.MkdirAll(objOutputDir, os.ModePerm); err != nil {
		return err
	}

	if err := populateScraperDir(finalSummary, filepath.Join(objOutputDir, SummaryFile)); err != nil {
		return err
	}

	if len(events) > 0 {
		if err := populateScraperDir(events, filepath.Join(objOutputDir, EventsFile)); err != nil {
			return err
		}
	}

	logger.Info("Successfully saved summary", zap.String("namespace", ns))

	return nil
}

func filterPersistentVolumes(out []byte) (finalOut []byte) {
	outList := bytes.Split(out, []byte("\n"))

	pvcNameSet.Insert("NAME")

	for _, o := range outList {
		for pvc := range pvcNameSet {
			if bytes.Contains(o, []byte(pvc)) {
				finalOut = append(finalOut, o...)
				finalOut = append(finalOut, []byte("\n")...)
			}
		}
	}

	return finalOut
}

func filterWebhooks(out []byte) (finalOut []byte) {
	outList := bytes.Split(out, []byte("\n"))
	webhookNameSet := sets.String{}

	webhookNameSet.Insert(
		MutatingWebhookName, MutatingWebhookPrefix, ValidatingWebhookName, ValidatingWebhookPrefix, "NAME")

	for _, o := range outList {
		for webhook := range webhookNameSet {
			if bytes.Contains(o, []byte(webhook)) {
				finalOut = append(finalOut, o...)
				finalOut = append(finalOut, []byte("\n")...)
			}
		}
	}

	return finalOut
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

func serializeAndWrite(obj unstructured.Unstructured, objOutputDir string) error {
	clusterData, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}

	fileName := filepath.Join(objOutputDir,
		obj.GetName()+FileSuffix)

	return populateScraperDir(clusterData, fileName)
}
