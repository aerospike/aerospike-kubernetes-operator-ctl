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
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
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

func NewParams(ctx context.Context, kubeconfigPath string, namespaces []string, allNamespaces,
	clusterScope bool,
) (*Parameters, error) {
	logger := InitializeConsoleLogger()
	logger.Info("Initialized logger")

	k8sClient, clientSet, err := createKubeClients(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	logger.Info("Created Kubernetes clients")

	params := &Parameters{
		K8sClient:     k8sClient,
		ClientSet:     clientSet,
		Logger:        logger,
		ClusterScope:  clusterScope,
		AllNamespaces: allNamespaces,
	}

	if err := params.ValidateNamespaces(ctx, namespaces); err != nil {
		return nil, err
	}

	return params, nil
}

func createKubeClients(kubeconfigPath string) (k8sClient client.Client, clientSet *kubernetes.Clientset, err error) {
	var cfg *rest.Config

	if kubeconfigPath != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, nil, err
		}
	} else {
		cfg = runtimeConfig.GetConfigOrDie()
	}

	scheme := runtime.NewScheme()

	err = clientgoscheme.AddToScheme(scheme)
	if err != nil {
		return nil, nil, err
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, nil, err
	}

	clientSet, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, nil, err
	}

	return k8sClient, clientSet, nil
}

func (p *Parameters) ValidateNamespaces(ctx context.Context, namespaces []string) error {
	if len(namespaces) == 0 && !p.AllNamespaces {
		return fmt.Errorf("either `namespaces` or `all-namespaces` argument must be provided")
	}

	userNsSet := sets.Set[string]{}
	userNsSet.Insert(namespaces...)

	allNsSet := sets.Set[string]{}
	namespaceObjs := &corev1.NamespaceList{}

	if err := p.K8sClient.List(ctx, namespaceObjs); err != nil {
		return err
	}

	for idx := range namespaceObjs.Items {
		allNsSet.Insert(namespaceObjs.Items[idx].Name)
	}

	if p.AllNamespaces {
		p.Logger.Info("Capturing for all namespaces")

		userNsSet = allNsSet
	} else {
		nonExistentNs := userNsSet.Difference(allNsSet)

		// error out if all the user given namespaces are not present in cluster
		if nonExistentNs.Len() > 0 {
			if nonExistentNs.Len() == userNsSet.Len() {
				return fmt.Errorf("all given namespaces are not present in cluster")
			}

			p.Logger.Warn(
				fmt.Sprintf("namespaces %+v not present in cluster, skipping those namespaces",
					nonExistentNs.UnsortedList()))

			userNsSet = userNsSet.Difference(nonExistentNs)
		}
	}

	p.Namespaces = userNsSet

	return nil
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
