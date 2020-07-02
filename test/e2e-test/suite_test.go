/*

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

package controllers_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	rbac "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
	// +kubebuilder:scaffold:imports
)

var k8sClient client.Client
var scheme = runtime.NewScheme()
var manualscalertrait v1alpha2.TraitDefinition
var roleBindingName = "oam-role-binding"

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"OAM Core Resource Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	By("Bootstrapping test environment")
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	err := clientgoscheme.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = core.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	By("Setting up kubernetes client")
	k8sClient, err = client.New(config.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		logf.Log.Error(err, "failed to create k8sClient")
		Fail("setup failed")
	}
	By("Finished setting up test environment")

	// Create manual scaler trait definition
	manualscalertrait = v1alpha2.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "manualscalertraits.core.oam.dev",
			Labels: map[string]string{"trait": "manualscalertrait"},
		},
		Spec: v1alpha2.TraitDefinitionSpec{
			WorkloadRefPath: "spec.workloadRef",
			Reference: v1alpha2.DefinitionReference{
				Name: "manualscalertraits.core.oam.dev",
			},
		},
	}
	// For some reason, traitDefinition is created as a Cluster scope object
	Expect(k8sClient.Create(context.Background(), &manualscalertrait)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	By("Created manual scalar trait definition")
	adminRoleBinding := rbac.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   roleBindingName,
			Labels: map[string]string{"oam": "clusterrole"},
		},
		Subjects: []rbac.Subject{
			{
				Kind: "User",
				Name: "system:serviceaccount:crossplane-system:crossplane",
			},
		},
		RoleRef: rbac.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
	}
	Expect(k8sClient.Create(context.Background(), &adminRoleBinding)).Should(BeNil())
	By("Created cluster role bind for the test service account")
	close(done)
}, 300)

var _ = AfterSuite(func() {
	By("Tearing down the test environment")
	adminRoleBinding := rbac.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   roleBindingName,
			Labels: map[string]string{"oam": "clusterrole"},
		},
	}
	Expect(k8sClient.Delete(context.Background(), &adminRoleBinding)).Should(BeNil())
	By("Deleted the cluster role binding")
	manualscalertrait = v1alpha2.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "manualscalertraits.core.oam.dev",
			Labels: map[string]string{"trait": "manualscalertrait"},
		},
	}
	Expect(k8sClient.Delete(context.Background(), &manualscalertrait)).Should(BeNil())
	By("Deleted the manual scalertrait definition")

})
