package testutils

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/configuration"
)

func NewTestParams(
	ctx context.Context, k8sClient client.Client, clientSet *kubernetes.Clientset,
	namespaces []string, allNamespaces, clusterScope bool) (
	*configuration.Parameters, error) {
	logger := configuration.InitializeConsoleLogger()
	logger.Info("Initialized test logger")

	params := &configuration.Parameters{
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

func CreateNamespace(
	ctx context.Context, k8sClient client.Client, name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	err := k8sClient.Create(ctx, ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}
