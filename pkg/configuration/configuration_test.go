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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/configuration"
)

// testScheme returns a scheme that knows about core types (incl. Namespace).
func testScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())

	return scheme
}

// seededClient builds a fake client preloaded with the given namespaces. GET of
// a seeded namespace succeeds; GET of any other namespace returns NotFound, and
// LIST returns the seeded set.
func seededClient(seededNamespaces ...string) client.Client {
	objs := make([]client.Object, 0, len(seededNamespaces))
	for _, name := range seededNamespaces {
		objs = append(objs, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})
	}

	return fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(objs...).
		Build()
}

// denyNamespaceAccessClient builds a fake client that forbids both GET and LIST
// of namespaces, mirroring a ServiceAccount that has no cluster-wide namespace
// permissions at all.
func denyNamespaceAccessClient() client.Client {
	forbidden := func(verb string) error {
		return apierrors.NewForbidden(
			schema.GroupResource{Resource: "namespaces"}, "",
			fmt.Errorf("cannot %s resource \"namespaces\" at the cluster scope", verb))
	}

	return fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList,
				opts ...client.ListOption) error {
				if _, ok := list.(*corev1.NamespaceList); ok {
					return forbidden("list")
				}

				return c.List(ctx, list, opts...)
			},
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey,
				obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*corev1.Namespace); ok {
					return forbidden("get")
				}

				return c.Get(ctx, key, obj, opts...)
			},
		}).
		Build()
}

func newParams(k8sClient client.Client, allNamespaces bool) *configuration.Parameters {
	return &configuration.Parameters{
		K8sClient:     k8sClient,
		Logger:        configuration.InitializeConsoleLogger(),
		AllNamespaces: allNamespaces,
	}
}

var _ = Describe("ValidateNamespaces", func() {
	ctx := context.TODO()

	Context("when neither namespaces nor all-namespaces is provided", func() {
		It("should return an error without contacting the API server", func() {
			// nil client ensures no API call is attempted before the validation error.
			params := newParams(nil, false)

			err := params.ValidateNamespaces(ctx, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("either `namespaces` or `all-namespaces`"))
		})
	})

	Context("when namespaces are provided via -n", func() {
		It("should keep all provided namespaces when they exist in the cluster", func() {
			params := newParams(seededClient("foo", "bar"), false)

			err := params.ValidateNamespaces(ctx, []string{"foo", "bar"})
			Expect(err).NotTo(HaveOccurred())
			Expect(params.Namespaces.UnsortedList()).To(ConsistOf("foo", "bar"))
		})

		It("should drop namespaces that do not exist and keep the rest", func() {
			// Only "foo" exists; "bar" should be skipped with a warning.
			params := newParams(seededClient("foo"), false)

			err := params.ValidateNamespaces(ctx, []string{"foo", "bar"})
			Expect(err).NotTo(HaveOccurred())
			Expect(params.Namespaces.UnsortedList()).To(ConsistOf("foo"))
		})

		It("should error when none of the provided namespaces exist", func() {
			params := newParams(seededClient(), false)

			err := params.ValidateNamespaces(ctx, []string{"foo", "bar"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("all given namespaces are not present"))
		})

		It("should succeed without requiring any cluster-wide namespace permission", func() {
			// The SA can neither GET nor LIST namespaces; existence validation is
			// skipped and the provided namespaces are used as-is.
			params := newParams(denyNamespaceAccessClient(), false)

			err := params.ValidateNamespaces(ctx, []string{"foo", "bar"})
			Expect(err).NotTo(HaveOccurred())
			Expect(params.Namespaces.UnsortedList()).To(ConsistOf("foo", "bar"))
		})

		It("should filter out empty-string entries caused by trailing or doubled commas", func() {
			params := newParams(seededClient("foo"), false)

			err := params.ValidateNamespaces(ctx, []string{"foo", "", ""})
			Expect(err).NotTo(HaveOccurred())
			Expect(params.Namespaces.UnsortedList()).To(ConsistOf("foo"))
		})

		It("should error when all provided namespace values are empty strings", func() {
			params := newParams(seededClient(), false)

			err := params.ValidateNamespaces(ctx, []string{"", ""})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("all provided namespace values are empty"))
		})
	})

	Context("when all-namespaces (-A) is set", func() {
		It("should list and capture every namespace in the cluster", func() {
			params := newParams(seededClient("ns1", "ns2", "ns3"), true)

			err := params.ValidateNamespaces(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(params.Namespaces.UnsortedList()).To(ConsistOf("ns1", "ns2", "ns3"))
		})

		It("should ignore namespaces passed via -n and use the full cluster list", func() {
			params := newParams(seededClient("ns1", "ns2"), true)

			err := params.ValidateNamespaces(ctx, []string{"only-this-one"})
			Expect(err).NotTo(HaveOccurred())
			Expect(params.Namespaces.UnsortedList()).To(ConsistOf("ns1", "ns2"))
		})

		It("should return an error when the namespace list is forbidden", func() {
			// -A still depends on cluster-wide namespace list permission.
			params := newParams(denyNamespaceAccessClient(), true)

			err := params.ValidateNamespaces(ctx, nil)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsForbidden(err)).To(BeTrue())
		})
	})
})
