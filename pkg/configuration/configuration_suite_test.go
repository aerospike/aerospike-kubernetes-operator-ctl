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
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	testEnv      *envtest.Environment
	envCfg       *rest.Config
	adminClient  client.Client
	envtestReady bool
)

func TestConfiguration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Configuration Suite")
}

var _ = BeforeSuite(func() {
	// The fake-client specs need no API server. Only the RBAC specs require
	// envtest; when its binaries are not provisioned (e.g. plain `go test`
	// without KUBEBUILDER_ASSETS), leave envtestReady false so those specs skip.
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		return
	}

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("Bootstrapping test environment")

	testEnv = &envtest.Environment{}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	adminClient, err = client.New(cfg, client.Options{Scheme: testScheme()})
	Expect(err).NotTo(HaveOccurred())
	Expect(adminClient).NotTo(BeNil())

	envCfg = cfg
	envtestReady = true
})

var _ = AfterSuite(func() {
	if testEnv != nil {
		By("Tearing down the test environment")
		Expect(testEnv.Stop()).To(Succeed())
	}
})
