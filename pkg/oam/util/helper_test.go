package util_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

var _ = Describe("Test workload related helper utils", func() {
	// Test common variables
	ctx := context.Background()
	namespace := "oamNS"
	workloadName := "oamWorkload"
	workloadKind := "ContainerizedWorkload"
	workloadAPIVersion := "core.oam.dev/v1"
	workloadDefinitionName := "containerizedworkloads.core.oam.dev"
	var workloadUID types.UID = "oamWorkloadUID"
	log := ctrl.Log.WithName("ManualScalarTraitReconciler")
	// workload CR
	workload := v1alpha2.ContainerizedWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workloadName,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: workloadAPIVersion,
			Kind:       workloadKind,
		},
	}
	workload.SetUID(workloadUID)
	unstructuredWorkload, _ := util.Object2Unstructured(workload)
	// workload Definition
	workloadDefinition := v1alpha2.WorkloadDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: workloadDefinitionName,
		},
		Spec: v1alpha2.WorkloadDefinitionSpec{
			Reference: v1alpha2.DefinitionReference{
				Name: workloadDefinitionName,
			},
		},
	}

	getErr := fmt.Errorf("get failed")

	BeforeEach(func() {
		logf.Log.Info("Set up resources before a unit test")
	})

	AfterEach(func() {
		logf.Log.Info("Clean up resources after a unit test")
	})

	It("Test fetch workloadDefinition", func() {
		type fields struct {
			getFunc test.ObjectFn
		}
		type want struct {
			wld *v1alpha2.WorkloadDefinition
			err error
		}

		cases := map[string]struct {
			fields fields
			want   want
		}{
			"FetchWorkloadDefinition fail when getWorkloadDefinition fails": {
				fields: fields{
					getFunc: func(obj runtime.Object) error {
						return getErr
					},
				},
				want: want{
					wld: nil,
					err: getErr,
				},
			},

			"FetchWorkloadDefinition Success": {
				fields: fields{
					getFunc: func(obj runtime.Object) error {
						o, _ := obj.(*v1alpha2.WorkloadDefinition)
						w := workloadDefinition
						*o = w
						return nil
					},
				},
				want: want{
					wld: &workloadDefinition,
					err: nil,
				},
			},
		}
		for name, tc := range cases {
			tclient := test.MockClient{
				MockGet: test.NewMockGetFn(nil, tc.fields.getFunc),
			}
			got, err := util.FetchWorkloadDefinition(ctx, &tclient, unstructuredWorkload)
			By(fmt.Sprint("Running test: ", name))
			Expect(tc.want.err).Should(util.BeEquivalentToError(err))
			Expect(tc.want.wld).Should(Equal(got))
		}
	})

	It("Test extract child resources from any workload", func() {
		crkl := []v1alpha2.ChildResourceKind{
			{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
			{
				Kind:       "Service",
				APIVersion: "v1",
			},
		}
		// cdResource is the child deployment owned by the workload
		cdResource := unstructured.Unstructured{}
		cdResource.SetOwnerReferences([]metav1.OwnerReference{
			{
				Kind: util.KindDeployment,
				UID:  workloadUID,
			},
		})
		// cdResource is the child service owned by the workload
		cSResource := unstructured.Unstructured{}
		cSResource.SetOwnerReferences([]metav1.OwnerReference{
			{
				Kind: util.KindService,
				UID:  workloadUID,
			},
		})
		// oResource is not owned by the workload
		oResource := unstructured.Unstructured{}
		oResource.SetOwnerReferences([]metav1.OwnerReference{
			{
				UID: "NotWorkloadUID",
			},
		})
		var nilListFunc test.ObjectFn = func(o runtime.Object) error {
			u := &unstructured.Unstructured{}
			l := o.(*unstructured.UnstructuredList)
			l.Items = []unstructured.Unstructured{*u}
			return nil
		}
		type fields struct {
			getFunc  test.ObjectFn
			listFunc test.ObjectFn
		}
		type want struct {
			crks []*unstructured.Unstructured
			err  error
		}

		cases := map[string]struct {
			fields fields
			want   want
		}{
			"FetchWorkloadChildResources fail when getWorkloadDefinition fails": {
				fields: fields{
					getFunc: func(obj runtime.Object) error {
						return getErr
					},
					listFunc: nilListFunc,
				},
				want: want{
					crks: nil,
					err:  getErr,
				},
			},
			"FetchWorkloadChildResources return nothing when the workloadDefinition doesn't have child list": {
				fields: fields{
					getFunc: func(obj runtime.Object) error {
						o, _ := obj.(*v1alpha2.WorkloadDefinition)
						*o = workloadDefinition
						return nil
					},
					listFunc: nilListFunc,
				},
				want: want{
					crks: nil,
					err:  nil,
				},
			},
			"FetchWorkloadChildResources Success": {
				fields: fields{
					getFunc: func(obj runtime.Object) error {
						o, _ := obj.(*v1alpha2.WorkloadDefinition)
						w := workloadDefinition
						w.Spec.ChildResourceKinds = crkl
						*o = w
						return nil
					},
					listFunc: func(o runtime.Object) error {
						l := o.(*unstructured.UnstructuredList)
						if l.GetKind() == util.KindDeployment {
							l.Items = append(l.Items, cdResource)
						} else if l.GetKind() == util.KindService {
							l.Items = append(l.Items, cSResource)
						} else {
							return getErr
						}
						return nil
					},
				},
				want: want{
					crks: []*unstructured.Unstructured{
						&cdResource, &cSResource,
					},
					err: nil,
				},
			},
			"FetchWorkloadChildResources with many resources only pick the child one": {
				fields: fields{
					getFunc: func(obj runtime.Object) error {
						o, _ := obj.(*v1alpha2.WorkloadDefinition)
						w := workloadDefinition
						w.Spec.ChildResourceKinds = crkl
						*o = w
						return nil
					},
					listFunc: func(o runtime.Object) error {
						l := o.(*unstructured.UnstructuredList)
						l.Items = []unstructured.Unstructured{oResource, oResource, oResource, oResource,
							oResource, oResource, oResource}
						if l.GetKind() == util.KindDeployment {
							l.Items = append(l.Items, cdResource)
						} else if l.GetKind() != util.KindService {
							return getErr
						}
						return nil
					},
				},
				want: want{
					crks: []*unstructured.Unstructured{
						&cdResource,
					},
					err: nil,
				},
			},
		}
		for name, tc := range cases {
			tclient := test.MockClient{
				MockGet:  test.NewMockGetFn(nil, tc.fields.getFunc),
				MockList: test.NewMockListFn(nil, tc.fields.listFunc),
			}
			got, err := util.FetchWorkloadChildResources(ctx, log, &tclient, unstructuredWorkload)
			By(fmt.Sprint("Running test: ", name))
			Expect(tc.want.err).Should(util.BeEquivalentToError(err))
			Expect(tc.want.crks).Should(Equal(got))
		}
	})
})

var _ = Describe("Test unstructured related helper utils", func() {
	It("Test get CRD name from an unstructured object", func() {
		tests := map[string]struct {
			u   *unstructured.Unstructured
			exp string
		}{
			"native resource": {
				u: &unstructured.Unstructured{Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
				}},
				exp: "deployments.apps",
			},
			"extended resource": {
				u: &unstructured.Unstructured{Object: map[string]interface{}{
					"apiVersion": "extend.oam.dev/v1alpha2",
					"kind":       "SimpleRolloutTrait",
				}},
				exp: "simplerollouttraits.extend.oam.dev",
			},
		}
		for name, ti := range tests {
			got := util.GetCRDName(ti.u)
			By(fmt.Sprint("Running test: ", name))
			Expect(ti.exp).Should(Equal(got))
		}
	})
})
