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
	"akoctl/pkg"
	"fmt"
	"k8s.io/client-go/util/homedir"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	kubeconfig  *string
	namespaces  []string
	pathToStore *string
)

// collectinfoCmd represents the collectinfo command
var collectinfoCmd = &cobra.Command{
	Use:   "collectinfo",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		pkg.CollectInfo(namespaces, kubeconfig, pathToStore)
	},
}

func init() {
	rootCmd.AddCommand(collectinfoCmd)

	collectinfoCmd.Flags().StringSliceVar(&namespaces, "namespaces", namespaces, fmt.Sprintf("Namespaces for which logs to be collected"))
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = collectinfoCmd.Flags().String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = collectinfoCmd.Flags().String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

	pathToStore = collectinfoCmd.Flags().String("pathtostore", "", "absolute path where generated tar file will be saved")
}
