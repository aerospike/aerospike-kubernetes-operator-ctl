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
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/aerospike/aerospike-kubernetes-operator-ctl/pkg/testutils"
)

var (
	testEnv   *envtest.Environment
	k8sClient client.Client
	testCtx   = context.TODO()
	namespace = "testns"
)

func TestPkg(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Auth Suite")
}

var _ = BeforeSuite(
	func() {
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		By("Bootstrapping test environment")
		testEnv = &envtest.Environment{}

		cfg, err := testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg).NotTo(BeNil())

		scheme := runtime.NewScheme()

		err = clientgoscheme.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		k8sClient, err = client.New(
			cfg, client.Options{Scheme: scheme},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient).NotTo(BeNil())

		err = testutils.CreateNamespace(testCtx, k8sClient, namespace)
		Expect(err).NotTo(HaveOccurred())
	})

var _ = AfterSuite(
	func() {
		By("Tearing down the test environment")
		gexec.KillAndWait(5 * time.Second)
		err := testEnv.Stop()
		Expect(err).ToNot(HaveOccurred())
	},
)
