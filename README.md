# Aerospike-kubernetes-operator-ctl

This is a command line tool for Aerospike Kubernetes Operator. It provides multiple sub-commands to perform different functions related to Aerospike Kubernetes Operator and Aerospike Kubernetes Cluster.

Available sub-commands:
1. [`collectinfo`](#aerospike-kubernetes-operator-log-collector)
2. [`auth`](#grant-aerospike-kubernetes-cluster-rbac)
### Building and quick start

#### Building akoctl binary for local testing
```sh
make build
```

#### Install via Krew plugin manager
[Krew](https://krew.sigs.k8s.io) is the plugin manager for kubectl command-line tool. Here `akoctl` has been added as a custom plugin to krew.

##### Install Krew
To install krew on any platform, follow [this](https://krew.sigs.k8s.io/docs/user-guide/setup/install/).
##### Install akoctl
```sh
kubectl krew index add akoctl https://github.com/aerospike/aerospike-kubernetes-operator-ctl.git

% kubectl krew index list
INDEX    URL
akoctl   https://github.com/aerospike/aerospike-kubernetes-operator-ctl.git
default  https://github.com/kubernetes-sigs/krew-index.git

% kubectl krew install akoctl/akoctl
Updated the local copy of plugin index "akoctl".
Updated the local copy of plugin index.
Installing plugin: akoctl
Installed plugin: akoctl
\
 | Use this plugin:
 | 	kubectl akoctl
 | Documentation:
 | 	https://github.com/aerospike/aerospike-kubernetes-operator-ctl
/
```

## Aerospike Kubernetes Operator Log Collector

### Overview

`collectinfo` command collects all the required info from kubernetes cluster, which are available at the time of command being executed.

Flag associated with this command:
* **path** - (type string) Absolute path to save output tar file.

### Requirements
* Current user should have the list and get permission for all the objects collected by the command.
* If **cluster-scope** flag is set, along with permissions mentioned above, user should have list and get permission for cluster-scoped resources like(nodes and storageclasses).
* * **Kubectl** binary should be available in **PATH** environment variable.

#### Collect cluster info using local binary
```sh
 ./bin/akoctl collectinfo -n aerospike,olm --path ~/abc/
```

#### Collect cluster info using krew
```sh
 kubectl akoctl collectinfo -n aerospike,olm --path ~/abc/
```

### Data Collected

This command collects the following data from the specified namespaces:

* Pods, StatefulSets, Deployments, PersistentVolumeClaims, PersistentVolumes, Services, AerospikeCluster objects .
* Container logs.
* Event logs.

Additionally, the following cluster-wide data points are collected:
* Storage class objects.
* Configurations of all nodes in the kubernetes cluster.
* Configurations of aerospike mutating and validating webhooks.

### Result Format

* This will create a tar file with timestamp called "scraperlogs-<time-stamp>" which contains all the collected info from the cluster.
* Directory structure will look like this.
```shell
akoctl_collectinfo
├── akoctl.log
├── k8s_cluster
│   ├── nodes
│   │   ├── <node1 name>.yaml
│   │   └── <node2 name>.yaml
│   └── storageclasses
│       ├── <storageclass name>.yaml
│   └── mutatingwebhookconfigurations
│       ├── <mutatingwebhook name>.yaml
│   └── validatingwebhookconfigurations
│       ├── <validatingwebhook name>.yaml
│   └── persistentvolumes
│       ├── <persistentvolume name>.yaml
│   └── summary
│       ├── summary.txt
└── k8s_namespaces
    └── aerospike
        ├── aerospikeclusters
        │   ├── <aerospikecluster name>.yaml
        ├── persistentvolumeclaims
        │   ├── <pvc name>.yaml
        ├── pods
        │   ├── <pod name>
        │   │   ├── <pod name>.yaml
        │   │   └── logs
        │   │       ├── previous
        │   │       │   └── <container name>.log
        │   │       └── <container name>.log
        └── statefulsets
        │   ├── <sts name>.yaml
        └── deployments
        │   ├── <deployment name>.yaml
        └── services
        │   ├── <service name>.yaml
        └── summary
        │   ├── summary.txt
        │   ├── events.txt
        └──────────────────────────

```

## Grant Aerospike Kubernetes Cluster RBAC

### Overview

`auth` command creates/deletes RBAC resources for Aerospike cluster for the given namespaces.
It creates/deletes ServiceAccount, RoleBinding or ClusterRoleBinding as per given scope of operation.

There are 2 sub-commands associated with this command:
* `create` - It creates/updates RBAC resources for the given namespaces.
* `delete` - It deletes RBAC resources for the given namespaces.

If `cluster-scope` is set (Default true), auth command grants cluster level RBAC whereas in case of `cluster-scope` false, it grants namespace level RBAC.

### Permission required
* Current user should have the CREATE, GET, UPDATE and DELETE permission for ServiceAccount and RoleBinding.
* If **cluster-scope** flag is set, user should have the CREATE, GET, UPDATE and DELETE permission for ServiceAccount and ClusterRoleBinding.

#### Create/Delete RBAC resources using local binary
```sh
 ./bin/akoctl auth create -n aerospike,olm  # creates RBAC resources for aerospike and olm namespaces
 ./bin/akoctl auth delete -n aerospike,olm  # deletes RBAC resources for aerospike and olm namespaces
```

#### Create/Delete RBAC resources using krew
```sh
kubectl akoctl auth create -n aerospike,olm # creates RBAC resources for aerospike and olm namespaces
kubectl akoctl auth delete -n aerospike,olm # deletes RBAC resources for aerospike and olm namespaces

```

## Global Flags:
There are certain global flags associated with akoctl:
* **all-namespaces** - (shorthand -A, type bool) Specify all namespaces present in cluster.
* **namespaces** - (shorthand -n, type string) Comma separated list of namespaces to perform operation in.
* **kubeconfig** - (type string) Absolute path to the kubeconfig file.
* **cluster-scope** - (type bool) Permission to work in cluster scoped mode (operate on cluster scoped resources like ClusterRoleBinding). Default true.