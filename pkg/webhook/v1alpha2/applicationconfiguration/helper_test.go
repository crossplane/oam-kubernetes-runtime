package applicationconfiguration

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"

	json "github.com/json-iterator/go"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

func TestCheckComponentVersionEnabled(t *testing.T) {
	ctx := context.Background()
	mockClient := test.NewMockClient()
	traitRevisionEnabled, _ := json.Marshal(v1alpha2.ManualScalerTrait{
		TypeMeta: v1.TypeMeta{
			Kind:       "ManualScalerTrait",
			APIVersion: "core.oam.dev",
		},
	})

	tests := []struct {
		caseName   string
		mockGetFun test.MockGetFn
		acc        v1alpha2.ApplicationConfigurationComponent
		result     bool
	}{
		{
			caseName: "Versioning Disabled",
			acc: v1alpha2.ApplicationConfigurationComponent{
				ComponentName: "compName",
			},
			result: false,
		},
		{
			caseName: "Versioning Enabled With RevisionName",
			acc: v1alpha2.ApplicationConfigurationComponent{
				RevisionName: "revisionName",
			},
			result: true,
		},
		{
			caseName: "Versioning Enabled With RevisionEnabled Trait",
			mockGetFun: func(_ context.Context, _ types.NamespacedName, obj runtime.Object) error {
				if o, ok := obj.(*v1alpha2.TraitDefinition); ok {
					*o = v1alpha2.TraitDefinition{
						Spec: v1alpha2.TraitDefinitionSpec{
							RevisionEnabled: true,
						},
					}
					return nil
				}
				return nil
			},
			acc: v1alpha2.ApplicationConfigurationComponent{
				ComponentName: "compName",
				Traits: []v1alpha2.ComponentTrait{
					{
						Trait: runtime.RawExtension{
							Raw: traitRevisionEnabled,
						},
					},
				},
			},
			result: true,
		},
	}

	for _, tv := range tests {
		mockClient.MockGet = tv.mockGetFun
		result, err := checkComponentVersionEnabled(ctx, mockClient, &tv.acc)
		if result != tv.result {
			t.Logf("Running test case: %q", tv.caseName)
			t.Errorf("Expect check result %v but got %v and error %q", tv.result, result, err.Error())
		}
	}

}
