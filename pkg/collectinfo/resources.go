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
	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	v1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/internal"
)

var (
	KindDirNames = map[string]string{
		internal.NodeKind:                   "nodes",
		internal.PVCKind:                    "persistentvolumeclaims",
		internal.PVKind:                     "persistentvolumes",
		internal.STSKind:                    "statefulsets",
		internal.DeployKind:                 "deployments",
		internal.SCKind:                     "storageclasses",
		internal.AerospikeClusterKind:       "aerospikeclusters",
		internal.PodKind:                    "pods",
		internal.EventKind:                  "events",
		internal.MutatingWebhookKind:        "mutatingwebhookconfigurations",
		internal.ValidatingWebhookKind:      "validatingwebhookconfigurations",
		internal.ServiceKind:                "services",
		internal.AerospikeBackupServiceKind: "aerospikebackupservices",
		internal.AerospikeBackupKind:        "aerospikebackups",
		internal.AerospikeRestoreKind:       "aerospikerestores",
		internal.PodDisruptionBudgetKind:    "poddisruptionbudgets",
		internal.ConfigMapKind:              "configmaps",
		internal.CRDKind:                    "customresourcedefinitions",
	}
	gvkListNSScoped = []schema.GroupVersionKind{
		{
			Group:   internal.Group,
			Version: "v1",
			Kind:    internal.AerospikeClusterKind,
		},
		{
			Group:   internal.Group,
			Version: internal.BetaVersion,
			Kind:    internal.AerospikeBackupServiceKind,
		},
		{
			Group:   internal.Group,
			Version: internal.BetaVersion,
			Kind:    internal.AerospikeBackupKind,
		},
		{
			Group:   internal.Group,
			Version: internal.BetaVersion,
			Kind:    internal.AerospikeRestoreKind,
		},
		appsv1.SchemeGroupVersion.WithKind(internal.STSKind),
		appsv1.SchemeGroupVersion.WithKind(internal.DeployKind),
		corev1.SchemeGroupVersion.WithKind(internal.PodKind),
		corev1.SchemeGroupVersion.WithKind(internal.PVCKind),
		corev1.SchemeGroupVersion.WithKind(internal.ServiceKind),
		policyv1.SchemeGroupVersion.WithKind(internal.PodDisruptionBudgetKind),
		corev1.SchemeGroupVersion.WithKind(internal.ConfigMapKind),
	}
	gvkListClusterScoped = []schema.GroupVersionKind{
		corev1.SchemeGroupVersion.WithKind(internal.NodeKind),
		v1.SchemeGroupVersion.WithKind(internal.SCKind),
		corev1.SchemeGroupVersion.WithKind(internal.PVKind),
		admissionv1.SchemeGroupVersion.WithKind(internal.MutatingWebhookKind),
		admissionv1.SchemeGroupVersion.WithKind(internal.ValidatingWebhookKind),
		apiextensionsv1.SchemeGroupVersion.WithKind(internal.CRDKind),
	}
)
