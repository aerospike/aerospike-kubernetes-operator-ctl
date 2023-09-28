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

package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	kubeconfig    string
	namespaces    []string
	allNamespaces bool
	clusterScope  bool
)

var rootCmd = &cobra.Command{
	Use:   "akoctl",
	Short: "A command line tool for Aerospike Kubernetes Operator",
	Long: `A CLI which is used to perform different functions related to Aerospike Kubernetes Operator and 
Aerospike Kubernetes Operator cluster.
For example:
akoctl collectinfo --namespaces aerospike,olm`,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringSliceVarP(&namespaces, "namespaces", "n", namespaces,
		"Comma separated list of namespaces to perform operation in")
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "",
		"Absolute path to the kubeconfig file")
	rootCmd.PersistentFlags().BoolVarP(&allNamespaces, "all-namespaces", "A", false,
		"Specify all namespaces present in cluster")
	rootCmd.PersistentFlags().BoolVar(&clusterScope, "cluster-scope", true,
		"Permission to work in cluster scoped mode (operate on cluster scoped resources like ClusterRoleBinding)")
}
