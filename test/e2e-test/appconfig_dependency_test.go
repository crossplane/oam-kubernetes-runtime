package controllers_test

import (
	"context"
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"time"
)

var _ = Describe("AppconfigDependency", func() {
	ctx := context.Background()
	namespace := "appconfig-dependency-test"
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	BeforeEach(func() {
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
	})
	AfterEach(func() {
		logf.Log.Info("Clean up resources")
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(BeNil())
	})

	It("trait depends on another trait", func() {
		label := map[string]string{"trait": "trait"}
		// create a trait definition
		tdOut := v1alpha2.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "services",
				Labels: label,
			},
			Spec: v1alpha2.TraitDefinitionSpec{
				AppliesToWorkloads: []string{
					"statefulsets.apps",
				},
				Reference: v1alpha2.DefinitionReference{
					Name: "services",
				},
			},
		}
		logf.Log.Info("Creating trait definition")
		Expect(k8sClient.Create(ctx, &tdOut)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		// Create a service trait CR
		outName := "trait-trait-out"
		svc := corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      outName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: corev1.ServiceSpec{
				Selector: label,
				Ports: []corev1.ServicePort{
					{
						Port:       80,
						TargetPort: intstr.FromInt(80),
						Name:       "web",
						Protocol:   corev1.ProtocolTCP,
					},
				},
				Type: corev1.ServiceTypeLoadBalancer,
			},
		}
		// reflect trait gvk from scheme
		gvks, _, _ := scheme.ObjectKinds(&svc)
		svc.APIVersion = gvks[0].GroupVersion().String()
		svc.Kind = gvks[0].Kind
		// create another trait definition
		tdIn := v1alpha2.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "ingresses.networking.k8s.io",
				Labels: label,
			},
			Spec: v1alpha2.TraitDefinitionSpec{
				AppliesToWorkloads: []string{
					"statefulsets.apps",
				},
				Reference: v1alpha2.DefinitionReference{
					Name: "ingresses.networking.k8s.io",
				},
			},
		}
		logf.Log.Info("Creating another trait definition")
		Expect(k8sClient.Create(ctx, &tdIn)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		// Create a ingress trait CR
		inName := "trait-trait-in"
		ingress := v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      inName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{
					{
						Host: "depend.oam.com",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{
									{
										Path: "/",
										Backend: v1beta1.IngressBackend{
											ServicePort: intstr.FromInt(80),
											ServiceName: outName,
										},
									},
								},
							},
						},
					},
				},
			},
		}
		// reflect trait gvk from scheme
		gvks, _, _ = scheme.ObjectKinds(&ingress)
		ingress.APIVersion = gvks[0].GroupVersion().String()
		ingress.Kind = gvks[0].Kind
		// create a workload definition
		wd := v1alpha2.WorkloadDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "statefulsets.apps",
				Labels: label,
			},
			Spec: v1alpha2.WorkloadDefinitionSpec{
				Reference: v1alpha2.DefinitionReference{
					Name: "statefulsets.apps",
				},
			},
		}
		logf.Log.Info("Creating workload definition")
		// For some reason, WorkloadDefinition is created as a Cluster scope object
		Expect(k8sClient.Create(ctx, &wd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		stsName := "trait-trait-wl"
		wl := appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      stsName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: appsv1.StatefulSetSpec{
				ServiceName: outName,
				Selector: &metav1.LabelSelector{
					MatchLabels: label,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: label,
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "nginx",
								Image: "nginx:1.17",
								Ports: []corev1.ContainerPort{
									{
										ContainerPort: 80,
									},
								},
							},
						},
					},
				},
			},
		}
		// reflect workload gvk from scheme
		gvks, _, _ = scheme.ObjectKinds(&wl)
		wl.APIVersion = gvks[0].GroupVersion().String()
		wl.Kind = gvks[0].Kind
		// Create a component definition
		componentName := "component-trait-trait"
		comp := v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: &wl,
				},
			},
		}
		logf.Log.Info("Creating component", "Name", comp.Name, "Namespace", comp.Namespace)
		Expect(k8sClient.Create(ctx, &comp)).Should(BeNil())
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
									Object: &svc,
								},
								DataOutputs: []v1alpha2.DataOutput{
									{
										Name:      "test",
										FieldPath: "spec.externalTrafficPolicy",
										Conditions: []v1alpha2.ConditionRequirement{
											{
												Operator:  "eq",
												Value:     "Local",
												FieldPath: "spec.externalTrafficPolicy",
											},
										},
									},
								},
							},
							{
								Trait: runtime.RawExtension{
									Object: &ingress,
								},
								DataInputs: []v1alpha2.DataInput{
									{
										ValueFrom: v1alpha2.DataInputValueFrom{
											DataOutputName: "test",
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
		// Verification before satisfying dependency
		By("Checking that trait which accepts data isn't created yet")
		ingressKey := client.ObjectKey{
			Name:      inName,
			Namespace: namespace,
		}
		ingressIn := &v1beta1.Ingress{}
		logf.Log.Info("Checking on ingress that inputs data", "Key", ingressKey)
		Eventually(
			func() error {
				return k8sClient.Get(ctx, ingressKey, ingressIn)
			},
			time.Second*15, time.Millisecond*500).Should(&util.NotFoundMatcher{})
		By("Checking that trait which provides data is created")
		svcKey := client.ObjectKey{
			Name:      outName,
			Namespace: namespace,
		}
		svcOut := &corev1.Service{}
		logf.Log.Info("Checking on service that outputs data", "Key", svcKey)
		Eventually(
			func() error {
				return k8sClient.Get(ctx, svcKey, svcOut)
			},
			time.Second*15, time.Millisecond*500).Should(BeNil())
		By("Verify the appconfig's dependency is unsatisfied")
		appconfigKey := client.ObjectKey{
			Name:      appConfigName,
			Namespace: namespace,
		}
		appconfig := &v1alpha2.ApplicationConfiguration{}
		unsatisfiedStatus := []v1alpha2.UnstaifiedDependency{
			{
				From: v1alpha2.DependencyFromObject{
					TypedReference: runtimev1alpha1.TypedReference{
						APIVersion: corev1.SchemeGroupVersion.String(),
						Name:       outName,
						Kind:       "Service",
					},
					FieldPath: "spec.externalTrafficPolicy",
				},
				To: v1alpha2.DependencyToObject{
					TypedReference: runtimev1alpha1.TypedReference{
						APIVersion: v1beta1.SchemeGroupVersion.String(),
						Name:       inName,
						Kind:       "Ingress",
					},
				},
			},
		}
		logf.Log.Info("Checking on appconfig", "Key", appconfigKey)
		Eventually(
			func() error {
				return k8sClient.Get(ctx, appconfigKey, appconfig)
			},
			time.Second*15, time.Millisecond*500).Should(BeNil())
		Expect(appconfig.Status.Dependency.Unsatisfied).Should(Equal(unsatisfiedStatus))
		// fill value to fieldPath
		updated := svcOut
		updated.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeLocal
		Expect(k8sClient.Update(ctx, updated)).Should(Succeed())
		// Verification after satisfying dependency
		By("Checking that trait which accepts data is created now")
		logf.Log.Info("Checking on ingress that inputs data", "Key", ingressKey)
		Eventually(
			func() error {
				return k8sClient.Get(ctx, ingressKey, ingressIn)
			},
			time.Second*15, time.Millisecond*500).Should(BeNil())
		By("Verify the appconfig's dependency is satisfied")
		appconfig = &v1alpha2.ApplicationConfiguration{}
		logf.Log.Info("Checking on appconfig", "Key", appconfigKey)
		Eventually(
			func() error {
				return k8sClient.Get(ctx, appconfigKey, appconfig)
			},
			time.Second*15, time.Millisecond*500).Should(BeNil())
		Expect(appconfig.Status.Dependency.Unsatisfied).Should(BeNil())
	})
})
