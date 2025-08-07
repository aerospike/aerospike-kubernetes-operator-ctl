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
	"context"

	"github.com/spf13/cobra"

	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/collectinfo"
	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/configuration"
)

var (
	path string
)

// collectinfoCmd represents the collectinfo command
var collectinfoCmd = &cobra.Command{
	Use:   "collectinfo",
	Short: "collectinfo command collects all the required info from kubernetes cluster",
	Long: `This command collects:
Following resources from the given namespaces:
* pods, statefulsets, deployments, persistentvolumeclaims, aerospikeclusters, 
aerospikebackupservices, aerospikebackups, aerospikerestores, configmaps, 
poddisruptionbudgets and services.

Following resources from the cluster:
* nodes, storageclasses, persistentvolumes, mutatingwebhookconfigurations, 
validatingwebhookconfigurations and customresourcedefinitions.

Containers logs and events logs.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.TODO()
		params, err := configuration.NewParams(ctx, kubeconfig, namespaces, allNamespaces, clusterScope)
		if err != nil {
			return err
		}

		return collectinfo.RunCollectInfo(ctx, params, path)
	},
}

func init() {
	rootCmd.AddCommand(collectinfoCmd)

	collectinfoCmd.Flags().StringVar(&path, "path", "",
		"Absolute path where generated tar file will be saved")
}
