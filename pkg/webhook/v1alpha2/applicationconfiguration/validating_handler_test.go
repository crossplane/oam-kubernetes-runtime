package applicationconfiguration

import (
	"context"
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/mock"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	json "github.com/json-iterator/go"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestApplicationConfigurationValidation(t *testing.T) {
	var handler admission.Handler = &ValidatingHandler{}

	cwRaw, _ := json.Marshal(v1alpha2.ContainerizedWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "",
		},
	})

	mgr := &mock.Manager{
		Client: &test.MockClient{
			MockGet: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				o, _ := obj.(*appsv1.ControllerRevision)
				*o = appsv1.ControllerRevision{
					Data: runtime.RawExtension{Object: &v1alpha2.Component{
						Spec: v1alpha2.ComponentSpec{
							Workload: runtime.RawExtension{
								Raw: cwRaw,
							},
						}}}}
				return nil
			},
		},
	}
	resource := metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applicationconfigurations"}
	injc := handler.(inject.Client)
	injc.InjectClient(mgr.GetClient())
	decoder := handler.(admission.DecoderInjector)
	var scheme = runtime.NewScheme()
	_ = core.AddToScheme(scheme)
	dec, _ := admission.NewDecoder(scheme)
	decoder.InjectDecoder(dec)

	app1, _ := json.Marshal(v1alpha2.ApplicationConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-ns",
		},
		Spec: v1alpha2.ApplicationConfigurationSpec{Components: []v1alpha2.ApplicationConfigurationComponent{
			{
				RevisionName:  "r1",
				ComponentName: "c1",
			},
		}}})
	app2, _ := json.Marshal(v1alpha2.ApplicationConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-ns",
		},
		Spec: v1alpha2.ApplicationConfigurationSpec{Components: []v1alpha2.ApplicationConfigurationComponent{
			{
				RevisionName: "r1",
			},
		}}})

	tests := []struct {
		req    admission.Request
		pass   bool
		reason string
	}{
		{
			req: admission.Request{
				AdmissionRequest: admissionv1beta1.AdmissionRequest{
					Resource: resource,
					Object:   runtime.RawExtension{Raw: app1},
				},
			},
			pass:   false,
			reason: "componentName and revisionName are mutually exclusive, you can only specify one of them",
		},
		{
			req: admission.Request{
				AdmissionRequest: admissionv1beta1.AdmissionRequest{
					Resource: resource,
					Object:   runtime.RawExtension{Raw: app2},
				},
			},
			pass: true,
		},
	}
	for _, tv := range tests {
		resp := handler.Handle(context.Background(), tv.req)
		if tv.pass != resp.Allowed {
			t.Errorf("expect %v but got %v from validation", tv.pass, resp.Allowed)
		}
		if tv.reason != "" {
			if tv.reason != string(resp.Result.Reason) {
				t.Errorf("\nvalidation should fail by reason: %v \ninstead of by reason: %v ", tv.reason, resp.Result.Reason)
			}
		}
	}
}

func TestCheckWorkloadNameForVersioning(t *testing.T) {
	ctx := context.Background()
	mockClient := test.NewMockClient()

	revisionName := "r"
	componentName := "c"
	workloadName := "WorkloadName"
	paramName := "workloadName"
	paramValue := workloadName

	getErr := errors.New("get error")

	cwRaw, _ := json.Marshal(v1alpha2.ContainerizedWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "",
		},
	})
	cwRawWithWorkloadName, _ := json.Marshal(v1alpha2.ContainerizedWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Name: workloadName,
		},
	})

	kind := "ManualScalerTrait"
	version := "core.oam.dev"
	tName := "ms"
	msTraitRaw, _ := json.Marshal(v1alpha2.ManualScalerTrait{
		TypeMeta: metav1.TypeMeta{
			Kind:       kind,
			APIVersion: version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: tName,
		}})

	mapper := mock.NewMockDiscoveryMapper()

	tests := []struct {
		caseName     string
		appConfig    v1alpha2.ApplicationConfiguration
		mockGetFunc  test.MockGetFn
		expectResult bool
		expectReason string
	}{
		{
			caseName: "Test validation fails for workload name fixed in component",
			appConfig: v1alpha2.ApplicationConfiguration{
				Spec: v1alpha2.ApplicationConfigurationSpec{
					Components: []v1alpha2.ApplicationConfigurationComponent{
						{
							RevisionName: revisionName,
						},
					},
				},
			},
			mockGetFunc: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				o, _ := obj.(*appsv1.ControllerRevision)
				*o = appsv1.ControllerRevision{
					Data: runtime.RawExtension{Object: &v1alpha2.Component{
						Spec: v1alpha2.ComponentSpec{
							Workload: runtime.RawExtension{
								Raw: cwRawWithWorkloadName,
							},
						}}}}
				return nil
			},
			expectResult: false,
			expectReason: fmt.Sprintf(reasonFmtWorkloadNameNotEmpty, workloadName),
		},
		{
			caseName: "Test validation fails for workload name assigned by parameter",
			appConfig: v1alpha2.ApplicationConfiguration{
				Spec: v1alpha2.ApplicationConfigurationSpec{
					Components: []v1alpha2.ApplicationConfigurationComponent{
						{
							RevisionName: revisionName,
							ParameterValues: []v1alpha2.ComponentParameterValue{
								{
									Name:  paramName,
									Value: intstr.FromString(paramValue),
								},
							},
						},
					},
				},
			},
			mockGetFunc: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				o, _ := obj.(*appsv1.ControllerRevision)
				*o = appsv1.ControllerRevision{
					Data: runtime.RawExtension{Object: &v1alpha2.Component{
						Spec: v1alpha2.ComponentSpec{
							Workload: runtime.RawExtension{
								Raw: cwRaw,
							},
							Parameters: []v1alpha2.ComponentParameter{
								{
									Name:       paramName,
									FieldPaths: []string{WorkloadNamePath},
								},
							},
						}}}}
				return nil
			},
			expectResult: false,
			expectReason: fmt.Sprintf(reasonFmtWorkloadNameNotEmpty, workloadName),
		},
		{
			caseName: "Test validation success",
			appConfig: v1alpha2.ApplicationConfiguration{
				Spec: v1alpha2.ApplicationConfigurationSpec{
					Components: []v1alpha2.ApplicationConfigurationComponent{
						{
							ComponentName: componentName,
							Traits: []v1alpha2.ComponentTrait{
								{
									Trait: runtime.RawExtension{
										Raw: msTraitRaw,
									},
								},
							},
						},
						{
							ComponentName: componentName,
						},
					},
				},
			},
			mockGetFunc: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				if o, ok := obj.(*v1alpha2.TraitDefinition); ok {
					*o = v1alpha2.TraitDefinition{
						Spec: v1alpha2.TraitDefinitionSpec{
							RevisionEnabled: true,
						},
					}
				}
				if o, ok := obj.(*v1alpha2.Component); ok {
					*o = v1alpha2.Component{
						Spec: v1alpha2.ComponentSpec{
							Workload: runtime.RawExtension{
								Raw: cwRaw,
							},
						},
					}
				}
				return nil
			},
			expectResult: true,
			expectReason: "",
		},
		{
			caseName: "Test checkVersionEnbled error occurs during validation",
			appConfig: v1alpha2.ApplicationConfiguration{
				Spec: v1alpha2.ApplicationConfigurationSpec{
					Components: []v1alpha2.ApplicationConfigurationComponent{
						{
							ComponentName: componentName,
							Traits: []v1alpha2.ComponentTrait{
								{
									Trait: runtime.RawExtension{
										Raw: msTraitRaw,
									},
								},
							},
						},
					},
				},
			},
			mockGetFunc: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				if _, ok := obj.(*v1alpha2.TraitDefinition); ok {
					return getErr
				}
				if o, ok := obj.(*v1alpha2.Component); ok {
					*o = v1alpha2.Component{
						Spec: v1alpha2.ComponentSpec{
							Workload: runtime.RawExtension{
								Raw: cwRaw,
							},
						},
					}
				}
				return nil
			},
			expectResult: false,
			expectReason: fmt.Sprintf(errFmtCheckWorkloadName, errors.Wrapf(getErr, errFmtGetTraitDefinition, version, kind, tName).Error()),
		},
		{
			caseName: "Test getComponent error occurs during validation",
			appConfig: v1alpha2.ApplicationConfiguration{
				Spec: v1alpha2.ApplicationConfigurationSpec{
					Components: []v1alpha2.ApplicationConfigurationComponent{
						{
							ComponentName: componentName,
							Traits: []v1alpha2.ComponentTrait{
								{
									Trait: runtime.RawExtension{
										Raw: msTraitRaw,
									},
								},
							},
						},
					},
				},
			},
			mockGetFunc: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				if o, ok := obj.(*v1alpha2.TraitDefinition); ok {
					*o = v1alpha2.TraitDefinition{
						Spec: v1alpha2.TraitDefinitionSpec{
							RevisionEnabled: true,
						},
					}
				}
				if _, ok := obj.(*v1alpha2.Component); ok {
					return getErr
				}
				return nil
			},
			expectResult: false,
			expectReason: "Error occurs when checking workload name. \"cannot get component \\\"c\\\": get error\"",
		},
		{
			caseName: "Test unmarshalWorkload error occurs during validation",
			appConfig: v1alpha2.ApplicationConfiguration{
				Spec: v1alpha2.ApplicationConfigurationSpec{
					Components: []v1alpha2.ApplicationConfigurationComponent{
						{
							ComponentName: componentName,
							Traits: []v1alpha2.ComponentTrait{
								{
									Trait: runtime.RawExtension{
										Raw: msTraitRaw,
									},
								},
							},
						},
					},
				},
			},
			mockGetFunc: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				if o, ok := obj.(*v1alpha2.TraitDefinition); ok {
					*o = v1alpha2.TraitDefinition{
						Spec: v1alpha2.TraitDefinitionSpec{
							RevisionEnabled: true,
						},
					}
				}
				if o, ok := obj.(*v1alpha2.Component); ok {
					*o = v1alpha2.Component{
						Spec: v1alpha2.ComponentSpec{
							Workload: runtime.RawExtension{},
						},
					}
				}
				return nil
			},
			expectResult: false,
			expectReason: "Error occurs when unmarshal workload of component \"\" error: \"unexpected end of JSON input\"",
		},
	}
	for _, tc := range tests {
		func(t *testing.T) {
			mockClient.MockGet = tc.mockGetFunc
			result, reason := checkWorkloadNameForVersioning(ctx, mockClient, mapper, &tc.appConfig)
			assert.Equal(t, tc.expectResult, result, fmt.Sprintf("Test case: %q", tc.caseName))
			assert.Equal(t, tc.expectReason, reason, fmt.Sprintf("Test case: %q", tc.caseName))
		}(t)
	}
}
