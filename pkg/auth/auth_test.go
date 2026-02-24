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

package auth_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/auth"
	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/configuration"
	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/internal"
	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/testutils"
)

var _ = Describe("Auth", func() {
	Context("Create and Delete ", func() {
		It("Should create and delete namespace level RBAC", func() {
			testCreateRbac([]string{namespace}, false)
			testDeleteRbac([]string{namespace}, false, true)
		})

		It("Should create and delete cluster level RBAC", func() {
			testCreateRbac([]string{namespace}, true)
			testDeleteRbac([]string{namespace}, true, true)
		})
	})

	Context("Update and Delete", func() {
		It("Should add or remove new entry for new namespace for cluster level RBAC", func() {
			By("Creating RBAC for testns namespace")
			testCreateRbac([]string{namespace}, true)

			By("Creating RBAC for default namespace")

			defaultNs := "default"
			testCreateRbac([]string{defaultNs}, true)

			By("Creating RBAC for default namespace again to check duplicate entries")
			testCreateRbac([]string{defaultNs}, true)

			By("Deleting RBAC")
			testDeleteRbac([]string{defaultNs}, true, false)
			testDeleteRbac([]string{namespace}, true, true)
		})
	})

	Context("Wrong kubeconfig path", func() {
		It("Should fail when wrong kubeconfig path is given", func() {
			_, err := configuration.NewParams(testCtx, "wrongpath", []string{namespace},
				false, false)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("wrongpath: no such file or directory"))
		})
	})
})

func testCreateRbac(namespaces []string, clusterScope bool) {
	params, err := testutils.NewTestParams(testCtx, k8sClient, nil, namespaces, false, clusterScope)
	Expect(err).NotTo(HaveOccurred())
	Expect(params).NotTo(BeNil())
	Expect(auth.Create(testCtx, params)).NotTo(HaveOccurred())

	validateRbacCreate(params)
}

func validateRbacCreate(params *configuration.Parameters) {
	for ns := range params.Namespaces {
		sa := &v1.ServiceAccount{}
		err := params.K8sClient.Get(testCtx, types.NamespacedName{
			Namespace: ns,
			Name:      auth.ServiceAccountName,
		}, sa)

		Expect(err).NotTo(HaveOccurred())

		if !params.ClusterScope {
			roleBinding := &rbac.RoleBinding{}
			err := params.K8sClient.Get(testCtx, types.NamespacedName{
				Namespace: ns,
				Name:      auth.RoleBindingName,
			}, roleBinding)

			Expect(err).NotTo(HaveOccurred())
		}
	}

	if params.ClusterScope {
		crb := &rbac.ClusterRoleBinding{}
		err := params.K8sClient.Get(testCtx, types.NamespacedName{
			Name: auth.ClusterRoleBindingName,
		}, crb)

		Expect(err).NotTo(HaveOccurred())
		Expect(len(crb.Subjects) >= params.Namespaces.Len()).To(BeTrue())

		var newEntries int

		for _, sub := range crb.Subjects {
			Expect(sub.Kind).To(Equal(internal.ServiceAccountKind))
			Expect(sub.Name).To(Equal(auth.ServiceAccountName))

			if params.Namespaces.Has(sub.Namespace) {
				newEntries++
			}
		}
		// Check new entries are as per the given input namespaces
		Expect(newEntries).To(Equal(len(params.Namespaces)))
	}
}

func testDeleteRbac(namespaces []string, clusterScope, lastEntry bool) {
	params, err := testutils.NewTestParams(testCtx, k8sClient, nil, namespaces, false, clusterScope)
	Expect(err).NotTo(HaveOccurred())
	Expect(params).NotTo(BeNil())
	Expect(auth.Delete(testCtx, params)).NotTo(HaveOccurred())

	validateRbacDelete(params, lastEntry)
}

func validateRbacDelete(params *configuration.Parameters, lastEntry bool) {
	for ns := range params.Namespaces {
		sa := &v1.ServiceAccount{}
		err := params.K8sClient.Get(testCtx, types.NamespacedName{
			Namespace: ns,
			Name:      auth.ServiceAccountName,
		}, sa)

		Expect(err).To(HaveOccurred())
		Expect(errors.IsNotFound(err)).To(BeTrue())

		if !params.ClusterScope {
			roleBinding := &rbac.RoleBinding{}
			err := params.K8sClient.Get(testCtx, types.NamespacedName{
				Namespace: ns,
				Name:      auth.RoleBindingName,
			}, roleBinding)

			Expect(err).To(HaveOccurred())
			Expect(errors.IsNotFound(err)).To(BeTrue())
		}
	}

	if params.ClusterScope {
		crb := &rbac.ClusterRoleBinding{}
		err := params.K8sClient.Get(testCtx, types.NamespacedName{
			Name: auth.ClusterRoleBindingName,
		}, crb)

		if lastEntry {
			Expect(err).To(HaveOccurred())
			Expect(errors.IsNotFound(err)).To(BeTrue())

			return
		}

		for _, sub := range crb.Subjects {
			Expect(sub.Kind).To(Equal(internal.ServiceAccountKind))
			Expect(sub.Name).To(Equal(auth.ServiceAccountName))
			Expect(params.Namespaces.Has(sub.Namespace)).To(BeFalse())
		}
	}
}
