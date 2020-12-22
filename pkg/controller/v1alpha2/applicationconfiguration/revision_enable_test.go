/*
Copyright 2020 The Crossplane Authors.

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

package applicationconfiguration

import (
	"context"
	"fmt"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

var _ = Describe("Test ApplicationConfiguration Component Revision Enabled trait", func() {
	const (
		namespace = "revision-enable-test"
		appName   = "revision-test-app"
		compName  = "revision-test-comp"
	)
	var (
		ctx          = context.Background()
		wr           v1.Deployment
		component    v1alpha2.Component
		appConfig    v1alpha2.ApplicationConfiguration
		appConfigKey = client.ObjectKey{
			Name:      appName,
			Namespace: namespace,
		}
		req = reconcile.Request{NamespacedName: appConfigKey}
		ns  = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
	)

	BeforeEach(func() {})

	AfterEach(func() {
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).
			Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
	})

	It("revision enabled should create workload with revisionName and work upgrade with new revision successfully", func() {

		getDeploy := func(image string) *v1.Deployment {
			return &v1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
				},
				Spec: v1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
						"app": compName,
					}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
							"app": compName,
						}},
						Spec: corev1.PodSpec{Containers: []corev1.Container{{
							Name:  "wordpress",
							Image: image,
							Ports: []corev1.ContainerPort{
								{
									Name:          "wordpress",
									ContainerPort: 80,
								},
							},
						},
						}}},
				},
			}
		}
		component = v1alpha2.Component{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "core.oam.dev/v1alpha2",
				Kind:       "Component",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      compName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: getDeploy("wordpress:4.6.1-apache"),
				},
			},
		}
		appConfig = v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
		}

		By("Create namespace")
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("Create Component")
		Expect(k8sClient.Create(ctx, &component)).Should(Succeed())
		cmpV1 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compName}, cmpV1)).Should(Succeed())

		By("component handler will automatically create controller revision")
		Expect(componentHandler.createControllerRevision(cmpV1, cmpV1)).Should(BeTrue())
		var crList v1.ControllerRevisionList
		By("Check controller revision created successfully")
		Eventually(func() error {
			labels := &metav1.LabelSelector{
				MatchLabels: map[string]string{
					ControllerRevisionComponentLabel: compName,
				},
			}
			selector, err := metav1.LabelSelectorAsSelector(labels)
			if err != nil {
				return err
			}
			err = k8sClient.List(ctx, &crList, &client.ListOptions{
				LabelSelector: selector,
			})
			if err != nil {
				return err
			}
			if len(crList.Items) != 1 {
				return fmt.Errorf("want only 1 revision created but got %d", len(crList.Items))
			}
			return nil
		}, time.Second, 300*time.Millisecond).Should(BeNil())

		By("Create an ApplicationConfiguration")
		appConfig = v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{Components: []v1alpha2.ApplicationConfigurationComponent{
				{
					ComponentName: compName,
					RevisionName:  compName + "-v1",
					Traits: []v1alpha2.ComponentTrait{
						{
							Trait: runtime.RawExtension{Object: &unstructured.Unstructured{Object: map[string]interface{}{
								"apiVersion": "example.com/v1",
								"kind":       "Foo",
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										"trait.oam.dev/type": "rollout-revision",
									},
								},
								"spec": map[string]interface{}{
									"key": "test1",
								},
							}}},
						},
					},
				},
			}},
		}
		By("Creat appConfig & check successfully")
		Expect(k8sClient.Create(ctx, &appConfig)).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, appConfigKey, &appConfig)
		}, time.Second, 300*time.Millisecond).Should(BeNil())

		By("Reconcile")
		reconcileRetry(reconciler, req)

		By("Check workload created successfully")
		Eventually(func() error {
			var workloadKey = client.ObjectKey{Namespace: namespace, Name: compName + "-v1"}
			return k8sClient.Get(ctx, workloadKey, &wr)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		By("Check workload should only have 1 generation")
		Expect(wr.GetGeneration()).Should(BeEquivalentTo(1))

		By("Check reconcile again and no error will happen")
		reconcileRetry(reconciler, req)
		By("Check appconfig condition should not have error")
		Eventually(func() string {
			By("Reconcile again and should not have error")
			reconcileRetry(reconciler, req)
			err := k8sClient.Get(ctx, appConfigKey, &appConfig)
			if err != nil {
				return err.Error()
			}
			if len(appConfig.Status.Conditions) != 1 {
				return "condition len should be 1 but now is " + strconv.Itoa(len(appConfig.Status.Conditions))
			}
			return string(appConfig.Status.Conditions[0].Reason)
		}, 3*time.Second, 300*time.Millisecond).Should(BeEquivalentTo("ReconcileSuccess"))

		By("Check workload will not update when reconcile again but no appconfig changed")
		Eventually(func() error {
			var workloadKey = client.ObjectKey{Namespace: namespace, Name: compName + "-v1"}
			return k8sClient.Get(ctx, workloadKey, &wr)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		By("Check workload should should still be 1 generation")
		Expect(wr.GetGeneration()).Should(BeEquivalentTo(1))
		By("Check trait was created as expected")
		var tr unstructured.Unstructured
		Eventually(func() error {
			tr.SetAPIVersion("example.com/v1")
			tr.SetKind("Foo")
			var traitKey = client.ObjectKey{Namespace: namespace, Name: appConfig.Status.Workloads[0].Traits[0].Reference.Name}
			return k8sClient.Get(ctx, traitKey, &tr)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		Expect(tr.Object["spec"]).Should(BeEquivalentTo(map[string]interface{}{"key": "test1"}))

		By("===================================== Start to Update =========================================")
		cmpV2 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: compName}, cmpV2)).Should(Succeed())
		cmpV2.Spec.Workload = runtime.RawExtension{
			Object: getDeploy("wordpress:v2"),
		}
		By("Update Component")
		Expect(k8sClient.Update(ctx, cmpV2)).Should(Succeed())
		By("component handler will automatically create a ne controller revision")
		Expect(componentHandler.createControllerRevision(cmpV2, cmpV2)).Should(BeTrue())
		By("Check controller revision created successfully")
		Eventually(func() error {
			labels := &metav1.LabelSelector{
				MatchLabels: map[string]string{
					ControllerRevisionComponentLabel: compName,
				},
			}
			selector, err := metav1.LabelSelectorAsSelector(labels)
			if err != nil {
				return err
			}
			err = k8sClient.List(ctx, &crList, &client.ListOptions{
				LabelSelector: selector,
			})
			if err != nil {
				return err
			}
			if len(crList.Items) != 2 {
				return fmt.Errorf("there should be exactly 2 revision created but got %d", len(crList.Items))
			}
			return nil
		}, time.Second, 300*time.Millisecond).Should(BeNil())

		By("Update appConfig & check successfully")
		appConfig.Spec.Components[0].RevisionName = compName + "-v2"
		appConfig.Spec.Components[0].Traits[0].Trait = runtime.RawExtension{Object: &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "example.com/v1",
			"kind":       "Foo",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"trait.oam.dev/type": "rollout-revision",
				},
			},
			"spec": map[string]interface{}{
				"key": "test2",
			},
		}}}
		Expect(k8sClient.Update(ctx, &appConfig)).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, appConfigKey, &appConfig)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		By("Reconcile for new revision")
		reconcileRetry(reconciler, req)

		By("Check new revision workload created successfully")
		Eventually(func() error {
			var workloadKey = client.ObjectKey{Namespace: namespace, Name: compName + "-v2"}
			return k8sClient.Get(ctx, workloadKey, &wr)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		By("Check the new workload should only have 1 generation")
		Expect(wr.GetGeneration()).Should(BeEquivalentTo(1))
		Expect(wr.Spec.Template.Spec.Containers[0].Image).Should(BeEquivalentTo("wordpress:v2"))

		By("Check the old workload is still there with no change")
		Eventually(func() error {
			var workloadKey = client.ObjectKey{Namespace: namespace, Name: compName + "-v1"}
			return k8sClient.Get(ctx, workloadKey, &wr)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		By("Check the new workload should only have 1 generation")
		Expect(wr.GetGeneration()).Should(BeEquivalentTo(1))

		By("Check reconcile again and no error will happen")
		reconcileRetry(reconciler, req)
		By("Check appconfig condition should not have error")
		Eventually(func() string {
			By("Once more Reconcile and should not have error")
			reconcileRetry(reconciler, req)
			err := k8sClient.Get(ctx, appConfigKey, &appConfig)
			if err != nil {
				return err.Error()
			}
			if len(appConfig.Status.Conditions) != 1 {
				return "condition len should be 1 but now is " + strconv.Itoa(len(appConfig.Status.Conditions))
			}
			return string(appConfig.Status.Conditions[0].Reason)
		}, 3*time.Second, 300*time.Millisecond).Should(BeEquivalentTo("ReconcileSuccess"))
		By("Check trait was updated as expected")
		Eventually(func() error {
			tr.SetAPIVersion("example.com/v1")
			tr.SetKind("Foo")
			var traitKey = client.ObjectKey{Namespace: namespace, Name: appConfig.Status.Workloads[0].Traits[0].Reference.Name}
			return k8sClient.Get(ctx, traitKey, &tr)
		}, time.Second, 300*time.Millisecond).Should(BeNil())
		Expect(tr.Object["spec"]).Should(BeEquivalentTo(map[string]interface{}{"key": "test2"}))
	})

})
