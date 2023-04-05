package pkg_test

import (
	goctx "context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	corev1 "k8s.io/api/core/v1"
	apixv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apixv1beta1client "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	testEnv      *envtest.Environment
	cfg          *rest.Config
	k8sClient    client.Client
	namespace    = "testns"
	k8sClientset *kubernetes.Clientset
	apixClient   *apixv1beta1client.ApiextensionsV1beta1Client
)

func TestPkg(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pkg Suite")
}

var _ = BeforeSuite(
	func() {
		//		Expect(os.Setenv("KUBEBUILDER_ASSETS", "../bin/k8s/1.26.1-darwin-arm64")).To(Succeed())
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		By("Bootstrapping test environment")
		t := false
		testEnv = &envtest.Environment{
			UseExistingCluster:    &t,
			CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
			ErrorIfCRDPathMissing: true,
		}
		var err error

		cfg, err = testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg).NotTo(BeNil())

		err = clientgoscheme.AddToScheme(clientgoscheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		err = apixv1beta1.AddToScheme(clientgoscheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		// +kubebuilder:scaffold:scheme

		k8sClient, err = client.New(
			cfg, client.Options{Scheme: clientgoscheme.Scheme},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient).NotTo(BeNil())

		apixClient, err = apixv1beta1client.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(apixClient).NotTo(BeNil())

		k8sClientset = kubernetes.NewForConfigOrDie(cfg)
		Expect(k8sClient).NotTo(BeNil())

		ctx := goctx.TODO()
		_ = createNamespace(k8sClient, ctx, namespace)
	})

var _ = AfterSuite(
	func() {
		By("tearing down the test environment")
		gexec.KillAndWait(5 * time.Second)
		err := testEnv.Stop()
		Expect(err).ToNot(HaveOccurred())
	},
)

func createNamespace(
	k8sClient client.Client, ctx goctx.Context, name string,
) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	err := k8sClient.Create(ctx, ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}
