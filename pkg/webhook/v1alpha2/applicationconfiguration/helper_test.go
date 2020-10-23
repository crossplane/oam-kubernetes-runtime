package applicationconfiguration

import (
	"context"
	"fmt"
	"testing"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/mock"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/stretchr/testify/assert"

	json "github.com/json-iterator/go"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

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
		{
			caseName: "Unmarshal error occurs",
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
							Raw: nil,
						},
					},
				},
			},
			result: false,
		},
	}
	mapper := mock.NewMockMapper()

	for _, tv := range tests {
		func(t *testing.T) {
			mockClient.MockGet = tv.mockGetFun
			result, _ := checkComponentVersionEnabled(ctx, mockClient, mapper, &tv.acc)
			assert.Equal(t, tv.result, result, fmt.Sprintf("Test case: %q", tv.caseName))
		}(t)
	}

}

func TestCheckParams(t *testing.T) {
	wlNameValue := "wlName"
	pName := "wlnameParam"
	wlParam := v1alpha2.ComponentParameter{
		Name:       pName,
		FieldPaths: []string{WorkloadNamePath},
	}
	wlParamValue := v1alpha2.ComponentParameterValue{
		Name:  pName,
		Value: intstr.FromString(wlNameValue),
	}

	mockValue := "mockValue"
	mockPName := "mockParam"
	mockFieldPath := "a.b"
	mockParam := v1alpha2.ComponentParameter{
		Name:       mockPName,
		FieldPaths: []string{mockFieldPath},
	}
	mockParamValue := v1alpha2.ComponentParameterValue{
		Name:  pName,
		Value: intstr.FromString(mockValue),
	}
	tests := []struct {
		caseName         string
		cps              []v1alpha2.ComponentParameter
		cpvs             []v1alpha2.ComponentParameterValue
		expectResult     bool
		expectParamValue string
	}{
		{
			caseName:         "get wokload name params and value",
			cps:              []v1alpha2.ComponentParameter{wlParam},
			cpvs:             []v1alpha2.ComponentParameterValue{wlParamValue},
			expectResult:     false,
			expectParamValue: wlNameValue,
		},
		{
			caseName:         "not found workload name params",
			cps:              []v1alpha2.ComponentParameter{mockParam},
			cpvs:             []v1alpha2.ComponentParameterValue{mockParamValue},
			expectResult:     true,
			expectParamValue: "",
		},
	}

	for _, tc := range tests {
		func(t *testing.T) {
			result, pValue := checkParams(tc.cps, tc.cpvs)
			assert.Equal(t, result, tc.expectResult,
				fmt.Sprintf("test case: %v", tc.caseName))
			assert.Equal(t, pValue, tc.expectParamValue,
				fmt.Sprintf("test case: %v", tc.caseName))
		}(t)

	}
}
