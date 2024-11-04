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

package internal

const (
	// Namespace scope resources
	PodKind                    = "Pod"
	STSKind                    = "StatefulSet"
	RSKind                     = "ReplicaSet"
	DeployKind                 = "Deployment"
	ServiceAccountKind         = "ServiceAccount"
	ServiceKind                = "Service"
	AerospikeClusterKind       = "AerospikeCluster"
	PVCKind                    = "PersistentVolumeClaim"
	EventKind                  = "Event"
	RoleBindingKind            = "RoleBinding"
	AerospikeBackupKind        = "AerospikeBackup"
	AerospikeRestoreKind       = "AerospikeRestore"
	AerospikeBackupServiceKind = "AerospikeBackupService"
	PodDisruptionBudgetKind    = "PodDisruptionBudget"
	ConfigMapKind              = "ConfigMap"

	// Cluster scope resources
	NodeKind               = "Node"
	PVKind                 = "PersistentVolume"
	SCKind                 = "StorageClass"
	MutatingWebhookKind    = "MutatingWebhookConfiguration"
	ValidatingWebhookKind  = "ValidatingWebhookConfiguration"
	ClusterRoleKind        = "ClusterRole"
	ClusterRoleBindingKind = "ClusterRoleBinding"
)
