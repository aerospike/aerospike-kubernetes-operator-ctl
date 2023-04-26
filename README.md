# Aerospike-kubernetes-operator-ctl

This is a command line tool for Aerospike kubernetes operator.

## Aerospike Kubernetes Operator Log Collector

### Overview

collectinfo command collects all the required info from kubernetes cluster, which are available at the time of command being executed.


### What info does it collect?

This command collects the following data from the given namespaces:

* Pods, STS, PVC, AerospikeCluster objects .
* Container logs.
* Event logs.

Some cluster-wide data points:
* Storage class objects.
* Configurations of all nodes in the kubernetes cluster.

### How does result looks like?

* This will create a tar file with timestamp called "scraperlogs-<time-stamp>" which contains all the collected info from the cluster.
