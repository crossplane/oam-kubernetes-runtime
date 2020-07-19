package controllers_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

var _ = Describe("Resource Dependency in an ApplicationConfiguration", func() {
	ctx := context.Background()
	namespace := "appconfig-dependency-test"
	var ns corev1.Namespace
	var wd v1alpha2.WorkloadDefinition
	var td v1alpha2.TraitDefinition
	tempFoo := &unstructured.Unstructured{}
	tempFoo.SetAPIVersion("example.com/v1")
	tempFoo.SetKind("Foo")
	tempFoo.SetNamespace(namespace)
	BeforeEach(func() {
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		logf.Log.Info("Start to run a test, clean up previous resources")
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).
			Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		logf.Log.Info("make sure all the resources are removed")
		objectKey := client.ObjectKey{
			Name: namespace,
		}
		res := &corev1.Namespace{}
		Eventually(
			// gomega has a bug that can't take nil as the actual input, so has to make it a func
			func() error {
				return k8sClient.Get(ctx, objectKey, res)
			},
			time.Second*30, time.Millisecond*500).Should(&util.NotFoundMatcher{})
		// recreate it
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		wd = v1alpha2.WorkloadDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo.example.com",
			},
			Spec: v1alpha2.WorkloadDefinitionSpec{
				Reference: v1alpha2.DefinitionReference{
					Name: "foo.example.com",
				},
			},
		}
		td = v1alpha2.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foos.example.com",
			},
			Spec: v1alpha2.TraitDefinitionSpec{
				Reference: v1alpha2.DefinitionReference{
					Name: "foos.example.com",
				},
			},
		}
		logf.Log.Info("Creating workload definition")
		// For some reason, WorkloadDefinition is created as a Cluster scope object
		Expect(k8sClient.Create(ctx, &wd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		logf.Log.Info("Creating trait definition")
		// For some reason, TraitDefinition is created as a Cluster scope object
		Expect(k8sClient.Create(ctx, &td)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})
	AfterEach(func() {
		logf.Log.Info("Clean up resources")
		// Delete the WorkloadDefinition and TraitDefinition
		Expect(k8sClient.Delete(ctx, &wd)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, &td)).Should(BeNil())
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(BeNil())
	})

	// common function for verification
	verify := func(appConfigName, outName, inName string) {
		// Verification before satisfying dependency
		By("Checking that resource which accepts data isn't created yet")
		inFooKey := client.ObjectKey{
			Name:      inName,
			Namespace: namespace,
		}
		inFoo := tempFoo.DeepCopy()
		logf.Log.Info("Checking on resource that inputs data", "Key", inFooKey)
		Eventually(
			func() error {
				return k8sClient.Get(ctx, inFooKey, inFoo)
			},
			time.Second*15, time.Millisecond*500).Should(&util.NotFoundMatcher{})
		By("Checking that resource which provides data is created")
		outFooKey := client.ObjectKey{
			Name:      outName,
			Namespace: namespace,
		}
		outFoo := tempFoo.DeepCopy()
		logf.Log.Info("Checking on resource that outputs data", "Key", outFoo)
		Eventually(
			func() error {
				return k8sClient.Get(ctx, outFooKey, outFoo)
			},
			time.Second*15, time.Millisecond*500).Should(BeNil())
		By("Verify the appconfig's dependency is unsatisfied")
		appconfigKey := client.ObjectKey{
			Name:      appConfigName,
			Namespace: namespace,
		}
		appconfig := &v1alpha2.ApplicationConfiguration{}
		depStatus := v1alpha2.DependencyStatus{
			Unsatisfied: []v1alpha2.UnstaifiedDependency{
				{
					From: v1alpha2.DependencyFromObject{
						TypedReference: v1alpha1.TypedReference{
							APIVersion: tempFoo.GetAPIVersion(),
							Name:       outName,
							Kind:       tempFoo.GetKind(),
						},
						FieldPath: "status.key",
					},
					To: v1alpha2.DependencyToObject{
						TypedReference: v1alpha1.TypedReference{
							APIVersion: tempFoo.GetAPIVersion(),
							Name:       inName,
							Kind:       tempFoo.GetKind(),
						},
						FieldPaths: []string{
							"spec.key",
						},
					},
				},
			},
		}
		logf.Log.Info("Checking on appconfig", "Key", appconfigKey)
		Eventually(
			func() v1alpha2.DependencyStatus {
				k8sClient.Get(ctx, appconfigKey, appconfig)
				return appconfig.Status.Dependency
			},
			time.Second*15, time.Millisecond*500).Should(Equal(depStatus))
		// fill value to fieldPath
		err := unstructured.SetNestedField(outFoo.Object, "test", "status", "key")
		Expect(err).Should(BeNil())
		Expect(k8sClient.Update(ctx, outFoo)).Should(Succeed())
		// Verification after satisfying dependency
		By("Checking that resource which accepts data is created now")
		logf.Log.Info("Checking on resource that inputs data", "Key", inFooKey)
		Eventually(
			func() error {
				return k8sClient.Get(ctx, inFooKey, inFoo)
			},
			time.Second*15, time.Millisecond*500).Should(BeNil())
		By("Verify the appconfig's dependency is unsatisfied")
		appconfig = &v1alpha2.ApplicationConfiguration{}
		logf.Log.Info("Checking on appconfig", "Key", appconfigKey)
		Eventually(
			func() []v1alpha2.UnstaifiedDependency {
				k8sClient.Get(ctx, appconfigKey, appconfig)
				return appconfig.Status.Dependency.Unsatisfied
			},
			time.Second*15, time.Millisecond*500).Should(BeNil())
	}

	It("trait depends on another trait", func() {
		label := map[string]string{"trait": "trait"}
		// Define a trait which provides data
		outName := "trait-out"
		tdOut := tempFoo.DeepCopy()
		tdOut.SetName(outName)
		// Define a trait which accepts data
		inName := "trait-in"
		tdIn := tempFoo.DeepCopy()
		tdIn.SetName(inName)
		// Define a workload
		wlName := "wl"
		wl := tempFoo.DeepCopy()
		wl.SetName(wlName)
		// Create a component
		componentName := "component"
		comp := v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: wl,
				},
			},
		}
		Expect(k8sClient.Create(ctx, &comp)).Should(BeNil())
		logf.Log.Info("Creating component", "Name", comp.Name, "Namespace", comp.Namespace)
		// Create application configuration
		appConfigName := "appconfig-trait-trait"
		appConfig := v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appConfigName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{
					{
						ComponentName: componentName,
						Traits: []v1alpha2.ComponentTrait{
							{
								Trait: runtime.RawExtension{
									Object: tdOut,
								},
								DataOutputs: []v1alpha2.DataOutput{
									{
										Name:      "trait-trait",
										FieldPath: "status.key",
									},
								},
							},
							{
								Trait: runtime.RawExtension{
									Object: tdIn,
								},
								DataInputs: []v1alpha2.DataInput{
									{
										ValueFrom: v1alpha2.DataInputValueFrom{
											DataOutputName: "trait-trait",
										},
										ToFieldPaths: []string{"spec.key"},
									},
								},
							},
						},
					},
				},
			},
		}
		logf.Log.Info("Creating application config", "Name", appConfig.Name, "Namespace", appConfig.Namespace)
		Expect(k8sClient.Create(ctx, &appConfig)).Should(BeNil())
		verify(appConfigName, outName, inName)
	})

	It("component depends on another component", func() {
		label := map[string]string{"component": "component"}
		// Define a workload which provides data
		outName := "comp-out"
		wlOut := tempFoo.DeepCopy()
		wlOut.SetName(outName)
		// Define a workload which accepts data
		inName := "comp-in"
		wlIn := tempFoo.DeepCopy()
		wlIn.SetName(inName)
		// Create a component
		componentOutName := "component-comp-out"
		compOut := v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentOutName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: wlOut,
				},
			},
		}
		Expect(k8sClient.Create(ctx, &compOut)).Should(BeNil())
		logf.Log.Info("Creating component that outputs data", "Name", compOut.Name, "Namespace", compOut.Namespace)
		// Create another component
		componentInName := "component-comp-in"
		compIn := v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentInName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: wlIn,
				},
			},
		}
		Expect(k8sClient.Create(ctx, &compIn)).Should(BeNil())
		logf.Log.Info("Creating component that inputs data", "Name", compIn.Name, "Namespace", compIn.Namespace)
		// Create application configuration
		appConfigName := "appconfig-comp-comp"
		appConfig := v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appConfigName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{
					{
						ComponentName: componentOutName,
						DataOutputs: []v1alpha2.DataOutput{
							{
								Name:      "comp-comp",
								FieldPath: "status.key",
							},
						},
					},
					{
						ComponentName: componentInName,
						DataInputs: []v1alpha2.DataInput{
							{
								ValueFrom: v1alpha2.DataInputValueFrom{
									DataOutputName: "comp-comp",
								},
								ToFieldPaths: []string{"spec.key"},
							},
						},
					},
				},
			},
		}
		logf.Log.Info("Creating application config", "Name", appConfig.Name, "Namespace", appConfig.Namespace)
		Expect(k8sClient.Create(ctx, &appConfig)).Should(BeNil())
		verify(appConfigName, outName, inName)
	})

	It("component depends on trait", func() {
		label := map[string]string{"trait": "component"}
		// Define a trait which provides data
		outName := "trait-out"
		tdOut := tempFoo.DeepCopy()
		tdOut.SetName(outName)
		// Define a workload which accepts data
		inName := "comp-in"
		wlIn := tempFoo.DeepCopy()
		wlIn.SetName(inName)
		// Create a component
		componentInName := "component-comp-in"
		compIn := v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentInName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: wlIn,
				},
			},
		}
		Expect(k8sClient.Create(ctx, &compIn)).Should(BeNil())
		logf.Log.Info("Creating component that inputs data", "Name", compIn.Name, "Namespace", compIn.Namespace)
		// Create application configuration
		appConfigName := "appconfig-trait-comp"
		appConfig := v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appConfigName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{
					{
						ComponentName: componentInName,
						DataInputs: []v1alpha2.DataInput{
							{
								ValueFrom: v1alpha2.DataInputValueFrom{
									DataOutputName: "trait-comp",
								},
								ToFieldPaths: []string{"spec.key"},
							},
						},
						Traits: []v1alpha2.ComponentTrait{
							{
								Trait: runtime.RawExtension{
									Object: tdOut,
								},
								DataOutputs: []v1alpha2.DataOutput{
									{
										Name:      "trait-comp",
										FieldPath: "status.key",
									},
								},
							},
						},
					},
				},
			},
		}
		logf.Log.Info("Creating application config", "Name", appConfig.Name, "Namespace", appConfig.Namespace)
		Expect(k8sClient.Create(ctx, &appConfig)).Should(BeNil())
		verify(appConfigName, outName, inName)
	})

	It("trait depends on component", func() {
		label := map[string]string{"component": "trait"}
		// Define a workload which provides data
		outName := "comp-out"
		wlOut := tempFoo.DeepCopy()
		wlOut.SetName(outName)
		// Define a trait which accepts data
		inName := "trait-in"
		tdIn := tempFoo.DeepCopy()
		tdIn.SetName(inName)
		// Create a component
		componentOutName := "component-comp-out"
		compOut := v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentOutName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: wlOut,
				},
			},
		}
		Expect(k8sClient.Create(ctx, &compOut)).Should(BeNil())
		logf.Log.Info("Creating component that outputs data", "Name", compOut.Name, "Namespace", compOut.Namespace)
		// Create application configuration
		appConfigName := "appconfig-comp-trait"
		appConfig := v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appConfigName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{
					{
						ComponentName: componentOutName,
						DataOutputs: []v1alpha2.DataOutput{
							{
								Name:      "comp-trait",
								FieldPath: "status.key",
							},
						},
						Traits: []v1alpha2.ComponentTrait{
							{
								Trait: runtime.RawExtension{
									Object: tdIn,
								},
								DataInputs: []v1alpha2.DataInput{
									{
										ValueFrom: v1alpha2.DataInputValueFrom{
											DataOutputName: "comp-trait",
										},
										ToFieldPaths: []string{"spec.key"},
									},
								},
							},
						},
					},
				},
			},
		}
		logf.Log.Info("Creating application config", "Name", appConfig.Name, "Namespace", appConfig.Namespace)
		Expect(k8sClient.Create(ctx, &appConfig)).Should(BeNil())
		verify(appConfigName, outName, inName)
	})
})
