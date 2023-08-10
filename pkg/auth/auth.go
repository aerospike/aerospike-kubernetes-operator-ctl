package auth

import (
	"context"
	"fmt"
	"reflect"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/configuration"
	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/internal"
)

const (
	ServiceAccountName     = "aerospike-operator-controller-manager"
	ClusterRoleName        = "aerospike-cluster"
	ClusterRoleBindingName = "aerospike-cluster"
	RoleBindingName        = "aerospike-cluster"
)

func Create(ctx context.Context, params *configuration.Parameters) error {
	subjects := make([]interface{}, 0, len(params.Namespaces))

	for ns := range params.Namespaces {
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ServiceAccountName,
				Namespace: ns,
			},
		}

		// Create SA and check namespace existence
		if err := params.K8sClient.Create(ctx, sa); err != nil && errors.IsNotFound(err) {
			params.Logger.Error(fmt.Sprintf("namespace: %s not found, skipping RBAC resources", ns))
			continue
		}

		params.Logger.Info("Created resource", zap.String("kind", internal.ServiceAccountKind),
			zap.String("name", ServiceAccountName), zap.String("namespace", ns))

		sub := map[string]interface{}{
			"kind":      internal.ServiceAccountKind,
			"name":      ServiceAccountName,
			"namespace": ns,
		}

		// If RBAC scope is namespace, then create RoleBinding and continue
		if !params.ClusterScope {
			if err := createOrUpdateBinding(
				ctx, params,
				v1.SchemeGroupVersion.WithKind(internal.RoleBindingKind),
				types.NamespacedName{Name: RoleBindingName, Namespace: ns},
				[]interface{}{sub}); err != nil {
				return err
			}

			continue
		}

		subjects = append(subjects, sub)
	}

	// Return from here if namespace scope or no change in subjects
	if !params.ClusterScope || len(subjects) == 0 {
		return nil
	}

	return createOrUpdateBinding(
		ctx, params,
		v1.SchemeGroupVersion.WithKind(internal.ClusterRoleBindingKind),
		types.NamespacedName{Name: ClusterRoleBindingName},
		subjects)
}

func createOrUpdateBinding(
	ctx context.Context, params *configuration.Parameters, gvk schema.GroupVersionKind,
	nsNm types.NamespacedName, subjects interface{},
) error {
	unstruct := &unstructured.Unstructured{}
	unstruct.SetGroupVersionKind(gvk)
	unstruct.SetName(nsNm.Name)
	unstruct.SetNamespace(nsNm.Namespace)

	roleRef := map[string]interface{}{
		"apiGroup": v1.GroupName,
		"kind":     internal.ClusterRoleKind,
		"name":     ClusterRoleName,
	}

	unstruct.Object["subjects"] = subjects
	unstruct.Object["roleRef"] = roleRef

	if err := params.K8sClient.Create(ctx, unstruct); err != nil {
		if errors.IsAlreadyExists(err) {
			params.Logger.Info("Resource already exists, trying to update", zap.String("kind", gvk.Kind),
				zap.String("name", nsNm.Name), zap.String("namespace", nsNm.Namespace))

			currentResource := &unstructured.Unstructured{}
			currentResource.SetGroupVersionKind(gvk)

			if gErr := params.K8sClient.Get(ctx, nsNm, currentResource); gErr != nil {
				return gErr
			}

			if !reflect.DeepEqual(currentResource.Object["roleRef"], unstruct.Object["roleRef"]) {
				return fmt.Errorf("%s: %s already exists with different roleRe,"+
					"can't update roleRef to %s", gvk.Kind, nsNm.Name, ClusterRoleName)
			}

			if !reflect.DeepEqual(currentResource.Object["subjects"], unstruct.Object["subjects"]) {
				currentResource.Object["subjects"] = mergeSubjects(currentResource.Object["subjects"].([]interface{}),
					unstruct.Object["subjects"].([]interface{}))

				if uErr := params.K8sClient.Update(ctx, currentResource); uErr != nil {
					return uErr
				}

				params.Logger.Info("Updated resource", zap.String("kind", gvk.Kind),
					zap.String("name", nsNm.Name), zap.String("namespace", nsNm.Namespace))
			}

			return nil
		}
	}

	params.Logger.Info("Created resource", zap.String("kind", gvk.Kind),
		zap.String("name", nsNm.Name), zap.String("namespace", nsNm.Namespace))

	return nil
}

func mergeSubjects(baseSub, patchSub []interface{}) []interface{} {
	var newSubs []interface{}

	for patchIdx := range patchSub {
		var matched bool

		for baseIdx := range baseSub {
			if reflect.DeepEqual(baseSub[baseIdx].(map[string]interface{}), patchSub[patchIdx].(map[string]interface{})) {
				matched = true
				break
			}
		}

		if !matched {
			newSubs = append(newSubs, patchSub[patchIdx])
		}
	}

	if len(newSubs) > 0 {
		baseSub = append(baseSub, newSubs...)
	}

	return baseSub
}

func Delete(ctx context.Context, params *configuration.Parameters) error {
	for ns := range params.Namespaces {
		// Delete serviceAccount
		deleteResource(
			ctx, params,
			corev1.SchemeGroupVersion.WithKind(internal.ServiceAccountKind),
			types.NamespacedName{Name: ServiceAccountName, Namespace: ns})

		// If RBAC scope is namespace, then delete RoleBinding
		if !params.ClusterScope {
			deleteResource(
				ctx, params,
				v1.SchemeGroupVersion.WithKind(internal.RoleBindingKind),
				types.NamespacedName{Name: RoleBindingName, Namespace: ns})
		}
	}

	// Return from here if namespace scope
	if !params.ClusterScope {
		return nil
	}

	crb := &v1.ClusterRoleBinding{}
	if err := params.K8sClient.Get(ctx, types.NamespacedName{
		Name: ClusterRoleBindingName,
	}, crb); err != nil {
		return err
	}

	// Removed subject entries for the given namespaces
	filtered := make([]v1.Subject, 0, len(crb.Subjects))

	for _, sub := range crb.Subjects {
		if sub.Kind == internal.ServiceAccountKind &&
			sub.Name == ServiceAccountName && params.Namespaces.Has(sub.Namespace) {
			continue
		}

		filtered = append(filtered, sub)
	}

	if len(filtered) == 0 {
		deleteResource(
			ctx, params,
			v1.SchemeGroupVersion.WithKind(internal.ClusterRoleKind),
			types.NamespacedName{Name: ClusterRoleBindingName})

		return nil
	}

	if len(filtered) == len(crb.Subjects) {
		params.Logger.Info("Update not required, skipping", zap.String("kind", internal.ClusterRoleBindingKind),
			zap.String("name", ClusterRoleBindingName))

		return nil
	}

	crb.Subjects = filtered

	params.Logger.Info(fmt.Sprintf("Updating %s subjects", internal.ClusterRoleKind),
		zap.String("name", ClusterRoleName))

	return params.K8sClient.Update(ctx, crb)
}

func deleteResource(
	ctx context.Context, params *configuration.Parameters, gvk schema.GroupVersionKind,
	nsNm types.NamespacedName) {
	unstruct := &unstructured.Unstructured{}

	unstruct.SetGroupVersionKind(gvk)
	unstruct.SetName(nsNm.Name)
	unstruct.SetNamespace(nsNm.Namespace)

	if err := params.K8sClient.Delete(ctx, unstruct); err != nil {
		if errors.IsNotFound(err) {
			params.Logger.Warn("Resource not found for deletion, skipping", zap.String("kind", gvk.Kind),
				zap.String("name", nsNm.Name), zap.String("namespace", nsNm.Namespace))
		} else {
			// TODO: Should we error out or continue if we are not able to delete?
			params.Logger.Error("failed to delete resource", zap.String("kind", gvk.Kind),
				zap.String("name", nsNm.Name), zap.String("namespace", nsNm.Namespace), zap.Error(err))
		}

		return
	}

	params.Logger.Info("Deleted resource", zap.String("kind", gvk.Kind),
		zap.String("name", nsNm.Name), zap.String("namespace", nsNm.Namespace))
}
