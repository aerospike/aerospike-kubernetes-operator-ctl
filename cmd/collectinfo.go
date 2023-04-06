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
	"github.com/spf13/cobra"

	"akoctl/pkg"
)

var (
	kubeconfig  string
	namespaces  []string
	pathToStore string
)

// collectinfoCmd represents the collectinfo command
var collectinfoCmd = &cobra.Command{
	Use:   "collectinfo",
	Short: "collectinfo command collects all the required info from kubernetes cluster",
	Long: `This command collects the following data from the given namespaces (all namespaces if none provided):

* Pods, STS, PVC, AerospikeCluster, Nodes, StorageClasses objects .
* Container logs.
* Event logs.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return pkg.CollectInfoUtil(namespaces, pathToStore)
	},
}

func init() {
	rootCmd.AddCommand(collectinfoCmd)

	collectinfoCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "",
		"Absolute path to the kubeconfig file")
	collectinfoCmd.Flags().StringSliceVar(&namespaces, "namespaces", namespaces,
		"Namespaces for which logs to be collected")
	collectinfoCmd.Flags().StringVar(&pathToStore, "pathtostore", "",
		"Absolute path where generated tar file will be saved")
}
