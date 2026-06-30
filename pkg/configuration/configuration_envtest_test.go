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

package configuration_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// These specs exercise ValidateNamespaces against a real (envtest) API server
// with RBAC enforced. They impersonate a ServiceAccount that has namespace
// scoped read access but NO cluster-wide permission to list namespaces, which
// is exactly the limited identity the -n flag is meant to support.
var _ = Describe("ValidateNamespaces against a real API server with RBAC", Ordered, func() {
	ctx := context.TODO()

	const (
		saNamespace = "limited-ns"
		saName      = "akoctl-limited"
	)

	var limitedClient client.Client

	BeforeAll(func() {
		if !envtestReady {
			Skip("envtest assets not available; set KUBEBUILDER_ASSETS or run via `make test`")
		}

		By("Creating a namespace, ServiceAccount and a namespace-scoped (no namespace list) Role")
		Expect(adminClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: saNamespace},
		})).To(Succeed())

		Expect(adminClient.Create(ctx, &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: saName, Namespace: saNamespace},
		})).To(Succeed())

		// Grants reads within saNamespace only. Crucially, it does NOT grant the
		// cluster-scoped "list namespaces" verb.
		Expect(adminClient.Create(ctx, &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: "akoctl-reader", Namespace: saNamespace},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods", "services", "configmaps", "persistentvolumeclaims"},
					Verbs:     []string{"get", "list"},
				},
			},
		})).To(Succeed())

		Expect(adminClient.Create(ctx, &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "akoctl-reader", Namespace: saNamespace},
			Subjects: []rbacv1.Subject{{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      saName,
				Namespace: saNamespace,
			}},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "Role",
				Name:     "akoctl-reader",
			},
		})).To(Succeed())

		By("Building a client that impersonates the limited ServiceAccount")

		impCfg := rest.CopyConfig(envCfg)
		impCfg.Impersonate = rest.ImpersonationConfig{
			UserName: fmt.Sprintf("system:serviceaccount:%s:%s", saNamespace, saName),
		}

		var err error

		limitedClient, err = client.New(impCfg, client.Options{Scheme: testScheme()})
		Expect(err).NotTo(HaveOccurred())

		By("Confirming the impersonated SA really cannot list namespaces")

		nsList := &corev1.NamespaceList{}
		err = limitedClient.List(ctx, nsList)
		Expect(apierrors.IsForbidden(err)).To(BeTrue())
	})

	It("should accept -n namespaces even when the SA cannot verify their existence", func() {
		// The SA cannot GET or LIST namespaces, so existence validation is skipped
		// and the provided namespaces are used as-is. This must not require any
		// cluster-wide namespace permission.
		params := newParams(limitedClient, false)

		err := params.ValidateNamespaces(ctx, []string{"foo", "bar"})
		Expect(err).NotTo(HaveOccurred())
		Expect(params.Namespaces.UnsortedList()).To(ConsistOf("foo", "bar"))
	})

	It("should fail with a Forbidden error when -A is used by the limited SA", func() {
		params := newParams(limitedClient, true)

		err := params.ValidateNamespaces(ctx, nil)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsForbidden(err)).To(BeTrue())
	})

	It("should drop a non-existent namespace when the caller can GET namespaces", func() {
		// Admin can GET namespaces, so existence validation runs for real:
		// the existing namespace is kept and the missing one is dropped.
		params := newParams(adminClient, false)

		err := params.ValidateNamespaces(ctx, []string{saNamespace, "does-not-exist"})
		Expect(err).NotTo(HaveOccurred())
		Expect(params.Namespaces.UnsortedList()).To(ConsistOf(saNamespace))
	})

	It("should list every namespace when -A is used by an admin", func() {
		params := newParams(adminClient, true)

		err := params.ValidateNamespaces(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(params.Namespaces.Has("default")).To(BeTrue())
		Expect(params.Namespaces.Has(saNamespace)).To(BeTrue())
	})
})
