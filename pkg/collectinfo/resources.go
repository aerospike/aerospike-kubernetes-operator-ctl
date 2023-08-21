package collectinfo

import (
	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	NodeKind              = "Node"
	PVCKind               = "PersistentVolumeClaim"
	PVKind                = "PersistentVolume"
	STSKind               = "StatefulSet"
	DeployKind            = "Deployment"
	SCKind                = "StorageClass"
	AerospikeClusterKind  = "AerospikeCluster"
	PodKind               = "Pod"
	EventKind             = "Event"
	MutatingWebhookKind   = "MutatingWebhookConfiguration"
	ValidatingWebhookKind = "ValidatingWebhookConfiguration"
	ServiceKind           = "Service"
)

var (
	KindDirNames = map[string]string{
		NodeKind:              "nodes",
		PVCKind:               "persistentvolumeclaims",
		PVKind:                "persistentvolumes",
		STSKind:               "statefulsets",
		DeployKind:            "deployments",
		SCKind:                "storageclasses",
		AerospikeClusterKind:  "aerospikeclusters",
		PodKind:               "pods",
		EventKind:             "events",
		MutatingWebhookKind:   "mutatingwebhookconfigurations",
		ValidatingWebhookKind: "validatingwebhookconfigurations",
		ServiceKind:           "services",
	}
	gvkListNSScoped = []schema.GroupVersionKind{
		{
			Group:   "asdb.aerospike.com",
			Version: "v1",
			Kind:    AerospikeClusterKind,
		},
		{
			Group:   "asdb.aerospike.com",
			Version: "v1beta1",
			Kind:    AerospikeClusterKind,
		},
		appsv1.SchemeGroupVersion.WithKind(STSKind),
		appsv1.SchemeGroupVersion.WithKind(DeployKind),
		corev1.SchemeGroupVersion.WithKind(PodKind),
		corev1.SchemeGroupVersion.WithKind(PVCKind),
		corev1.SchemeGroupVersion.WithKind(ServiceKind),
	}
	gvkListClusterScoped = []schema.GroupVersionKind{
		corev1.SchemeGroupVersion.WithKind(NodeKind),
		v1.SchemeGroupVersion.WithKind(SCKind),
		corev1.SchemeGroupVersion.WithKind(PVKind),
		admissionv1.SchemeGroupVersion.WithKind(MutatingWebhookKind),
		admissionv1.SchemeGroupVersion.WithKind(ValidatingWebhookKind),
	}
)
