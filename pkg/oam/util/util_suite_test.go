package util_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestUtil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OAM Util Suite")
}

var _ = BeforeSuite(func(done Done) {
	By("Bootstrapping OAM util test environment")
	close(done)
}, 300)

var _ = AfterSuite(func() {
	By("Tearing down the OAM util test environment")
})
