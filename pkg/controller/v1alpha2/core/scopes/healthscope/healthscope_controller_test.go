package healthscope

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	appsv1 "k8s.io/api/apps/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/mock"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

var (
	errNotFound = errors.New("HealthScope not found")
	// errGetResources = errors.New("cannot get resources")
)

func TestHealthScope(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HealthScope Suite")
}

var _ = Describe("HealthScope Controller Reconcile Test", func() {
	mockMgr := &mock.Manager{
		Client: &test.MockClient{},
	}
	MockHealthyChecker := WorkloadHealthCheckFn(
		func(context.Context, client.Client, v1alpha1.TypedReference, string) *HealthCondition {
			return &HealthCondition{IsHealthy: true}
		})
	MockUnhealthyChecker := WorkloadHealthCheckFn(
		func(context.Context, client.Client, v1alpha1.TypedReference, string) *HealthCondition {
			return &HealthCondition{IsHealthy: false}
		})
	reconciler := NewReconciler(mockMgr,
		WithLogger(logging.NewNopLogger().WithValues("HealthScopeReconciler")),
		WithRecorder(event.NewNopRecorder()),
		WithChecker(MockHealthyChecker),
	)

	hs := v1alpha2.HealthScope{Spec: corev1alpha2.HealthScopeSpec{WorkloadReferences: []v1alpha1.TypedReference{
		// add one wlRef to trigger mockChecker
		{
			APIVersion: "mock",
			Kind:       "mock",
		},
	}}}

	BeforeEach(func() {
		logf.Log.Info("Set up resources before an unit test")
		// remove built-in checkers then fulfill mock checkers
		reconciler.checkers = []WorloadHealthChecker{}
	})

	AfterEach(func() {
		logf.Log.Info("Clean up resources after an unit test")
	})

	It("Test HealthScope Not Found", func() {
		reconciler.client = &test.MockClient{
			MockGet: func(ctx context.Context,
				key client.ObjectKey, obj runtime.Object) error {
				return errNotFound
			},
		}
		result, err := reconciler.Reconcile(reconcile.Request{})
		Expect(result).Should(Equal(reconcile.Result{}))
		Expect(err).Should(util.BeEquivalentToError(errors.Wrap(errNotFound, errGetHealthScope)))
	})

	It("Test Reconcile UpdateHealthStatus Error", func() {
		reconciler.checkers = append(reconciler.checkers, MockHealthyChecker)
		reconciler.client = &test.MockClient{
			MockGet: func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
				o, _ := obj.(*v1alpha2.HealthScope)
				*o = hs
				return nil
			},
			MockStatusUpdate: func(_ context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
				return errMockErr
			},
		}
		_, err := reconciler.Reconcile(reconcile.Request{})
		Expect(err).Should(util.BeEquivalentToError(errors.Wrap(errMockErr, errUpdateHealthScopeStatus)))
	})

	It("Test Reconcile Success with healthy scope", func() {
		reconciler.checkers = append(reconciler.checkers, MockHealthyChecker)
		reconciler.client = &test.MockClient{
			MockGet: func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
				o, _ := obj.(*v1alpha2.HealthScope)
				*o = hs
				return nil
			},
			MockStatusUpdate: func(_ context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
				return nil
			},
		}
		_, err := reconciler.Reconcile(reconcile.Request{})
		Expect(err).Should(BeNil())
	})

	It("Test Reconcile Success with unhealthy scope", func() {
		reconciler.checkers = append(reconciler.checkers, MockUnhealthyChecker)
		reconciler.client = &test.MockClient{
			MockGet: func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
				o, _ := obj.(*v1alpha2.HealthScope)
				*o = hs
				return nil
			},
			MockStatusUpdate: func(_ context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
				return nil
			},
		}
		_, err := reconciler.Reconcile(reconcile.Request{})
		Expect(err).Should(BeNil())
	})
})

var _ = Describe("Test GetScopeHealthStatus", func() {
	ctx := context.Background()
	mockMgr := &mock.Manager{
		Client: &test.MockClient{},
	}
	reconciler := NewReconciler(mockMgr,
		WithLogger(logging.NewNopLogger().WithValues("HealthScopeReconciler")),
		WithRecorder(event.NewNopRecorder()),
	)
	reconciler.client = test.NewMockClient()

	hs := v1alpha2.HealthScope{}

	var cwRef, deployRef, svcRef v1alpha1.TypedReference
	cwRef.SetGroupVersionKind(corev1alpha2.SchemeGroupVersion.WithKind(kindContainerizedWorkload))
	deployRef.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind(kindDeployment))
	svcRef.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind(kindService))
	hGeneralRef := v1alpha1.TypedReference{
		APIVersion: "unknown",
		Kind:       "unknown",
		Name:       "healthyGeneral",
	}

	cw := corev1alpha2.ContainerizedWorkload{}
	cw.SetGroupVersionKind(corev1alpha2.SchemeGroupVersion.WithKind(kindContainerizedWorkload))
	cw.Status.Resources = []v1alpha1.TypedReference{deployRef, svcRef}

	hDeploy := appsv1.Deployment{
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 1, // healthy
		},
	}
	hDeploy.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind(kindDeployment))
	hGeneralWL := &unstructured.Unstructured{Object: make(map[string]interface{})}
	fieldpath.Pave(hGeneralWL.Object).SetValue("status.readyReplicas", 1)         // healthy
	fieldpath.Pave(hGeneralWL.Object).SetValue("metadata.name", "healthyGeneral") // healthy

	uhGeneralRef := v1alpha1.TypedReference{
		APIVersion: "unknown",
		Kind:       "unknown",
		Name:       "unhealthyGeneral",
	}
	uhDeploy := hDeploy
	uhDeploy.Status.ReadyReplicas = 0 // unhealthy
	uhGeneralWL := &unstructured.Unstructured{Object: make(map[string]interface{})}
	fieldpath.Pave(uhGeneralWL.Object).SetValue("status.readyReplicas", 0)           // healthy
	fieldpath.Pave(uhGeneralWL.Object).SetValue("metadata.name", "unhealthyGeneral") // healthy
	unsupporttedWL := &unstructured.Unstructured{Object: make(map[string]interface{})}
	fieldpath.Pave(unsupporttedWL.Object).SetValue("status.unknown", 1) // healthy

	BeforeEach(func() {
		logf.Log.Info("Set up resources before an unit test")
		hs.Spec.WorkloadReferences = []v1alpha1.TypedReference{}
	})

	AfterEach(func() {
		logf.Log.Info("Clean up resources after an unit test")
	})

	// use ContainerizedWorkload and Deployment checker
	It("Test healthy scope", func() {
		tests := []struct {
			caseName       string
			hsWorkloadRefs []v1alpha1.TypedReference
			mockGetFn      test.MockGetFn
		}{
			{
				caseName:       "2 supportted workloads(cw,deploy)",
				hsWorkloadRefs: []v1alpha1.TypedReference{cwRef, deployRef},
				mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
					if o, ok := obj.(*corev1alpha2.ContainerizedWorkload); ok {
						*o = cw
					}
					if o, ok := obj.(*appsv1.Deployment); ok {
						*o = hDeploy
					}
					return nil
				},
			},
			{
				caseName:       "1 supportted workload and 1 general workload",
				hsWorkloadRefs: []v1alpha1.TypedReference{cwRef, hGeneralRef},
				mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
					switch o := obj.(type) {
					case *corev1alpha2.ContainerizedWorkload:
						*o = cw
					case *appsv1.Deployment:
						*o = hDeploy
					case *unstructured.Unstructured:
						*o = *hGeneralWL
					}
					return nil
				},
			},
			{
				caseName:       "2 general workloads",
				hsWorkloadRefs: []v1alpha1.TypedReference{hGeneralRef, hGeneralRef},
				mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
					o, _ := obj.(*unstructured.Unstructured)
					*o = *hGeneralWL
					return nil
				},
			},
		}
		for _, tc := range tests {
			By("Running: " + tc.caseName)
			mockClient := &test.MockClient{
				MockGet: tc.mockGetFn,
			}
			reconciler.client = mockClient
			hs.Spec.WorkloadReferences = tc.hsWorkloadRefs
			result := reconciler.GetScopeHealthStatus(ctx, &hs)
			Expect(result).ShouldNot(BeNil())
			Expect(result.IsHealthy).Should(Equal(true))
		}
	})

	// use ContainerizedWorkload and Deployment checker
	It("Test unhealthy scope", func() {
		tests := []struct {
			caseName       string
			hsWorkloadRefs []v1alpha1.TypedReference
			mockGetFn      test.MockGetFn
		}{
			{
				caseName:       "2 supportted workloads but one is unhealthy",
				hsWorkloadRefs: []v1alpha1.TypedReference{cwRef, deployRef},
				mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
					switch o := obj.(type) {
					case *corev1alpha2.ContainerizedWorkload:
						*o = cw
					case *appsv1.Deployment:
						*o = hDeploy
					case *unstructured.Unstructured:
						// return err when get svc of cw, then check fails
						return errMockErr
					}
					return nil
				},
			},
			{
				caseName:       "2 general workloads but one is unhealthy",
				hsWorkloadRefs: []v1alpha1.TypedReference{hGeneralRef, uhGeneralRef},
				mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
					if key.Name == "healthyGeneral" {
						o, _ := obj.(*unstructured.Unstructured)
						*o = *hGeneralWL
					}
					if key.Name == "unhealthyGeneral" {
						o, _ := obj.(*unstructured.Unstructured)
						*o = *uhGeneralWL
					}
					return nil
				},
			},
			{
				caseName:       "1 healthy supportted workload and 1 unsupportted workloads",
				hsWorkloadRefs: []v1alpha1.TypedReference{cwRef, uhGeneralRef},
				mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
					switch o := obj.(type) {
					case *corev1alpha2.ContainerizedWorkload:
						*o = cw
					case *appsv1.Deployment:
						*o = hDeploy
					case *unstructured.Unstructured:
						*o = *unsupporttedWL
					}
					return nil
				},
			},
		}

		for _, tc := range tests {
			By("Running: " + tc.caseName)
			mockClient := &test.MockClient{
				MockGet: tc.mockGetFn,
			}
			reconciler.client = mockClient
			hs.Spec.WorkloadReferences = tc.hsWorkloadRefs
			result := reconciler.GetScopeHealthStatus(ctx, &hs)
			Expect(result).ShouldNot(BeNil())
			Expect(result.IsHealthy).Should(Equal(false))
		}
	})
})
