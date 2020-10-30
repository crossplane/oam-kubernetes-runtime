package containerizedworkload

import (
	"context"
	"reflect"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

func TestContainerizedWorkloadReconciler_cleanupResources(t *testing.T) {
	type args struct {
		ctx        context.Context
		workload   *v1alpha2.ContainerizedWorkload
		deployUID  *types.UID
		serviceUID *types.UID
	}
	testCases := map[string]struct {
		reconciler Reconciler
		args       args
		wantErr    bool
	}{
		// TODO: Add test cases.
	}
	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			if err := testCase.reconciler.cleanupResources(testCase.args.ctx, testCase.args.workload, testCase.args.deployUID,
				testCase.args.serviceUID); (err != nil) != testCase.wantErr {
				t.Errorf("cleanupResources() error = %v, wantErr %v", err, testCase.wantErr)
			}
		})
	}
}

func TestRenderDeployment(t *testing.T) {
	var scheme = runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = core.AddToScheme(scheme)

	r := Reconciler{
		Client: nil,
		log:    ctrl.Log.WithName("ContainerizedWorkload"),
		record: nil,
		Scheme: scheme,
	}

	cwLabel := map[string]string{
		"oam.dev/enabled": "true",
	}
	dmLabel := cwLabel
	dmLabel[labelKey] = workloadUID
	cwAnnotation := map[string]string{
		"dapr.io/enabled": "true",
	}
	dmAnnotation := cwAnnotation

	w := containerizedWorkload(cwWithAnnotation(cwAnnotation), cwWithLabel(cwLabel))
	deploy, err := r.renderDeployment(context.Background(), w)

	if diff := cmp.Diff(nil, err, test.EquateErrors()); diff != "" {
		t.Errorf("%s\ncontainerizedWorkloadTranslator(...): -want error, +got error:\n%s", "translate", diff)
	}

	if diff := cmp.Diff(dmLabel, deploy.GetLabels()); diff != "" {
		t.Errorf("\nReason: %s\ncontainerizedWorkloadTranslator(...): -want, +got:\n%s", "pass label", diff)
	}

	if diff := cmp.Diff(dmAnnotation, deploy.GetAnnotations()); diff != "" {
		t.Errorf("\nReason: %s\ncontainerizedWorkloadTranslator(...): -want, +got:\n%s", "pass annotation", diff)
	}

	if diff := cmp.Diff(dmLabel, deploy.Spec.Template.GetLabels()); diff != "" {
		t.Errorf("\nReason: %s\ncontainerizedWorkloadTranslator(...): -want, +got:\n%s", "pass label", diff)
	}

	if len(deploy.GetOwnerReferences()) != 1 {
		t.Errorf("deplyment should have one owner reference")
	}

	dof := deploy.GetOwnerReferences()[0]
	if dof.Name != workloadName || dof.APIVersion != v1alpha2.SchemeGroupVersion.String() ||
		dof.Kind != reflect.TypeOf(v1alpha2.ContainerizedWorkload{}).Name() {
		t.Errorf("deplyment should have one owner reference pointing to the ContainerizedWorkload")
	}

}
