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

package configuration

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	runtimeConfig "sigs.k8s.io/controller-runtime/pkg/client/config"
)

type Parameters struct {
	K8sClient     client.Client
	ClientSet     *kubernetes.Clientset
	Logger        *zap.Logger
	Namespaces    sets.Set[string]
	ClusterScope  bool
	AllNamespaces bool
}

func NewParams(ctx context.Context, namespaces []string, allNamespaces, clusterScope bool) (*Parameters, error) {
	logger := InitializeConsoleLogger()
	logger.Info("Initialized logger")

	k8sClient, clientSet, err := createKubeClients()
	if err != nil {
		return nil, err
	}

	logger.Info("Created Kubernetes clients")

	if len(namespaces) == 0 && !allNamespaces {
		return nil, fmt.Errorf("either `namespaces` or `all-namespaces` argument must be provided")
	}

	nsSet := sets.Set[string]{}
	nsSet.Insert(namespaces...)

	if allNamespaces {
		logger.Info("Capturing for all namespaces")

		namespaceObjs := &corev1.NamespaceList{}
		if err := k8sClient.List(ctx, namespaceObjs); err != nil {
			return nil, err
		}

		for idx := range namespaceObjs.Items {
			nsSet.Insert(namespaceObjs.Items[idx].Name)
		}
	}

	return &Parameters{
		K8sClient:     k8sClient,
		ClientSet:     clientSet,
		Logger:        logger,
		Namespaces:    nsSet,
		ClusterScope:  clusterScope,
		AllNamespaces: allNamespaces,
	}, nil
}

func createKubeClients() (client.Client, *kubernetes.Clientset, error) {
	scheme := runtime.NewScheme()
	cfg := runtimeConfig.GetConfigOrDie()

	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, nil, err
	}

	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, nil, err
	}

	return k8sClient, clientSet, nil
}

func InitializeConsoleLogger() *zap.Logger {
	cfg := zap.NewProductionEncoderConfig()
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder
	consoleEncoder := zapcore.NewConsoleEncoder(cfg)
	defaultLogLevel := zapcore.InfoLevel
	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), defaultLogLevel),
	)

	return zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.DPanicLevel))
}
