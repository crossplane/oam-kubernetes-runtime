package e2e_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	oamv1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

var _ = Describe("Test OMA Kubernetes runtime", func() {
	ctx := context.Background()
	namespace := "controller-test"
	lablel := map[string]string{"app": "test"}
	trueVar := true
	falseVar := false
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   namespace,
			Labels: lablel,
		},
	}
	BeforeEach(func() {
		logf.Log.Info("Start to run a test, clean up previous resources")
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).
			Should(SatisfyAny(BeNil(), &NotFoundMatcher{}))
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
			time.Second*30, time.Millisecond*500).Should(&NotFoundMatcher{})
		// recreate the name space
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &AlreadyExistMatcher{}))

	})
	AfterEach(func() {
		logf.Log.Info("Clean up resources")
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(BeNil())
	})

	It("apply an application config", func() {
		// create a workload definition
		wd := oamv1alpha2.WorkloadDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "containerizedworkloads.core.oam.dev",
				Labels: lablel,
			},
			Spec: oamv1alpha2.WorkloadDefinitionSpec{
				Reference: oamv1alpha2.DefinitionReference{
					Name: "containerizedworkloads.core.oam.dev",
				},
			},
		}
		logf.Log.Info("Creating workload definition")
		// For some reason, WorkloadDefinition is created as a Cluster scope object
		Expect(k8sClient.Create(ctx, &wd)).Should(SatisfyAny(BeNil(), &AlreadyExistMatcher{}))
		// create a workload CR
		wl := oamv1alpha2.ContainerizedWorkload{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Labels:    lablel,
			},
			Spec: oamv1alpha2.ContainerizedWorkloadSpec{
				Containers: []oamv1alpha2.Container{
					{
						Name:  "wordpress",
						Image: "wordpress:4.6.1-apache",
						Ports: []oamv1alpha2.ContainerPort{
							{
								Name: "wordpress",
								Port: 80,
							},
						},
					},
				},
			},
		}
		// reflect workload gvk from scheme
		gvks, _, _ := scheme.ObjectKinds(&wl)
		wl.APIVersion = gvks[0].GroupVersion().String()
		wl.Kind = gvks[0].Kind
		// Create a component definition
		comp := oamv1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-component",
				Namespace: namespace,
				Labels:    lablel,
			},
			Spec: oamv1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: &wl,
				},
				Parameters: []oamv1alpha2.ComponentParameter{
					{
						Name:       "instance-name",
						Required:   &trueVar,
						FieldPaths: []string{"metadata.name"},
					},
					{
						Name:       "image",
						Required:   &falseVar,
						FieldPaths: []string{"spec.containers[0].image"},
					},
				},
			},
		}
		logf.Log.Info("Creating component", "Name", comp.Name, "Namespace", comp.Namespace)
		Expect(k8sClient.Create(ctx, &comp)).Should(BeNil())
		// Create manual scaler trait definition
		mt := oamv1alpha2.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "manualscalertraits.core.oam.dev",
				Labels: lablel,
			},
			Spec: oamv1alpha2.TraitDefinitionSpec{
				Reference: oamv1alpha2.DefinitionReference{
					Name: "manualscalertraits.core.oam.dev",
				},
			},
		}
		logf.Log.Info("Creating trait definition")
		// For some reason, traitDefinition is created as a Cluster scope object
		Expect(k8sClient.Create(ctx, &mt)).Should(SatisfyAny(BeNil(), &AlreadyExistMatcher{}))
		// Create a manualscaler trait CR
		var replica int32 = 5
		traitInstanceName := "example-appconfig-trait"
		mts := oamv1alpha2.ManualScalerTrait{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      traitInstanceName,
				Labels:    lablel,
			},
			Spec: oamv1alpha2.ManualScalerTraitSpec{
				ReplicaCount: replica,
			},
		}
		// reflect workload gvk from scheme
		gvks, _, _ = scheme.ObjectKinds(&mts)
		mts.APIVersion = gvks[0].GroupVersion().String()
		mts.Kind = gvks[0].Kind
		// Create application configuration
		workloadInstanceName := "example-appconfig-workload"
		imageName := "wordpress:php7.2"
		appConfig := oamv1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-appconfig",
				Namespace: namespace,
				Labels:    lablel,
			},
			Spec: oamv1alpha2.ApplicationConfigurationSpec{
				Components: []oamv1alpha2.ApplicationConfigurationComponent{
					{
						ComponentName: "example-component",
						ParameterValues: []oamv1alpha2.ComponentParameterValue{
							{
								Name:  "instance-name",
								Value: intstr.IntOrString{StrVal: workloadInstanceName, Type: intstr.String},
							},
							{
								Name:  "image",
								Value: intstr.IntOrString{StrVal: imageName, Type: intstr.String},
							},
						},
						Traits: []oamv1alpha2.ComponentTrait{
							{
								Trait: runtime.RawExtension{
									Object: &mts,
								},
							},
						},
					},
				},
			},
		}
		logf.Log.Info("Creating application config", "Name", appConfig.Name, "Namespace", appConfig.Namespace)
		Expect(k8sClient.Create(ctx, &appConfig)).Should(BeNil())
		// Verification
		By("Checking containerized workload is created")
		objectKey := client.ObjectKey{
			Name:      workloadInstanceName,
			Namespace: namespace,
		}
		workload := &oamv1alpha2.ContainerizedWorkload{}
		logf.Log.Info("Checking on containerized workload", "Key", objectKey)
		Eventually(
			func() error {
				return k8sClient.Get(ctx, objectKey, workload)
			},
			time.Second*15, time.Millisecond*500).Should(BeNil())

		By("Verify that the parameter substitute works")
		Expect(workload.Spec.Containers[0].Image).Should(Equal(imageName))

		// Verification
		By("Checking Manual scalar trait is created")
		objectKey.Name = traitInstanceName
		trait := &oamv1alpha2.ManualScalerTrait{}
		logf.Log.Info("Checking on manual scalar trait", "Key", objectKey)
		Eventually(
			func() error {
				return k8sClient.Get(ctx, objectKey, trait)
			},
			time.Second*15, time.Millisecond*500).Should(BeNil())

		By("Verify deployment scaled according to the manualScaler trait")
		Expect(trait.Spec.ReplicaCount).Should(BeEquivalentTo(replica))
	})
})
