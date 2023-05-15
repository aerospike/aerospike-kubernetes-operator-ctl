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

### Permission required
* Current user should have the list and get permission for all the objects collected by the command.
* If **cluster-scope** flag is set, along with permissions mentioned above, user should have list and get permission for cluster-scoped resources like(nodes and storageclasses).

### Building and quick start

#### Building akoctl binary
```sh
make build
```

#### Collect cluster info
```sh
 ./bin/akoctl collectinfo -n aerospike,olm --path ~/abc/ --cluster-scope=false
```


### Data Collected

This command collects the following data from the specified namespaces:

* Pods, StatefulSets, PersistentVolumeClaims, AerospikeCluster objects .
* Container logs.
* Event logs.

Additionally, the following cluster-wide data points are collected:
* Storage class objects.
* Configurations of all nodes in the kubernetes cluster.

### Result Format

* This will create a tar file with timestamp called "scraperlogs-<time-stamp>" which contains all the collected info from the cluster.
* Directory structure will look like this.
```shell
akoctl_collectinfo
├── akoctl.log
├── k8s-cluster
│   ├── Node
│   │   ├── <node1 name>.yaml
│   │   └── <node2 name>.yaml
│   └── StorageClass
│       ├── <storageclass name>.yaml
└── k8s-namespaces
    └── aerospike
        ├── AerospikeCluster
        ├── Event
        ├── PersistentVolumeClaim
        │   ├── <pvc name>.yaml
        ├── Pod
        │   ├── <pod name>
        │   │   ├── <pod name>.yaml
        │   │   └── logs
        │   │       ├── previous
        │   │       │   └── <container name>.log
        │   │       └── <container name>.log
        └── StatefulSet
        │   ├── <sts name>.yaml

```