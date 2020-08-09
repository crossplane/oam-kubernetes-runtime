package controllers_test

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Versioning mechanism of components", func() {
	ctx := context.Background()
	namespace := "component-versioning-test"
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	componentName := "example-component"

	// to identify different revisions of components
	imageV1 := "wordpress:4.6.1-apache"
	imageV2 := "wordpress:4.6.2-apache"

	var cwV1, cwV2 v1alpha2.ContainerizedWorkload
	var componentV1 v1alpha2.Component
	var appConfig v1alpha2.ApplicationConfiguration

	BeforeEach(func() {
		cwV1 = v1alpha2.ContainerizedWorkload{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ContainerizedWorkload",
				APIVersion: "core.oam.dev/v1alpha2",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
			},
			Spec: v1alpha2.ContainerizedWorkloadSpec{
				Containers: []v1alpha2.Container{
					{
						Name:  "wordpress",
						Image: imageV1,
						Ports: []v1alpha2.ContainerPort{
							{
								Name: "wordpress",
								Port: 80,
							},
						},
					},
				},
			},
		}

		cwV2 = v1alpha2.ContainerizedWorkload{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ContainerizedWorkload",
				APIVersion: "core.oam.dev/v1alpha2",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
			},
			Spec: v1alpha2.ContainerizedWorkloadSpec{
				Containers: []v1alpha2.Container{
					{
						Name:  "wordpress",
						Image: imageV2,
						Ports: []v1alpha2.ContainerPort{
							{
								Name: "wordpress",
								Port: 80,
							},
						},
					},
				},
			},
		}

		componentV1 = v1alpha2.Component{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "core.oam.dev/v1alpha2",
				Kind:       "Component",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: &cwV1,
				},
			},
		}

		appConfig = v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-appconfig",
				Namespace: namespace,
			},
		}

		logf.Log.Info("Start to run a test, clean up previous resources")
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
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
			time.Second*60, time.Millisecond*500).Should(&util.NotFoundMatcher{})
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

	When("create or update a component", func() {
		It("should create corresponding ControllerRevision", func() {
			By("Create Component v1")
			Expect(k8sClient.Create(ctx, &componentV1)).Should(Succeed())

			cmpV1 := &v1alpha2.Component{}
			By("Get Component v1")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cmpV1)).Should(Succeed())

			By("Get Component latest status after ControllerRevision created")
			Eventually(
				func() *v1alpha2.Revision {
					k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cmpV1)
					return cmpV1.Status.LatestRevision
				},
				time.Second*15, time.Millisecond*500).ShouldNot(BeNil())

			revisionNameV1 := cmpV1.Status.LatestRevision.Name
			By("Get corresponding ControllerRevision of Component v1")
			cr := &appsv1.ControllerRevision{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revisionNameV1}, cr)).Should(Succeed())
			By("Check revision seq number")
			Expect(cr.Revision).Should(Equal(int64(1)))

			cwV2raw, _ := json.Marshal(cwV2)
			cmpV1.Spec.Workload.Raw = cwV2raw
			By("Update Component into revision v2")
			Expect(k8sClient.Update(ctx, cmpV1)).Should(Succeed())

			cmpV2 := &v1alpha2.Component{}
			By("Get Component v2")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cmpV2)).Should(Succeed())

			By("Get Component latest status after ControllerRevision created")
			Eventually(
				func() string {
					k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cmpV2)
					return cmpV2.Status.LatestRevision.Name
				},
				time.Second*15, time.Millisecond*500).ShouldNot(Equal(revisionNameV1))

			revisionNameV2 := cmpV2.Status.LatestRevision.Name
			crV2 := &appsv1.ControllerRevision{}
			By("Get corresponding ControllerRevision of Component v2")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revisionNameV2}, crV2)).Should(Succeed())
			By("Check revision seq number")
			Expect(crV2.Revision).Should(Equal(int64(2)))

		})
	})

	When("Components have revisionName in AppConfig", func() {
		It("should NOT create NOR update workloads, when update components", func() {
			By("Create Component v1")
			Expect(k8sClient.Create(ctx, &componentV1)).Should(Succeed())

			cmpV1 := &v1alpha2.Component{}
			By("Get Component v1")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cmpV1)).Should(Succeed())

			By("Get Component latest status after ControllerRevision created")
			Eventually(
				func() *v1alpha2.Revision {
					k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cmpV1)
					return cmpV1.Status.LatestRevision
				},
				time.Second*15, time.Millisecond*500).ShouldNot(BeNil())

			revisionNameV1 := cmpV1.Status.LatestRevision.Name

			appConfigWithRevisionName := appConfig
			appConfigWithRevisionName.Spec.Components = append(appConfigWithRevisionName.Spec.Components,
				v1alpha2.ApplicationConfigurationComponent{
					RevisionName: revisionNameV1,
				})
			By("Apply appConfig")
			Expect(k8sClient.Create(ctx, &appConfigWithRevisionName)).Should(Succeed())

			cwWlV1 := v1alpha2.ContainerizedWorkload{}
			By("Check ContainerizedWorkload workload's image field is v1")
			Eventually(
				func() error {
					return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revisionNameV1}, &cwWlV1)
				},
				time.Second*15, time.Millisecond*500).Should(BeNil())
			Expect(cwWlV1.Spec.Containers[0].Image).Should(Equal(imageV1))

			cwV2raw, _ := json.Marshal(cwV2)
			cmpV1.Spec.Workload.Raw = cwV2raw
			By("Update Component to revision v2")
			Expect(k8sClient.Update(ctx, cmpV1)).Should(Succeed())

			By("Check ContainerizedWorkload workload's image field is still v1")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revisionNameV1}, &cwWlV1)).Should(Succeed())
			Expect(cwWlV1.Spec.Containers[0].Image).Should(Equal(imageV1))

			By("Check no workloads newly created")
			wlList := &unstructured.UnstructuredList{}
			wlList.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "core.oam.dev",
				Kind:    "ContainerizedWorkloadList",
				Version: "v1alpha2",
			})
			Expect(k8sClient.List(ctx, wlList)).Should(Succeed())
			Expect(len(wlList.Items)).Should(Equal(1))
		})

		It("should allow multiple revisions of one component exist at the same time", func() {
			By("Create Component v1")
			Expect(k8sClient.Create(ctx, &componentV1)).Should(Succeed())

			comp := &v1alpha2.Component{}
			By("Get Component v1")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, comp)).Should(Succeed())

			By("Get latest revision: revision 1")
			Eventually(
				func() *v1alpha2.Revision {
					k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, comp)
					return comp.Status.LatestRevision
				},
				time.Second*15, time.Millisecond*500).ShouldNot(BeNil())

			revisionNameV1 := comp.Status.LatestRevision.Name

			cwV2raw, _ := json.Marshal(cwV2)
			comp.Spec.Workload.Raw = cwV2raw
			By("Update Component to revision v2")
			Expect(k8sClient.Update(ctx, comp)).Should(Succeed())

			compV2 := &v1alpha2.Component{}
			By("Get latest Component revision: revision 2")
			Eventually(
				func() string {
					k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, compV2)
					return compV2.Status.LatestRevision.Name
				},
				time.Second*15, time.Millisecond*500).ShouldNot(Equal(revisionNameV1))

			revisionNameV2 := compV2.Status.LatestRevision.Name

			appConfigMultiRevision := appConfig
			appConfigMultiRevision.Spec.Components = []v1alpha2.ApplicationConfigurationComponent{
				{RevisionName: revisionNameV1}, {RevisionName: revisionNameV2},
			}
			By("Apply appConfig")
			Expect(k8sClient.Create(ctx, &appConfigMultiRevision)).Should(Succeed())

			By("Check two workload instances corresponding to two revisions")
			wlList := &v1alpha2.ContainerizedWorkloadList{}
			Eventually(func() int {
				k8sClient.List(ctx, wlList)
				return len(wlList.Items)
			}, time.Second*30, time.Millisecond*500).Should(Equal(2))
			cwWlV1 := v1alpha2.ContainerizedWorkload{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revisionNameV1}, &cwWlV1)).Should(BeNil())
			cwWlV2 := v1alpha2.ContainerizedWorkload{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revisionNameV2}, &cwWlV2)).Should(BeNil())
		})
	})

	When("Components have componentName", func() {
		It("should update workloads with new revision of components, when update components", func() {
			By("Create Component v1")
			Expect(k8sClient.Create(ctx, &componentV1)).Should(Succeed())

			cmpV1 := &v1alpha2.Component{}
			By("Get Component latest status after ControllerRevision created")
			Eventually(
				func() *v1alpha2.Revision {
					k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cmpV1)
					return cmpV1.Status.LatestRevision
				},
				time.Second*15, time.Millisecond*500).ShouldNot(BeNil())

			revisionNameV1 := cmpV1.Status.LatestRevision.Name

			appConfigWithRevisionName := appConfig
			appConfigWithRevisionName.Spec.Components = append(appConfigWithRevisionName.Spec.Components,
				v1alpha2.ApplicationConfigurationComponent{
					ComponentName: componentName,
				})
			By("Apply appConfig")
			Expect(k8sClient.Create(ctx, &appConfigWithRevisionName)).Should(Succeed())

			cwWlV1 := &v1alpha2.ContainerizedWorkload{}
			By("Check ContainerizedWorkload workload's image field is v1")
			Eventually(
				func() error {
					return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cwWlV1)
				},
				time.Second*15, time.Millisecond*500).Should(BeNil())
			Expect(cwWlV1.Spec.Containers[0].Image).Should(Equal(imageV1))

			cwV2raw, _ := json.Marshal(cwV2)
			cmpV1.Spec.Workload.Raw = cwV2raw
			By("Update Component to revision v2")
			Expect(k8sClient.Update(ctx, cmpV1)).Should(Succeed())

			By("Check Component has been changed to revision v2")
			By("Get latest Component revision: revision 2")
			cmpV2 := &v1alpha2.Component{}
			Eventually(
				func() string {
					k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cmpV2)
					return cmpV2.Status.LatestRevision.Name
				},
				time.Second*30, time.Millisecond*500).ShouldNot(Equal(revisionNameV1))

			By("Check ContainerizedWorkload workload's image field has been changed to v2")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cwWlV1)).Should(Succeed())
			Expect(cwWlV1.Spec.Containers[0].Image).Should(Equal(imageV2))
		})
	})

	//TODO(roywang) Components have componentName and have revision-enabled trait
	//TODO(roywang) Components have componentName and have no revision-enabled trait
})
