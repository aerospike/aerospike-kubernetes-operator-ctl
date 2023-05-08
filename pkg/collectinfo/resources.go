package collectinfo

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	NodeKind             = "Node"
	PVCKind              = "PersistentVolumeClaim"
	STSKind              = "StatefulSet"
	EventKind            = "Event"
	SCKind               = "StorageClass"
	AerospikeClusterKind = "AerospikeCluster"
	PodKind              = "Pod"
)

var (
	gvkListNSScoped = []schema.GroupVersionKind{
		corev1.SchemeGroupVersion.WithKind(PVCKind),
		appsv1.SchemeGroupVersion.WithKind(STSKind),
		corev1.SchemeGroupVersion.WithKind(EventKind),
		{
			Group:   "asdb.aerospike.com",
			Version: "v1beta1",
			Kind:    AerospikeClusterKind,
		},
	}
	gvkListClusterScoped = []schema.GroupVersionKind{
		corev1.SchemeGroupVersion.WithKind(NodeKind),
		v1.SchemeGroupVersion.WithKind(SCKind),
	}
)
