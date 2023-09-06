# Aerospike-kubernetes-operator-ctl

This is a command line tool for Aerospike kubernetes operator.

## Aerospike Kubernetes Operator Log Collector

### Overview

collectinfo command collects all the required info from kubernetes cluster, which are available at the time of command being executed.

There are certain flags associated with command:
* **all-namespaces** - (shorthand -A, type bool) Collect info from all namespaces present in cluster.
* **namespaces** - (shorthand -n, type string) Comma separated list of namespaces from which data needs to be collected.
* **kubeconfig** - (type string) Absolute path to the kubeconfig file.
* **path** - (type string) Absolute path to save output tar file.
* **cluster-scope** - (type bool) Permission to collect cluster scoped objects info. Default true.

### Requirements
* Current user should have the list and get permission for all the objects collected by the command.
* If **cluster-scope** flag is set, along with permission mentioned above, user should have list and get permission for cluster-scoped resources like(nodes and storageclasses).
* **Kubectl** binary should be available in **PATH** environment variable.

### Building and quick start

#### Building akoctl binary
```sh
make build
```

#### Collect cluster info
```sh
 ./bin/akoctl collectinfo -n aerospike,olm --path ~/abc/
```

#### Install via krew plugin manager
[Krew](https://krew.sigs.k8s.io) is the plugin manager for kubectl command-line tool. Here `akoctl` has been added as a custom plugin to krew.

##### Install krew 
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