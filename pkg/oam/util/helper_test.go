package util_test

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/adler32"
	"reflect"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakeClient "k8s.io/client-go/kubernetes/fake"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

var _ = Describe("Test LocateParentAppConfig helper utils", func() {
	ctx := context.Background()
	namespace := "oamNS"
	acKind := reflect.TypeOf(v1alpha2.ApplicationConfiguration{}).Name()
	mockVersion := "core.oam.dev/v1alpha2"
	acName := "mockAC"

	mockAC := v1alpha2.ApplicationConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       acKind,
			APIVersion: mockVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      acName,
			Namespace: namespace,
		},
		Spec: v1alpha2.ApplicationConfigurationSpec{
			Components: nil,
		},
	}

	mockOwnerRef := metav1.OwnerReference{
		APIVersion: mockVersion,
		Kind:       acKind,
		Name:       acName,
	}

	cmpKind := "Component"
	cmpName := "mockComponent"

	// use Component as mock oam.Object
	mockComp := v1alpha2.Component{
		TypeMeta: metav1.TypeMeta{
			Kind:       cmpKind,
			APIVersion: mockVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            cmpName,
			Namespace:       namespace,
			OwnerReferences: []metav1.OwnerReference{mockOwnerRef},
		},
		Spec: v1alpha2.ComponentSpec{
			Workload: runtime.RawExtension{
				Raw:    nil,
				Object: nil,
			},
			Parameters: nil,
		},
	}

	mockCompWithEmptyOwnerRef := mockComp
	mockCompWithEmptyOwnerRef.ObjectMeta.OwnerReferences = nil

	getErr := fmt.Errorf("get error")

	It("Test LocateParentAppConfig", func() {
		type fields struct {
			getFunc test.ObjectFn
			oamObj  oam.Object
		}
		type want struct {
			ac  oam.Object
			err error
		}

		cases := map[string]struct {
			fields fields
			want   want
		}{
			"LocateParentAppConfig fail when getAppConfig fails": {
				fields: fields{
					getFunc: func(obj runtime.Object) error {
						return getErr
					},
					oamObj: &mockComp,
				},
				want: want{
					ac:  nil,
					err: getErr,
				},
			},

			"LocateParentAppConfig fail when no ApplicationConfiguration in OwnerReferences": {
				fields: fields{
					getFunc: func(obj runtime.Object) error {
						return getErr
					},
					oamObj: &mockCompWithEmptyOwnerRef,
				},
				want: want{
					ac:  nil,
					err: errors.Errorf(util.ErrLocateAppConfig),
				},
			},
			"LocateParentAppConfig success": {
				fields: fields{
					getFunc: func(obj runtime.Object) error {
						o, _ := obj.(*v1alpha2.ApplicationConfiguration)
						ac := mockAC
						*o = ac
						return nil
					},
					oamObj: &mockComp,
				},
				want: want{
					ac:  &mockAC,
					err: nil,
				},
			},
		}
		for name, tc := range cases {
			tclient := test.MockClient{
				MockGet: test.NewMockGetFn(nil, tc.fields.getFunc),
			}
			got, err := util.LocateParentAppConfig(ctx, &tclient, tc.fields.oamObj)
			By(fmt.Sprint("Running test: ", name))
			Expect(tc.want.err).Should(util.BeEquivalentToError(err))
			if tc.want.ac == nil {
				Expect(tc.want.ac).Should(BeNil())
			} else {
				Expect(tc.want.ac).Should(Equal(got))
			}
		}
	})
})

var _ = Describe(" Trait Controller Test", func() {
	BeforeEach(func() {
		logf.Log.Info("Set up resources before a unit test")
	})

	AfterEach(func() {
		logf.Log.Info("Clean up resources after a unit test")
	})

	It("Test fetch the workload the trait is reference to", func() {
		By("Setting up variables")
		log := ctrl.Log.WithName("ManualScalarTraitReconciler")
		noRefNameTrait := v1alpha2.ManualScalerTrait{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.ManualScalerTraitKind,
			},
			Spec: v1alpha2.ManualScalerTraitSpec{
				ReplicaCount: 3,
				WorkloadReference: v1alpha1.TypedReference{
					APIVersion: "apiversion",
					Kind:       "Kind",
				},
			},
		}
		// put the workload name back
		manualScalar := noRefNameTrait
		manualScalar.Spec.WorkloadReference.Name = "wokload-example"
		ctx := context.Background()
		wl := v1alpha2.ContainerizedWorkload{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.ContainerizedWorkloadKind,
			},
		}
		uwl, _ := util.Object2Unstructured(wl)
		refErr := errors.New("no workload reference")
		workloadErr := fmt.Errorf("workload errr")

		type fields struct {
			trait   oam.Trait
			getFunc test.ObjectFn
		}
		type want struct {
			wl  *unstructured.Unstructured
			err error
		}
		cases := map[string]struct {
			fields fields
			want   want
		}{
			"FetchWorkload fail with mal-structured workloadRef": {
				fields: fields{
					trait: &noRefNameTrait,
				},
				want: want{
					wl:  nil,
					err: refErr,
				},
			},
			"FetchWorkload fails when getWorkload fails": {
				fields: fields{
					trait: &manualScalar,
					getFunc: func(obj runtime.Object) error {
						return workloadErr
					},
				},
				want: want{
					wl:  nil,
					err: workloadErr,
				},
			},
			"FetchWorkload succeeds when getWorkload succeeds": {
				fields: fields{
					trait: &manualScalar,
					getFunc: func(obj runtime.Object) error {
						o, _ := obj.(*unstructured.Unstructured)
						*o = *uwl
						return nil
					},
				},
				want: want{
					wl:  uwl,
					err: nil,
				},
			},
		}
		for name, tc := range cases {
			tclient := test.NewMockClient()
			tclient.MockGet = test.NewMockGetFn(nil, tc.fields.getFunc)
			gotWL, err := util.FetchWorkload(ctx, tclient, log, tc.fields.trait)
			By(fmt.Sprint("Running test: ", name))
			Expect(tc.want.err).Should(util.BeEquivalentToError(err))
			Expect(tc.want.wl).Should(Equal(gotWL))
		}
	})
})

var _ = Describe("Test scope related helper utils", func() {
	ctx := context.Background()
	namespace := "oamNS"
	scopeDefinitionKind := "ScopeDefinition"
	mockVerision := "core.oam.dev/v1alpha2"
	scopeDefinitionName := "mockscopes.core.oam.dev"
	scopeDefinitionRefName := "mockscopes.core.oam.dev"
	scopeDefinitionWorkloadRefsPath := "spec.workloadRefs"

	mockScopeDefinition := v1alpha2.ScopeDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       scopeDefinitionKind,
			APIVersion: mockVerision,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      scopeDefinitionName,
			Namespace: namespace,
		},
		Spec: v1alpha2.ScopeDefinitionSpec{
			Reference: v1alpha2.DefinitionReference{
				Name: scopeDefinitionRefName,
			},
			WorkloadRefsPath:      scopeDefinitionWorkloadRefsPath,
			AllowComponentOverlap: false,
		},
	}

	scopeKind := "HealthScope"
	scopeName := "HealthScope"

	mockScope := v1alpha2.HealthScope{
		TypeMeta: metav1.TypeMeta{
			Kind:       scopeKind,
			APIVersion: mockVerision,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      scopeName,
			Namespace: namespace,
		},
		Spec: v1alpha2.HealthScopeSpec{
			ProbeTimeout:       nil,
			ProbeInterval:      nil,
			WorkloadReferences: nil,
		},
	}

	unstructuredScope, _ := util.Object2Unstructured(mockScope)

	getErr := fmt.Errorf("get error")

	It("Test FetchScopeDefinition", func() {
		type fields struct {
			getFunc test.ObjectFn
		}
		type want struct {
			spd *v1alpha2.ScopeDefinition
			err error
		}

		cases := map[string]struct {
			fields fields
			want   want
		}{
			"FetchScopeDefinition fail when getScopeDefinition fails": {
				fields: fields{
					getFunc: func(obj runtime.Object) error {
						return getErr
					},
				},
				want: want{
					spd: nil,
					err: getErr,
				},
			},

			"FetchScopeDefinition Success": {
				fields: fields{
					getFunc: func(obj runtime.Object) error {
						o, _ := obj.(*v1alpha2.ScopeDefinition)
						sd := mockScopeDefinition
						*o = sd
						return nil
					},
				},
				want: want{
					spd: &mockScopeDefinition,
					err: nil,
				},
			},
		}
		for name, tc := range cases {
			tclient := test.MockClient{
				MockGet: test.NewMockGetFn(nil, tc.fields.getFunc),
			}
			got, err := util.FetchScopeDefinition(ctx, &tclient, unstructuredScope)
			By(fmt.Sprint("Running test: ", name))
			Expect(tc.want.err).Should(util.BeEquivalentToError(err))
			Expect(tc.want.spd).Should(Equal(got))
		}
	})
})

var _ = Describe("Test trait related helper utils", func() {
	ctx := context.Background()
	namespace := "oamNS"
	traitDefinitionKind := "TraitDefinition"
	mockVerision := "core.oam.dev/v1alpha2"
	traiitDefinitionName := "mocktraits.core.oam.dev"
	traiitDefinitionRefName := "mocktraits.core.oam.dev"
	traiitDefinitionWorkloadRefPath := "spec.workloadRef"

	mockTraitDefinition := v1alpha2.TraitDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       traitDefinitionKind,
			APIVersion: mockVerision,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      traiitDefinitionName,
			Namespace: namespace,
		},
		Spec: v1alpha2.TraitDefinitionSpec{
			Reference: v1alpha2.DefinitionReference{
				Name: traiitDefinitionRefName,
			},
			RevisionEnabled:    false,
			WorkloadRefPath:    traiitDefinitionWorkloadRefPath,
			AppliesToWorkloads: nil,
		},
	}

	traitName := "ms-trait"

	mockTrait := v1alpha2.ManualScalerTrait{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      traitName,
		},
		Spec: v1alpha2.ManualScalerTraitSpec{
			ReplicaCount: 3,
		},
	}

	unstructuredTrait, _ := util.Object2Unstructured(mockTrait)

	getErr := fmt.Errorf("get error")

	It("Test FetchTraitDefinition", func() {
		type fields struct {
			getFunc test.ObjectFn
		}
		type want struct {
			td  *v1alpha2.TraitDefinition
			err error
		}

		cases := map[string]struct {
			fields fields
			want   want
		}{
			"FetchTraitDefinition fail when getTraitDefinition fails": {
				fields: fields{
					getFunc: func(obj runtime.Object) error {
						return getErr
					},
				},
				want: want{
					td:  nil,
					err: getErr,
				},
			},

			"FetchTraitDefinition Success": {
				fields: fields{
					getFunc: func(obj runtime.Object) error {
						o, _ := obj.(*v1alpha2.TraitDefinition)
						td := mockTraitDefinition
						*o = td
						return nil
					},
				},
				want: want{
					td:  &mockTraitDefinition,
					err: nil,
				},
			},
		}
		for name, tc := range cases {
			tclient := test.MockClient{
				MockGet: test.NewMockGetFn(nil, tc.fields.getFunc),
			}
			got, err := util.FetchTraitDefinition(ctx, &tclient, unstructuredTrait)
			By(fmt.Sprint("Running test: ", name))
			Expect(tc.want.err).Should(util.BeEquivalentToError(err))
			Expect(tc.want.td).Should(Equal(got))
		}
	})
})

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

var _ = Describe("Test get component helper utils", func() {
	ctx := context.Background()
	It("Test Get Component", func() {
		type Case struct {
			caseName           string
			acc                v1alpha2.ApplicationConfigurationComponent
			expectComponent    *v1alpha2.Component
			expectRevisionName string
			expectError        error
		}

		namespace := "ns"
		componentName := "newcomponent"
		revisionName := "newcomponent-aa1111"
		revisionName2 := "newcomponent-bb1111"

		componnet1 := v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: namespace,
			},
			Spec:   v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &unstructured.Unstructured{}}},
			Status: v1alpha2.ComponentStatus{},
		}

		component2 := v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"apiVersion": "New",
				},
			}}}},
			Status: v1alpha2.ComponentStatus{
				LatestRevision: &v1alpha2.Revision{Name: revisionName2, Revision: 2},
			},
		}

		fakeAppClient := fakeClient.NewSimpleClientset().AppsV1()
		fakeAppClient.ControllerRevisions(namespace).Create(context.Background(), &appsv1.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{Name: revisionName, Namespace: namespace},
			Data:       runtime.RawExtension{Object: &componnet1},
			Revision:   1,
		}, metav1.CreateOptions{})

		client := &test.MockClient{MockGet: test.NewMockGetFn(nil, func(obj runtime.Object) error {
			if o, ok := obj.(*v1alpha2.Component); ok {
				*o = component2
			}
			return nil

		})}
		testCases := []Case{
			{
				caseName:           "get component by revisionName",
				acc:                v1alpha2.ApplicationConfigurationComponent{RevisionName: revisionName},
				expectComponent:    &componnet1,
				expectRevisionName: revisionName,
				expectError:        nil,
			},
			{
				caseName:           "get component by componentName",
				acc:                v1alpha2.ApplicationConfigurationComponent{ComponentName: componentName},
				expectComponent:    &component2,
				expectRevisionName: revisionName2,
				expectError:        nil,
			},
		}

		for _, tc := range testCases {
			By("Running:" + tc.caseName)
			c, r, err := util.GetComponent(ctx, client, fakeAppClient, tc.acc, namespace)
			Expect(c).Should(Equal(tc.expectComponent))
			Expect(r).Should(Equal(tc.expectRevisionName))
			Expect(err).Should(util.BeEquivalentToError(tc.expectError))
		}
	})

})

var _ = Describe("Test UnpackRevisionData helper util", func() {
	It("Test unpack revision data", func() {
		comp1 := v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp1"}}
		comp1Raw, _ := json.Marshal(comp1)
		tests := map[string]struct {
			rev     *appsv1.ControllerRevision
			expComp *v1alpha2.Component
			expErr  error
			reason  string
		}{
			"controllerRevision with Component Obj": {
				rev:     &appsv1.ControllerRevision{Data: runtime.RawExtension{Object: &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp1"}}}},
				expComp: &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp1"}},
				reason:  "controllerRevision should align with component object",
			},
			"controllerRevision with Unknown Obj": {
				rev:    &appsv1.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev1"}, Data: runtime.RawExtension{Object: &runtime.Unknown{Raw: comp1Raw}}},
				reason: "controllerRevision must be decode into component object",
				expErr: fmt.Errorf("invalid type of revision rev1, type should not be *runtime.Unknown"),
			},
			"unmarshal with component data": {
				rev:     &appsv1.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev1"}, Data: runtime.RawExtension{Raw: comp1Raw}},
				reason:  "controllerRevision should unmarshal data and align with component object",
				expComp: &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp1"}},
			},
		}
		for name, ti := range tests {
			By("Running: " + name)
			comp, err := util.UnpackRevisionData(ti.rev)
			if ti.expErr != nil {
				Expect(err).Should(Equal(ti.expErr))
			} else {
				Expect(err).Should(BeNil())
				Expect(comp).Should(Equal(ti.expComp))
			}
		}

	})

})
