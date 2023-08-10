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

	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/auth"
	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/configuration"
	"github.com/spf13/cobra"
)

// authCmd represents the auth command
var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "auth command is used to create or delete RBAC resources for Aerospike cluster for the given namespaces",
	Long: `This command has subcommands that will create or delete RBAC resources for Aerospike cluster for the given 
namespaces.
It creates/deletes ServiceAccount, RoleBinding or ClusterRoleBinding as per given scope`,
}

var authCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "create command is used to create or update RBAC resources for Aerospike cluster for the given namespaces",
	Long: `This command will create RBAC resources for Aerospike cluster for the given 
namespaces.
It creates ServiceAccount, RoleBinding or ClusterRoleBinding as per given scope`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.TODO()
		params, err := configuration.NewParams(ctx, namespaces, allNamespaces, clusterScope)
		if err != nil {
			return err
		}

		return auth.Create(ctx, params)
	},
}

var authDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "delete command is used to delete RBAC resources for Aerospike cluster for the given namespaces",
	Long: `This command will delete RBAC resources for Aerospike cluster for the given 
namespaces.
It deletes ServiceAccount, RoleBinding or ClusterRoleBinding as per given scope`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.TODO()
		params, err := configuration.NewParams(ctx, namespaces, allNamespaces, clusterScope)
		if err != nil {
			return err
		}

		return auth.Delete(ctx, params)
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authCreateCmd)
	authCmd.AddCommand(authDeleteCmd)
}
