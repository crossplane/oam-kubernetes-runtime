package manualscalertrait

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	oamv1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

func TestManualscalertrait(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Manualscalertrait Suite")
}

var _ = Describe("Manualscalar Trait Controller Test", func() {
	BeforeEach(func() {
		logf.Log.Info("Set up resources before a unit test")
	})

	AfterEach(func() {
		logf.Log.Info("Clean up resources after a unit test")
	})

	It("Test fetch the workload the trait is reference to", func() {
		By("Setting up variables")
		log := ctrl.Log.WithName("ManualScalarTraitReconciler")
		reconciler := &Reconciler{
			log: log,
		}
		manualScalar := &oamv1alpha2.ManualScalerTrait{
			TypeMeta: metav1.TypeMeta{
				APIVersion: oamv1alpha2.SchemeGroupVersion.String(),
				Kind:       oamv1alpha2.ManualScalerTraitKind,
			},
			Spec: oamv1alpha2.ManualScalerTraitSpec{
				ReplicaCount: 3,
				WorkloadReference: runtimev1alpha1.TypedReference{
					APIVersion: "apiversion",
					Kind:       "Kind",
				},
			},
		}
		ctx := context.Background()
		wl := oamv1alpha2.ContainerizedWorkload{
			TypeMeta: metav1.TypeMeta{
				APIVersion: oamv1alpha2.SchemeGroupVersion.String(),
				Kind:       oamv1alpha2.ContainerizedWorkloadKind,
			},
		}
		uwl, _ := util.Object2Unstructured(wl)
		workloadErr := fmt.Errorf("workload errr")
		updateErr := fmt.Errorf("update errr")

		type fields struct {
			getFunc         test.ObjectFn
			patchStatusFunc test.MockStatusPatchFn
		}
		type want struct {
			wl     *unstructured.Unstructured
			result ctrl.Result
			err    error
		}
		cases := map[string]struct {
			fields fields
			want   want
		}{
			"FetchWorkload fails when getWorkload fails": {
				fields: fields{
					getFunc: func(obj runtime.Object) error {
						return workloadErr
					},
					patchStatusFunc: func(_ context.Context, obj runtime.Object, patch client.Patch,
						_ ...client.PatchOption) error {
						return nil
					},
				},
				want: want{
					wl:     nil,
					result: util.ReconcileWaitResult,
					err:    nil,
				},
			},
			"FetchWorkload fail and update fails when getWorkload fails": {
				fields: fields{
					getFunc: func(obj runtime.Object) error {
						return workloadErr
					},
					patchStatusFunc: func(_ context.Context, obj runtime.Object, patch client.Patch,
						_ ...client.PatchOption) error {
						return updateErr
					},
				},
				want: want{
					wl:     nil,
					result: util.ReconcileWaitResult,
					err:    errors.Wrap(updateErr, util.ErrUpdateStatus),
				},
			},
			"FetchWorkload succeeds when getWorkload succeeds": {
				fields: fields{
					getFunc: func(obj runtime.Object) error {
						o, _ := obj.(*unstructured.Unstructured)
						*o = *uwl
						return nil
					},
					patchStatusFunc: func(_ context.Context, obj runtime.Object, patch client.Patch,
						_ ...client.PatchOption) error {
						return updateErr
					},
				},
				want: want{
					wl:     uwl,
					result: ctrl.Result{},
					err:    nil,
				},
			},
		}
		for name, tc := range cases {
			tclient := test.NewMockClient()
			tclient.MockGet = test.NewMockGetFn(nil, tc.fields.getFunc)
			tclient.MockStatusPatch = tc.fields.patchStatusFunc
			reconciler.Client = tclient
			gotWL, result, err := reconciler.fetchWorkload(ctx, log, manualScalar)
			By(fmt.Sprint("Running test: ", name))
			Expect(tc.want.err).Should(util.BeEquivalentToError(err))
			Expect(tc.want.wl).Should(Equal(gotWL))
			Expect(tc.want.result).Should(Equal(result))
		}
	})
})
