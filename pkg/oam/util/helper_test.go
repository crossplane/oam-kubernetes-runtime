package util_test

import (
	"context"
	"fmt"
	"hash/adler32"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

var _ = Describe("Test GenTraitName helper utils", func() {
	It("Test generate trait name", func() {

		mts := v1alpha2.ManualScalerTrait{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "sample-manualscaler-trait",
			},
			Spec: v1alpha2.ManualScalerTraitSpec{
				ReplicaCount: 3,
			},
		}

		test := []struct {
			name     string
			template *v1alpha2.ComponentTrait
			exp      string
		}{
			{
				name:     "simple",
				template: &v1alpha2.ComponentTrait{},
				exp:      "simple-trait-67b8949f8d",
			},
			{
				name: "simple",
				template: &v1alpha2.ComponentTrait{
					Trait: runtime.RawExtension{
						Object: &mts,
					},
				},
				exp: "simple-trait-5ddc8b7556",
			},
		}
		for _, test := range test {

			got := util.GenTraitName(test.name, test.template)
			By(fmt.Sprint("Running test: ", test.name))
			Expect(test.exp).Should(Equal(got))
		}

	})
})

var _ = Describe("Test ComputeHash helper utils", func() {
	It("Test generate hash", func() {

		mts := v1alpha2.ManualScalerTrait{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "sample-manualscaler-trait",
			},
			Spec: v1alpha2.ManualScalerTraitSpec{
				ReplicaCount: 3,
			},
		}

		test := []struct {
			name     string
			template *v1alpha2.ComponentTrait
			exp      string
		}{
			{
				name:     "simple",
				template: &v1alpha2.ComponentTrait{},
				exp:      "67b8949f8d",
			},
			{
				name: "simple",
				template: &v1alpha2.ComponentTrait{
					Trait: runtime.RawExtension{
						Object: &mts,
					},
				},
				exp: "5ddc8b7556",
			},
		}
		for _, test := range test {
			got := util.ComputeHash(test.template)

			By(fmt.Sprint("Running test: ", got))
			Expect(test.exp).Should(Equal(got))
		}
	})
})

var _ = Describe("Test DeepHashObject helper utils", func() {
	It("Test generate hash", func() {

		successCases := []func() interface{}{
			func() interface{} { return 8675309 },
			func() interface{} { return "Jenny, I got your number" },
			func() interface{} { return []string{"eight", "six", "seven"} },
		}

		for _, tc := range successCases {
			hasher1 := adler32.New()
			util.DeepHashObject(hasher1, tc())
			hash1 := hasher1.Sum32()
			util.DeepHashObject(hasher1, tc())
			hash2 := hasher1.Sum32()

			Expect(hash1).Should(Equal(hash2))
		}
	})
})

var _ = Describe("Test PatchCondition helper utils", func() {
	It("Test PatchCondition", func() {
		type args struct {
			ctx       context.Context
			r         client.StatusClient
			workload  util.ConditionedObject
			condition []v1alpha1.Condition
		}
		patchErr := fmt.Errorf("eww")
		tests := []struct {
			name     string
			args     args
			expected error
		}{
			{
				name: "success",
				args: args{
					ctx: context.Background(),
					r: &test.MockClient{
						MockStatusPatch: test.NewMockStatusPatchFn(nil),
					},
					workload: &fake.Claim{},
					condition: []v1alpha1.Condition{
						{},
					},
				},
				expected: nil,
			},
			{
				name: "fail",
				args: args{
					ctx: context.Background(),
					r: &test.MockClient{
						MockStatusPatch: test.NewMockStatusPatchFn(patchErr),
					},
					workload: &fake.Claim{},
					condition: []v1alpha1.Condition{
						{},
					},
				},
				expected: errors.Wrap(patchErr, util.ErrUpdateStatus),
			},
		}
		for _, tt := range tests {
			err := util.PatchCondition(tt.args.ctx, tt.args.r, tt.args.workload, tt.args.condition...)
			if tt.expected == nil {
				BeNil().Match(err)
			} else {
				Expect(err.Error()).Should(Equal(tt.expected.Error()))
			}
		}
	})
})
