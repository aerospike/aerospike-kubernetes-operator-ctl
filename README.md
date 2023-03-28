# Aerospike-kubernetes-operator-ctl

This is a command line tool for Aerospike kubernetes operator.

## Aerospike Kubernetes Operator Log Collector

### Overview

collectinfo command collects all the required logs of kubernetes cluster, which are available at the time of command being executed.


### What logs does it collect?

This command collects the following data from the given namespaces (all namespaces if none provided):

* Pods, STS, PVC, AerospikeCluster objects .
* Container logs.
* Event logs.

Some cluster-wide data points:
* Storage class objects.
* Configurations of all nodes in the kubernetes cluster.

### How does result looks like?

* This will create a tar file containes a directory with name "scraperlogs".
* Inside that all cluster wide information will be available.
