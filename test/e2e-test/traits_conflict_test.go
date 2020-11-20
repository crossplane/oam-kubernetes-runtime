package controllers_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
)

var _ = Describe("Trait conflict", func() {
	ctx := context.Background()
	apiVersion := "core.oam.dev/v1alpha2"
	namespace := "default"
	componentName := "conflict"
	appConfigName := "conflict"
	workload := v1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace},
		Spec: v1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nginx",
				},
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "nginx",
							Image: "nginx:1.9.4"},
					},
				},
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "nginx"}},
			},
		},
	}
	component := v1alpha2.Component{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiVersion,
			Kind:       "Component",
		},
		ObjectMeta: metav1.ObjectMeta{Name: componentName, Namespace: namespace},
		Spec: v1alpha2.ComponentSpec{
			Workload: runtime.RawExtension{Object: workload.DeepCopyObject()},
		},
	}

	traitTypeMeta := metav1.TypeMeta{
		APIVersion: apiVersion,
		Kind:       "TraitDefinition",
	}

	var traitDefinition1Name = "trait1.core.oam.dev"
	traitDefinition1 := v1alpha2.TraitDefinition{
		TypeMeta: traitTypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:      traitDefinition1Name,
			Namespace: namespace,
		},
		Spec: v1alpha2.TraitDefinitionSpec{
			Reference: v1alpha2.DefinitionReference{
				Name: traitDefinition1Name,
			},
			WorkloadRefPath: "spec.workloadRef",
		},
	}

	var traitDefinition2Name = "trait2.core.oam.dev"
	traitDefinition2 := v1alpha2.TraitDefinition{
		TypeMeta: traitTypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:      traitDefinition2Name,
			Namespace: namespace,
		},
		Spec: v1alpha2.TraitDefinitionSpec{
			Reference: v1alpha2.DefinitionReference{
				Name: traitDefinition2Name,
			},
			WorkloadRefPath: "spec.workloadRef",
			ConflictsWith:   []string{traitDefinition1Name},
		},
	}

	var traitDefinition3Name = "trait3.core.oam.dev"
	traitDefinition3 := v1alpha2.TraitDefinition{
		TypeMeta: traitTypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:      traitDefinition3Name,
			Namespace: namespace,
		},
		Spec: v1alpha2.TraitDefinitionSpec{
			Reference: v1alpha2.DefinitionReference{
				Name: traitDefinition3Name,
			},
			WorkloadRefPath: "spec.workloadRef",
		},
	}

	var trait1, trait2, trait3 unstructured.Unstructured
	trait1.SetAPIVersion(apiVersion)
	trait1.SetKind("Trait1")
	trait1.SetLabels(map[string]string{
		oam.TraitTypeLabel: traitDefinition1Name,
	})

	trait2.SetAPIVersion(apiVersion)
	trait2.SetKind("Trait2")
	trait2.SetLabels(map[string]string{
		oam.TraitTypeLabel: traitDefinition2Name,
	})

	trait3.SetAPIVersion(apiVersion)
	trait3.SetKind("Trait3")
	trait3.SetLabels(map[string]string{
		oam.TraitTypeLabel: traitDefinition3Name,
	})

	appConfig := v1alpha2.ApplicationConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiVersion,
			Kind:       "ApplicationConfiguration",
		},
		ObjectMeta: metav1.ObjectMeta{Name: appConfigName, Namespace: namespace},
		Spec: v1alpha2.ApplicationConfigurationSpec{
			Components: []v1alpha2.ApplicationConfigurationComponent{{
				ComponentName: componentName,
				Traits: []v1alpha2.ComponentTrait{
					{Trait: runtime.RawExtension{Object: trait1.DeepCopyObject()}},
					{Trait: runtime.RawExtension{Object: trait2.DeepCopyObject()}},
					{Trait: runtime.RawExtension{Object: trait3.DeepCopyObject()}},
				},
			},
			},
		},
	}

	appConfigObjKey := client.ObjectKey{Name: appConfigName, Namespace: namespace}

	BeforeEach(func() {
		Expect(k8sClient.Create(ctx, &traitDefinition1)).Should(Succeed())
		Expect(k8sClient.Create(ctx, &traitDefinition2)).Should(Succeed())
		Expect(k8sClient.Create(ctx, &traitDefinition3)).Should(Succeed())
		Expect(k8sClient.Create(ctx, &component)).Should(Succeed())
	})

	Context("Apply conflicted traits", func() {
		It("Apply conflicted traits when there is no existing traits", func() {
			By("submit ApplicationConfiguration")
			err := k8sClient.Create(ctx, &appConfig)
			Expect(err).Should(HaveOccurred())
		})

		It("Apply a trait which conflicts to an existing traits", func() {
			By("submit ApplicationConfiguration")
			appConfig.Spec.Components[0].Traits = []v1alpha2.ComponentTrait{{Trait: runtime.RawExtension{Object: trait1.DeepCopyObject()}}}
			Expect(k8sClient.Create(ctx, &appConfig)).Should(Succeed())

			By("apply new ApplicationConfiguration with a conflicted trait")
			Expect(k8sClient.Get(ctx, appConfigObjKey, &appConfig)).Should(Succeed())
			appConfig.Spec.Components[0].Traits = []v1alpha2.ComponentTrait{
				{Trait: runtime.RawExtension{Object: trait1.DeepCopyObject()}},
				{Trait: runtime.RawExtension{Object: trait2.DeepCopyObject()}},
				{Trait: runtime.RawExtension{Object: trait3.DeepCopyObject()}},
			}
			Expect(k8sClient.Update(ctx, &appConfig)).Should(HaveOccurred())
		})
	})

	AfterEach(func() {
		k8sClient.Delete(ctx, &appConfig)
		k8sClient.Delete(ctx, &component)
		k8sClient.Delete(ctx, &traitDefinition1)
		k8sClient.Delete(ctx, &traitDefinition2)
		k8sClient.Delete(ctx, &traitDefinition3)
	})
})
