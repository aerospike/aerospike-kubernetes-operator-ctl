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
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/duration"
	runtimeresource "k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/configuration"
	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/internal"
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
)

var (
	currentTime = time.Now().Format("20060102_150405")
	TarName     = RootOutputDir + "_" + currentTime + ".tar.gzip"
)

func RunCollectInfo(ctx context.Context, params *configuration.Parameters, path string) error {
	rootOutputPath := filepath.Join(path, RootOutputDir)
	if err := os.Mkdir(rootOutputPath, os.ModePerm); err != nil {
		return err
	}

	params.Logger = AttachFileLogger(params.Logger, filepath.Join(rootOutputPath, LogFileName))

	if err := CollectInfo(ctx, params, path); err != nil {
		params.Logger.Error("Not able to collect object info", zap.String("err", err.Error()))
	}

	return nil
}

func AttachFileLogger(logger *zap.Logger, path string) *zap.Logger {
	cfg := zap.NewProductionEncoderConfig()
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewJSONEncoder(cfg)
	logFile, _ := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gocritic // file permission
	defaultLogLevel := zapcore.InfoLevel
	core := zapcore.NewTee(
		zapcore.NewCore(fileEncoder, zapcore.AddSync(logFile), defaultLogLevel),
	)

	updateCore := zap.WrapCore(func(c zapcore.Core) zapcore.Core {
		return zapcore.NewTee(c, core)
	})

	return logger.WithOptions(updateCore)
}

func CollectInfo(ctx context.Context, params *configuration.Parameters, path string) error {
	rootOutputPath := filepath.Join(path, RootOutputDir)

	params.Logger.Info("Capturing namespace scoped objects info")

	for ns := range params.Namespaces {
		objOutputDir := filepath.Join(rootOutputPath, NamespaceScopedDir, ns)
		if err := os.MkdirAll(objOutputDir, os.ModePerm); err != nil {
			return err
		}

		if err := capturePodLogs(ctx, params.Logger, params.ClientSet, ns, objOutputDir); err != nil {
			return err
		}

		if err := captureEvents(ctx, params.Logger, params.ClientSet, ns, objOutputDir); err != nil {
			return err
		}

		for _, gvk := range gvkListNSScoped {
			if err := captureObject(params.Logger, params.K8sClient, gvk, ns, objOutputDir); err != nil {
				return err
			}
		}
	}

	if params.ClusterScope {
		params.Logger.Info("Capturing cluster scoped objects info")

		objOutputDir := filepath.Join(rootOutputPath, ClusterScopedDir)
		if err := os.MkdirAll(objOutputDir, os.ModePerm); err != nil {
			return err
		}

		for _, gvk := range gvkListClusterScoped {
			if err := captureObject(params.Logger, params.K8sClient, gvk, "", objOutputDir); err != nil {
				return err
			}
		}

		for _, webhooksGVK := range gvkListWebhooks {
			if err := captureWebhookConfigurations(params.Logger, params.K8sClient, webhooksGVK, objOutputDir); err != nil {
				return err
			}
		}
	}

	params.Logger.Info("Compressing and deleting all logs and created ", zap.String("tar file", TarName))

	return makeTarAndClean(path)
}

func captureObject(logger *zap.Logger, k8sClient client.Client, gvk schema.GroupVersionKind,
	ns, rootOutputPath string) error {
	listOps := &client.ListOptions{Namespace: ns}
	u := &unstructured.UnstructuredList{}

	u.SetGroupVersionKind(gvk)

	if err := k8sClient.List(context.TODO(), u, listOps); err != nil {
		logger.Error("Not able to list ", zap.String("kind", gvk.Kind), zap.Error(err))
		return err
	}

	objOutputDir := filepath.Join(rootOutputPath, KindDirNames[gvk.Kind])
	if err := os.MkdirAll(objOutputDir, os.ModePerm); err != nil {
		return err
	}

	if len(u.Items) == 0 {
		logger.Info("No resource found in namespace", zap.String("kind", gvk.Kind),
			zap.String("namespace", ns))
		return nil
	}

	for idx := range u.Items {
		if err := serializeAndWrite(u.Items[idx], objOutputDir); err != nil {
			return err
		}
	}

	logger.Info("Successfully saved ", zap.String("kind", gvk.Kind),
		zap.Int("number of objects", len(u.Items)), zap.String("namespace", ns))

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
		logger.Error("Not able to list ", zap.String("kind", internal.PodKind), zap.Error(err))
		return err
	}

	if len(pods.Items) == 0 {
		logger.Info("No resource found in namespace", zap.String("kind", "Pod"),
			zap.String("namespace", ns))
		return nil
	}

	for podIndex := range pods.Items {
		podData, err := yaml.Marshal(pods.Items[podIndex])
		if err != nil {
			return err
		}

		podLogsDir := filepath.Join(rootOutputPath, KindDirNames[internal.PodKind], pods.Items[podIndex].Name, "logs")
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

	logger.Info("Successfully saved ", zap.String("kind", internal.PodKind),
		zap.Int("number of objects", len(pods.Items)), zap.String("namespace", ns))

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
		if errors.IsBadRequest(reqErr) && previous {
			logger.Debug("Previous container's logs not found ", zap.String("container", containerName),
				zap.Error(reqErr))
			return nil
		}

		logger.Error("Could not fetch container's logs ", zap.String("container", containerName),
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
	event := fmt.Sprintf("%s\t\t\t%s\t\t%s\t\t\t%s/%s\t\t\t%v\n", getInterval(e), e.Type, e.Reason, e.InvolvedObject.Kind,
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
		logger.Info("No resource found in namespace", zap.String("kind", "Event"),
			zap.String("namespace", namespace))
		return nil
	}

	// Sort the events by `.metadata.creationTimestamp`
	sort.SliceStable(el.Items, func(i, j int) bool {
		return el.Items[i].GetCreationTimestamp().Time.Before(el.Items[j].GetCreationTimestamp().Time)
	})

	data := []byte("LAST SEEN\t\tTYPE\t\tREASON\t\t\tOBJECT\t\t\tMESSAGE\n")
	for idx := range el.Items {
		appendOneEvent(&data, &el.Items[idx])
	}

	objOutputDir := filepath.Join(rootOutputPath, KindDirNames[internal.EventKind])
	if err := os.MkdirAll(objOutputDir, os.ModePerm); err != nil {
		return err
	}

	fileName := filepath.Join(objOutputDir, KindDirNames[internal.EventKind]+".log")

	if err := populateScraperDir(data, fileName); err != nil {
		return err
	}

	logger.Info("Successfully saved ", zap.String("kind", "Events"),
		zap.Int("number of objects", len(el.Items)), zap.String("namespace", namespace))

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

func captureWebhookConfigurations(logger *zap.Logger, k8sClient client.Client, gvk schema.GroupVersionKind,
	rootOutputPath string) error {
	listOps := &client.ListOptions{}
	u := &unstructured.UnstructuredList{}

	u.SetGroupVersionKind(gvk)

	if err := k8sClient.List(context.TODO(), u, listOps); err != nil {
		logger.Error("Not able to list ", zap.String("kind", gvk.Kind), zap.Error(err))
		return err
	}

	objOutputDir := filepath.Join(rootOutputPath, KindDirNames[gvk.Kind])
	if err := os.MkdirAll(objOutputDir, os.ModePerm); err != nil {
		return err
	}

	count := 0

	switch gvk.Kind {
	case internal.MutatingWebhookKind:
		for idx := range u.Items {
			name := u.Items[idx].GetName()
			if strings.HasPrefix(name, MutatingWebhookPrefix) || name == MutatingWebhookName {
				if err := serializeAndWrite(u.Items[idx], objOutputDir); err != nil {
					return err
				}
				count++
			}
		}
	case internal.ValidatingWebhookKind:
		for idx := range u.Items {
			name := u.Items[idx].GetName()
			if strings.HasPrefix(name, ValidatingWebhookPrefix) || name == ValidatingWebhookName {
				if err := serializeAndWrite(u.Items[idx], objOutputDir); err != nil {
					return err
				}
				count++
			}
		}
	}

	logger.Info("Successfully saved ", zap.String("kind", gvk.Kind),
		zap.Int("number of objects", count))

	return nil
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
