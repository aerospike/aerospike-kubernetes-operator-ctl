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
	v1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/internal"
)

var (
	KindDirNames = map[string]string{
		internal.NodeKind:              "nodes",
		internal.PVCKind:               "persistentvolumeclaims",
		internal.STSKind:               "statefulsets",
		internal.SCKind:                "storageclasses",
		internal.AerospikeClusterKind:  "aerospikeclusters",
		internal.PodKind:               "pods",
		internal.EventKind:             "events",
		internal.MutatingWebhookKind:   "mutatingwebhookconfigurations",
		internal.ValidatingWebhookKind: "validatingwebhookconfigurations",
	}
	gvkListNSScoped = []schema.GroupVersionKind{
		corev1.SchemeGroupVersion.WithKind(internal.PVCKind),
		appsv1.SchemeGroupVersion.WithKind(internal.STSKind),
		{
			Group:   "asdb.aerospike.com",
			Version: "v1beta1",
			Kind:    internal.AerospikeClusterKind,
		},
	}
	gvkListClusterScoped = []schema.GroupVersionKind{
		corev1.SchemeGroupVersion.WithKind(internal.NodeKind),
		v1.SchemeGroupVersion.WithKind(internal.SCKind),
	}
	gvkListWebhooks = []schema.GroupVersionKind{
		admissionv1.SchemeGroupVersion.WithKind(internal.MutatingWebhookKind),
		admissionv1.SchemeGroupVersion.WithKind(internal.ValidatingWebhookKind),
	}
)
