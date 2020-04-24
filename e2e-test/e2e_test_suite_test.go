package e2e_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/onsi/gomega/format"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
)

func TestE2eTest(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2eTest Suite")
}

var k8sClient client.Client
var scheme = runtime.NewScheme()

var _ = BeforeSuite(func(done Done) {
	By("Bootstrapping test environment")
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	err := clientgoscheme.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = core.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	k8sClient, err = client.New(config.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		logf.Log.Error(err, "failed to create k8sClient")
		Fail("setup failed")
	}
	close(done)
}, 30)

var _ = AfterSuite(func() {
	By("Tearing down the test environment")
})

// Match the error to be already exist
type AlreadyExistMatcher struct {
}

func (matcher AlreadyExistMatcher) Match(actual interface{}) (success bool, err error) {
	if actual == nil {
		return false, nil
	}
	actualError := actual.(error)
	return apierrors.IsAlreadyExists(actualError), nil
}

func (matcher AlreadyExistMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to be already exist")
}

func (matcher AlreadyExistMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to be already exist")
}

// Match the error to be not found
type NotFoundMatcher struct {
}

func (matcher NotFoundMatcher) Match(actual interface{}) (success bool, err error) {
	if actual == nil {
		return false, nil
	}
	actualError := actual.(error)
	return apierrors.IsNotFound(actualError), nil
}

func (matcher NotFoundMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to be already exist")
}

func (matcher NotFoundMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to be already exist")
}
