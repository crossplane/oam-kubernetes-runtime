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
	outName := "data-output"
	out := tempFoo.DeepCopy()
	out.SetName(outName)
	inName := "data-input"
	in := tempFoo.DeepCopy()
	in.SetName(inName)
	componentOutName := "component-out"
	componentInName := "component-in"
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
		// recreate the namespace for testing
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
		// Create a component for data outputs
		compOut := v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentOutName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: out,
				},
			},
		}
		Expect(k8sClient.Create(ctx, &compOut)).Should(BeNil())
		logf.Log.Info("Creating component that outputs data", "Name", compOut.Name, "Namespace", compOut.Namespace)
		// Create a component for data inputs
		compIn := v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentInName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: in,
				},
			},
		}
		Expect(k8sClient.Create(ctx, &compIn)).Should(BeNil())
		logf.Log.Info("Creating component that inputs data", "Name", compIn.Name, "Namespace", compIn.Namespace)
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
	verify := func(appConfigName, reason string) {
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
			time.Second*60, time.Second*2).Should(&util.NotFoundMatcher{})
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
			time.Second*60, time.Second*2).Should(BeNil())
		By("Verify the appconfig's dependency is unsatisfied")
		appconfigKey := client.ObjectKey{
			Name:      appConfigName,
			Namespace: namespace,
		}
		appconfig := &v1alpha2.ApplicationConfiguration{}
		depStatus := v1alpha2.DependencyStatus{
			Unsatisfied: []v1alpha2.UnstaifiedDependency{
				{
					Reason: reason,
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
			time.Second*60, time.Second*2).Should(Equal(depStatus))
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
			time.Second*80, time.Second*2).Should(BeNil())
		By("Verify the appconfig's dependency is satisfied")
		appconfig = &v1alpha2.ApplicationConfiguration{}
		logf.Log.Info("Checking on appconfig", "Key", appconfigKey)
		Eventually(
			func() []v1alpha2.UnstaifiedDependency {
				k8sClient.Get(ctx, appconfigKey, appconfig)
				return appconfig.Status.Dependency.Unsatisfied
			},
			time.Second*180, time.Second*5).Should(BeNil())
	}

	It("trait depends on another trait", func() {
		label := map[string]string{"trait": "trait"}
		// Define a workload
		wl := tempFoo.DeepCopy()
		wl.SetName("workload")
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
									Object: out,
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
									Object: in,
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
		verify(appConfigName, "status.key not found in object")
	})

	It("component depends on another component", func() {
		label := map[string]string{"component": "component"}
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
		verify(appConfigName, "status.key not found in object")
	})

	It("component depends on trait", func() {
		label := map[string]string{"trait": "component"}
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
									Object: out,
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
		verify(appConfigName, "status.key not found in object")
	})

	It("trait depends on component", func() {
		label := map[string]string{"component": "trait"}
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
									Object: in,
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
		verify(appConfigName, "status.key not found in object")
	})

	It("component depends on trait with updated condition", func() {
		label := map[string]string{"trait": "component", "app-hash": "hash-v1"}
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
									Object: out,
								},
								DataOutputs: []v1alpha2.DataOutput{
									{
										Name:      "trait-comp",
										FieldPath: "status.key",
										Conditions: []v1alpha2.ConditionRequirement{
											{
												Operator:  v1alpha2.ConditionEqual,
												ValueFrom: v1alpha2.ValueFrom{FieldPath: "metadata.labels.app-hash"},
												FieldPath: "status.app-hash",
											},
										},
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
			time.Second*60, time.Second*2).Should(&util.NotFoundMatcher{})

		By("Checking that resource which provides data is created")
		// Originally the trait has value in `status.key`, but the hash label is old
		outFooKey := client.ObjectKey{
			Name:      outName,
			Namespace: namespace,
		}
		outFoo := tempFoo.DeepCopy()
		Eventually(
			func() error {
				return k8sClient.Get(ctx, outFooKey, outFoo)
			},
			time.Second*60, time.Second*2).Should(BeNil())
		err := unstructured.SetNestedField(outFoo.Object, "test", "status", "key")
		Expect(err).Should(BeNil())
		err = unstructured.SetNestedField(outFoo.Object, "hash-v1", "status", "app-hash")
		Expect(err).Should(BeNil())
		Expect(k8sClient.Update(ctx, outFoo)).Should(Succeed())

		appconfigKey := client.ObjectKey{
			Name:      appConfigName,
			Namespace: namespace,
		}
		newAppConfig := &v1alpha2.ApplicationConfiguration{}

		// Verification after satisfying dependency
		By("Verify the appconfig's dependency is satisfied")
		newAppConfig = &v1alpha2.ApplicationConfiguration{}
		logf.Log.Info("Checking on appconfig", "Key", appconfigKey)
		Eventually(
			func() []v1alpha2.UnstaifiedDependency {
				var tempApp = &v1alpha2.ApplicationConfiguration{}
				k8sClient.Get(ctx, appconfigKey, tempApp)
				tempApp.DeepCopyInto(newAppConfig)
				return tempApp.Status.Dependency.Unsatisfied
			},
			time.Second*80, time.Second*2).Should(BeNil())
		By("Checking that resource which accepts data is created now")
		logf.Log.Info("Checking on resource that inputs data", "Key", inFooKey)
		Eventually(
			func() error {
				return k8sClient.Get(ctx, inFooKey, inFoo)
			},
			time.Second*80, time.Second*2).Should(BeNil())

		newAppConfig.Labels["app-hash"] = "hash-v2"
		Expect(k8sClient.Update(ctx, newAppConfig)).Should(BeNil())

		By("Verify the appconfig's dependency should be unsatisfied, because requirementCondition valueFrom not match")

		depStatus := v1alpha2.DependencyStatus{
			Unsatisfied: []v1alpha2.UnstaifiedDependency{
				{
					Reason: "got(hash-v1) expected to be hash-v2",
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
				k8sClient.Get(ctx, appconfigKey, newAppConfig)
				return newAppConfig.Status.Dependency
			},
			time.Second*60, time.Second*2).Should(Equal(depStatus))

		// Update trait object
		k8sClient.Get(ctx, outFooKey, outFoo) // Get the latest before update
		err = unstructured.SetNestedField(outFoo.Object, "test-new", "status", "key")
		Expect(err).Should(BeNil())
		err = unstructured.SetNestedField(outFoo.Object, "hash-v2", "status", "app-hash")
		Expect(err).Should(BeNil())
		Expect(k8sClient.Update(ctx, outFoo)).Should(Succeed())

		// Verification after satisfying dependency
		By("Checking that resource which accepts data is updated now")
		logf.Log.Info("Checking on resource that inputs data", "Key", inFooKey)
		Eventually(
			func() string {
				k8sClient.Get(ctx, inFooKey, inFoo)
				outdata, _, _ := unstructured.NestedString(inFoo.Object, "spec", "key")
				return outdata
			},
			time.Second*80, time.Second*2).Should(Equal("test-new"))
		By("Verify the appconfig's dependency is satisfied")
		logf.Log.Info("Checking on appconfig", "Key", appconfigKey)
		Eventually(
			func() []v1alpha2.UnstaifiedDependency {
				tempAppConfig := &v1alpha2.ApplicationConfiguration{}
				k8sClient.Get(ctx, appconfigKey, tempAppConfig)
				return tempAppConfig.Status.Dependency.Unsatisfied
			},
			time.Second*80, time.Second*2).Should(BeNil())
	})
})
