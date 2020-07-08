/*
Copyright 2020 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	htcp://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package applicationconfiguration

import (
	"context"
	"reflect"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	clientappv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

func TestRenderComponents(t *testing.T) {
	errBoom := errors.New("boom")

	namespace := "ns"
	acName := "coolappconfig"
	acUID := types.UID("definitely-a-uuid")
	componentName := "coolcomponent"
	workloadName := "coolworkload"
	traitName := "coolTrait"
	revisionName := "coolcomponent-aa1111"
	revisionName2 := "coolcomponent-bb2222"

	ac := &v1alpha2.ApplicationConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      acName,
			UID:       acUID,
		},
		Spec: v1alpha2.ApplicationConfigurationSpec{
			Components: []v1alpha2.ApplicationConfigurationComponent{
				{
					ComponentName: componentName,
					Traits:        []v1alpha2.ComponentTrait{{}},
				},
			},
		},
	}
	revAC := &v1alpha2.ApplicationConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      acName,
			UID:       acUID,
		},
		Spec: v1alpha2.ApplicationConfigurationSpec{
			Components: []v1alpha2.ApplicationConfigurationComponent{
				{
					RevisionName: revisionName,
					Traits:       []v1alpha2.ComponentTrait{{}},
				},
			},
		},
	}
	fakeAppClient := fake.NewSimpleClientset().AppsV1()
	fakeAppClient.ControllerRevisions(namespace).Create(context.Background(), &v1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{Name: revisionName, Namespace: namespace},
		Data: runtime.RawExtension{Object: &v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: namespace,
			},
			Spec:   v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &unstructured.Unstructured{}}},
			Status: v1alpha2.ComponentStatus{},
		}},
		Revision: 1,
	}, metav1.CreateOptions{})

	ref := metav1.NewControllerRef(ac, v1alpha2.ApplicationConfigurationGroupVersionKind)

	type fields struct {
		client    client.Reader
		appclient clientappv1.AppsV1Interface
		params    ParameterResolver
		workload  ResourceRenderer
		trait     ResourceRenderer
	}
	type args struct {
		ctx context.Context
		ac  *v1alpha2.ApplicationConfiguration
	}
	type want struct {
		w   []Workload
		err error
	}
	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"GetError": {
			reason: "An error getting a component should be returned",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(errBoom)},
			},
			args: args{ac: ac},
			want: want{
				err: errors.Wrapf(errBoom, errFmtGetComponent, componentName),
			},
		},
		"ResolveParamsError": {
			reason: "An error resolving the parameters of a component should be returned",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, errBoom
				}),
			},
			args: args{ac: ac},
			want: want{
				err: errors.Wrapf(errBoom, errFmtResolveParams, componentName),
			},
		},
		"RenderWorkloadError": {
			reason: "An error rendering a component's workload should be returned",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, nil
				}),
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					return nil, errBoom
				}),
			},
			args: args{ac: ac},
			want: want{
				err: errors.Wrapf(errBoom, errFmtRenderWorkload, componentName),
			},
		},
		"RenderTraitError": {
			reason: "An error rendering a component's traits should be returned",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, nil
				}),
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					return &unstructured.Unstructured{}, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					return nil, errBoom
				}),
			},
			args: args{ac: ac},
			want: want{
				err: errors.Wrapf(errBoom, errFmtRenderTrait, componentName),
			},
		},
		"Success": {
			reason: "One workload and one trait should successfully be rendered",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, nil
				}),
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					w := &unstructured.Unstructured{}
					w.SetName(workloadName)
					return w, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					t := &unstructured.Unstructured{}
					t.SetName(traitName)
					return t, nil
				}),
			},
			args: args{ac: ac},
			want: want{
				w: []Workload{
					{
						ComponentName: componentName,
						Workload: func() *unstructured.Unstructured {
							w := &unstructured.Unstructured{}
							w.SetNamespace(namespace)
							w.SetName(workloadName)
							w.SetOwnerReferences([]metav1.OwnerReference{*ref})
							return w
						}(),
						Traits: []unstructured.Unstructured{
							func() unstructured.Unstructured {
								t := &unstructured.Unstructured{}
								t.SetNamespace(namespace)
								t.SetName(traitName)
								t.SetOwnerReferences([]metav1.OwnerReference{*ref})
								return *t
							}(),
						},
						Scopes: []unstructured.Unstructured{},
					},
				},
			},
		},
		"Success-With-RevisionName": {
			reason: "Workload should successfully be rendered with fixed componentRevision",
			fields: fields{
				client:    &test.MockClient{MockGet: test.NewMockGetFn(nil)},
				appclient: fakeAppClient,
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, nil
				}),
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					w := &unstructured.Unstructured{}
					return w, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					t := &unstructured.Unstructured{}
					t.SetName(traitName)
					return t, nil
				}),
			},
			args: args{ac: revAC},
			want: want{
				w: []Workload{
					{
						ComponentName:         componentName,
						ComponentRevisionName: revisionName,
						Workload: func() *unstructured.Unstructured {
							w := &unstructured.Unstructured{}
							w.SetNamespace(namespace)
							w.SetName(componentName)
							w.SetOwnerReferences([]metav1.OwnerReference{*ref})
							return w
						}(),
						Traits: []unstructured.Unstructured{
							func() unstructured.Unstructured {
								t := &unstructured.Unstructured{}
								t.SetNamespace(namespace)
								t.SetName(traitName)
								t.SetOwnerReferences([]metav1.OwnerReference{*ref})
								return *t
							}(),
						},
						Scopes: []unstructured.Unstructured{},
					},
				},
			},
		},
		"Success-With-RevisionEnabledTrait": {
			reason: "Workload name should successfully be rendered with revisionName",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil, func(obj runtime.Object) error {
					switch robj := obj.(type) {
					case *v1alpha2.Component:
						ccomp := v1alpha2.Component{Status: v1alpha2.ComponentStatus{LatestRevision: &v1alpha2.Revision{Name: revisionName2}}}
						ccomp.DeepCopyInto(robj)
					case *v1alpha2.TraitDefinition:
						ttrait := v1alpha2.TraitDefinition{ObjectMeta: metav1.ObjectMeta{Name: traitName}, Spec: v1alpha2.TraitDefinitionSpec{RevisionEnabled: true}}
						ttrait.DeepCopyInto(robj)
					}
					return nil
				})},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, nil
				}),
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					w := &unstructured.Unstructured{}
					return w, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					t := &unstructured.Unstructured{}
					t.SetName(traitName)
					return t, nil
				}),
			},
			args: args{ac: ac},
			want: want{
				w: []Workload{
					{
						ComponentName:         componentName,
						ComponentRevisionName: revisionName2,
						Workload: func() *unstructured.Unstructured {
							w := &unstructured.Unstructured{}
							w.SetNamespace(namespace)
							w.SetName(revisionName2)
							w.SetOwnerReferences([]metav1.OwnerReference{*ref})
							return w
						}(),
						Traits: []unstructured.Unstructured{
							func() unstructured.Unstructured {
								t := &unstructured.Unstructured{}
								t.SetNamespace(namespace)
								t.SetName(traitName)
								t.SetOwnerReferences([]metav1.OwnerReference{*ref})
								return *t
							}(),
						},
						Scopes: []unstructured.Unstructured{},
					},
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := &components{tc.fields.client, tc.fields.appclient, tc.fields.params, tc.fields.workload, tc.fields.trait}
			got, err := r.Render(tc.args.ctx, tc.args.ac)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nr.Render(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.w, got); diff != "" {
				t.Errorf("\n%s\nr.Render(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestRenderWorkload(t *testing.T) {
	namespace := "ns"
	paramName := "coolparam"
	strVal := "coolstring"
	intVal := 32

	type args struct {
		data []byte
		p    []Parameter
	}
	type want struct {
		workload *unstructured.Unstructured
		err      error
	}
	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"UnmarshalError": {
			reason: "Errors unmarshalling JSON should be returned",
			args: args{
				data: []byte(`wat`),
			},
			want: want{
				err: errors.Wrapf(errors.New("invalid character 'w' looking for beginning of value"), errUnmarshalWorkload),
			},
		},
		"SetStringError": {
			reason: "Errors setting a string value should be returned",
			args: args{
				data: []byte(`{"metadata":{}}`),
				p: []Parameter{{
					Name:       paramName,
					Value:      intstr.FromString(strVal),
					FieldPaths: []string{"metadata[0]"},
				}},
			},
			want: want{
				err: errors.Wrapf(errors.New("metadata is not an array"), errFmtSetParam, paramName),
			},
		},
		"SetNumberError": {
			reason: "Errors setting a number value should be returned",
			args: args{
				data: []byte(`{"metadata":{}}`),
				p: []Parameter{{
					Name:       paramName,
					Value:      intstr.FromInt(intVal),
					FieldPaths: []string{"metadata[0]"},
				}},
			},
			want: want{
				err: errors.Wrapf(errors.New("metadata is not an array"), errFmtSetParam, paramName),
			},
		},
		"Success": {
			reason: "A workload should be returned with the supplied parameters set",
			args: args{
				data: []byte(`{"metadata":{"namespace":"` + namespace + `","name":"name"}}`),
				p: []Parameter{{
					Name:       paramName,
					Value:      intstr.FromString(strVal),
					FieldPaths: []string{"metadata.name"},
				}},
			},
			want: want{
				workload: func() *unstructured.Unstructured {
					w := &unstructured.Unstructured{}
					w.SetNamespace(namespace)
					w.SetName(strVal)
					return w
				}(),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := renderWorkload(tc.args.data, tc.args.p...)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nrenderWorkload(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.workload, got); diff != "" {
				t.Errorf("\n%s\nrenderWorkload(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestRenderTrait(t *testing.T) {
	apiVersion := "coolversion"
	kind := "coolkind"

	type args struct {
		data []byte
	}
	type want struct {
		workload *unstructured.Unstructured
		err      error
	}
	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"UnmarshalError": {
			reason: "Errors unmarshalling JSON should be returned",
			args: args{
				data: []byte(`wat`),
			},
			want: want{
				err: errors.Wrapf(errors.New("invalid character 'w' looking for beginning of value"), errUnmarshalTrait),
			},
		},
		"Success": {
			reason: "A workload should be returned with the supplied parameters set",
			args: args{
				data: []byte(`{"apiVersion":"` + apiVersion + `","kind":"` + kind + `"}`),
			},
			want: want{
				workload: func() *unstructured.Unstructured {
					w := &unstructured.Unstructured{}
					w.SetAPIVersion(apiVersion)
					w.SetKind(kind)
					return w
				}(),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := renderTrait(tc.args.data)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nrenderTrait(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.workload, got); diff != "" {
				t.Errorf("\n%s\nrenderTrait(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestResolveParams(t *testing.T) {
	paramName := "coolparam"
	required := true
	paths := []string{"metadata.name"}
	value := "cool"

	type args struct {
		cp  []v1alpha2.ComponentParameter
		cpv []v1alpha2.ComponentParameterValue
	}
	type want struct {
		p   []Parameter
		err error
	}
	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"MissingRequired": {
			reason: "An error should be returned when a required parameter is omitted",
			args: args{
				cp: []v1alpha2.ComponentParameter{
					{
						Name:     paramName,
						Required: &required,
					},
				},
				cpv: []v1alpha2.ComponentParameterValue{},
			},
			want: want{
				err: errors.Errorf(errFmtRequiredParam, paramName),
			},
		},
		"Unsupported": {
			reason: "An error should be returned when an unsupported parameter value is supplied",
			args: args{
				cp: []v1alpha2.ComponentParameter{},
				cpv: []v1alpha2.ComponentParameterValue{
					{
						Name: paramName,
					},
				},
			},
			want: want{
				err: errors.Errorf(errFmtUnsupportedParam, paramName),
			},
		},
		"MissingNotRequired": {
			reason: "Nothing should be returned when an optional parameter is omitted",
			args: args{
				cp: []v1alpha2.ComponentParameter{
					{
						Name: paramName,
					},
				},
				cpv: []v1alpha2.ComponentParameterValue{},
			},
			want: want{},
		},
		"SupportedAndSet": {
			reason: "A parameter should be returned when it is supported and set",
			args: args{
				cp: []v1alpha2.ComponentParameter{
					{
						Name:       paramName,
						FieldPaths: paths,
					},
				},
				cpv: []v1alpha2.ComponentParameterValue{
					{
						Name:  paramName,
						Value: intstr.FromString(value),
					},
				},
			},
			want: want{
				p: []Parameter{
					{
						Name:       paramName,
						FieldPaths: paths,
						Value:      intstr.FromString(value),
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := resolve(tc.args.cp, tc.args.cpv)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nresolve(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.p, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("\n%s\nresolve(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestRenderTraitWithoutMetadataName(t *testing.T) {
	namespace := "ns"
	acName := "coolappconfig"
	acUID := types.UID("definitely-a-uuid")
	componentName := "coolcomponent"
	workloadName := "coolworkload"

	ac := &v1alpha2.ApplicationConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      acName,
			UID:       acUID,
		},
		Spec: v1alpha2.ApplicationConfigurationSpec{
			Components: []v1alpha2.ApplicationConfigurationComponent{
				{
					ComponentName: componentName,
					Traits:        []v1alpha2.ComponentTrait{{}},
				},
			},
		},
	}

	ref := metav1.NewControllerRef(ac, v1alpha2.ApplicationConfigurationGroupVersionKind)

	type fields struct {
		client   client.Reader
		params   ParameterResolver
		workload ResourceRenderer
		trait    ResourceRenderer
	}
	type args struct {
		ctx context.Context
		ac  *v1alpha2.ApplicationConfiguration
	}
	type want struct {
		w []Workload
		// err error
	}
	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"Success": {
			reason: "One workload and one trait should successfully be rendered",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, nil
				}),
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					w := &unstructured.Unstructured{}
					w.SetName(workloadName)
					return w, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					t := &unstructured.Unstructured{}
					return t, nil
				}),
			},
			args: args{ac: ac},
			want: want{
				w: []Workload{
					{
						ComponentName: componentName,
						Workload: func() *unstructured.Unstructured {
							w := &unstructured.Unstructured{}
							w.SetNamespace(namespace)
							w.SetName(workloadName)
							w.SetOwnerReferences([]metav1.OwnerReference{*ref})
							return w
						}(),
						Traits: []unstructured.Unstructured{
							func() unstructured.Unstructured {
								t := &unstructured.Unstructured{}
								t.SetNamespace(namespace)
								t.SetOwnerReferences([]metav1.OwnerReference{*ref})
								return *t
							}(),
						},
					},
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := &components{tc.fields.client, nil, tc.fields.params, tc.fields.workload, tc.fields.trait}
			got, _ := r.Render(tc.args.ctx, tc.args.ac)
			if len(got) == 0 || len(got[0].Traits) == 0 || got[0].Traits[0].GetName() != componentName {
				t.Errorf("\n%s\nr.Render(...): -want error, +got error:\n%s\n", tc.reason, "Trait name is NOT"+
					"automatically set.")
			}
		})
	}
}

func TestGetCRDName(t *testing.T) {
	tests := map[string]struct {
		u      *unstructured.Unstructured
		exp    string
		reason string
	}{
		"native resource": {
			u: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
			}},
			exp:    "deployments.apps",
			reason: "native resource match",
		},
		"extended resource": {
			u: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "extend.oam.dev/v1alpha2",
				"kind":       "SimpleRolloutTrait",
			}},
			exp:    "simplerollouttraits.extend.oam.dev",
			reason: "extend resource match",
		},
	}
	for name, ti := range tests {
		t.Run(name, func(t *testing.T) {
			got := util.GetCRDName(ti.u)
			if got != ti.exp {
				t.Errorf("%s getCRDName want %s got %s ", ti.reason, ti.exp, got)
			}
		})
	}
}

func TestSetWorkloadInstanceName(t *testing.T) {
	tests := map[string]struct {
		traitDefs []v1alpha2.TraitDefinition
		u         *unstructured.Unstructured
		c         *v1alpha2.Component
		exp       *unstructured.Unstructured
		expErr    error
		reason    string
	}{
		"with a name, no change": {
			u: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "myname",
				},
			}},
			c: &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp"}, Status: v1alpha2.ComponentStatus{LatestRevision: &v1alpha2.Revision{Name: "rev-1"}}},
			exp: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "myname",
				},
			}},
			reason: "workloadName should not change if already set",
		},
		"revisionEnabled true, set revisionName": {
			traitDefs: []v1alpha2.TraitDefinition{
				{
					Spec: v1alpha2.TraitDefinitionSpec{RevisionEnabled: true},
				},
			},
			u: &unstructured.Unstructured{Object: map[string]interface{}{}},
			c: &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp"}, Status: v1alpha2.ComponentStatus{LatestRevision: &v1alpha2.Revision{Name: "rev-1"}}},
			exp: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "rev-1",
				},
			}},
			reason: "workloadName should align with revisionName",
		},
		"revisionEnabled false, set componentName": {
			traitDefs: []v1alpha2.TraitDefinition{
				{
					Spec: v1alpha2.TraitDefinitionSpec{RevisionEnabled: false},
				},
			},
			u: &unstructured.Unstructured{Object: map[string]interface{}{}},
			c: &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp"}, Status: v1alpha2.ComponentStatus{LatestRevision: &v1alpha2.Revision{Name: "rev-1"}}},
			exp: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "comp",
				},
			}},
			reason: "workloadName should align with componentName",
		},
		"set value error": {
			u: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": []string{},
			}},
			c:      &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp"}, Status: v1alpha2.ComponentStatus{LatestRevision: &v1alpha2.Revision{Name: "rev-1"}}},
			expErr: errors.Wrapf(errors.New("metadata is not an object"), errSetValueForField, instanceNamePath, "comp"),
		},
		"set value error for trait revisionEnabled": {
			u: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": []string{},
			}},
			traitDefs: []v1alpha2.TraitDefinition{
				{
					Spec: v1alpha2.TraitDefinitionSpec{RevisionEnabled: false},
				},
			},
			c:      &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp"}, Status: v1alpha2.ComponentStatus{LatestRevision: &v1alpha2.Revision{Name: "rev-1"}}},
			expErr: errors.Wrapf(errors.New("metadata is not an object"), errSetValueForField, instanceNamePath, "comp"),
		},
	}
	for name, ti := range tests {
		t.Run(name, func(t *testing.T) {
			if ti.expErr != nil {
				assert.Equal(t, ti.expErr.Error(), SetWorkloadInstanceName(ti.traitDefs, ti.u, ti.c).Error())
			} else {
				err := SetWorkloadInstanceName(ti.traitDefs, ti.u, ti.c)
				assert.NoError(t, err)
				assert.Equal(t, ti.exp, ti.u, ti.reason)
			}
		})
	}
}

func TestSetTraitProperties(t *testing.T) {
	u := &unstructured.Unstructured{}
	u.SetName("hasName")
	setTraitProperties(u, "comp1", "ns", &metav1.OwnerReference{Name: "comp1"})
	expU := &unstructured.Unstructured{}
	expU.SetName("hasName")
	expU.SetNamespace("ns")
	expU.SetOwnerReferences([]metav1.OwnerReference{{Name: "comp1"}})
	assert.Equal(t, expU, u)

	u = &unstructured.Unstructured{}
	setTraitProperties(u, "comp1", "ns", &metav1.OwnerReference{Name: "comp1"})
	expU = &unstructured.Unstructured{}
	expU.SetName("comp1")
	expU.SetNamespace("ns")
	expU.SetOwnerReferences([]metav1.OwnerReference{{Name: "comp1"}})
}

func TestGetComponent(t *testing.T) {
	type Fields struct {
		client    client.Reader
		appclient clientappv1.AppsV1Interface
		params    ParameterResolver
		workload  ResourceRenderer
		trait     ResourceRenderer
	}
	namespace := "ns"
	componentName := "newcomponent"
	revisionName := "newcomponent-aa1111"
	revisionName2 := "newcomponent-bb1111"

	fakeAppClient := fake.NewSimpleClientset().AppsV1()
	fakeAppClient.ControllerRevisions(namespace).Create(context.Background(), &v1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{Name: revisionName, Namespace: namespace},
		Data: runtime.RawExtension{Object: &v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: namespace,
			},
			Spec:   v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &unstructured.Unstructured{}}},
			Status: v1alpha2.ComponentStatus{},
		}},
		Revision: 1,
	}, metav1.CreateOptions{})
	var fields = Fields{
		client: &test.MockClient{MockGet: test.NewMockGetFn(nil, func(obj runtime.Object) error {
			objc, ok := obj.(*v1alpha2.Component)
			if !ok {
				return nil
			}

			c := &v1alpha2.Component{
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
			c.DeepCopyInto(objc)
			return nil
		})},
		appclient: fakeAppClient,
		params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
			return nil, nil
		}),
		workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
			w := &unstructured.Unstructured{}
			return w, nil
		}),
	}
	r := &components{fields.client, fields.appclient, fields.params, fields.workload, fields.trait}
	c, revision, err := r.getComponent(context.Background(), v1alpha2.ApplicationConfigurationComponent{RevisionName: revisionName}, namespace)
	assert.NoError(t, err)
	assert.Equal(t, revisionName, revision)
	assert.Equal(t, &v1alpha2.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: namespace,
		},
		Spec:   v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &unstructured.Unstructured{}}},
		Status: v1alpha2.ComponentStatus{},
	}, c)

	c, revision, err = r.getComponent(context.Background(), v1alpha2.ApplicationConfigurationComponent{ComponentName: componentName}, namespace)
	assert.NoError(t, err)
	assert.Equal(t, revisionName2, revision)
	assert.Equal(t, &v1alpha2.Component{
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
	}, c)
}

func TestPassThroughObjMeta(t *testing.T) {
	c := components{}
	ac := &v1alpha2.ApplicationConfiguration{}

	labels := map[string]string{
		"core.oam.dev/ns":         "oam-system",
		"core.oam.dev/controller": "oam-kubernetes-runtime",
	}

	annotation := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	ac.SetLabels(labels)
	ac.SetAnnotations(annotation)

	t.Run("workload and trait have no labels and annotation", func(t *testing.T) {
		var u unstructured.Unstructured
		c.passThroughObjMeta(ac.ObjectMeta, &u)
		got := u.GetLabels()
		want := labels
		if !reflect.DeepEqual(want, got) {
			t.Errorf("labels want:%v,got:%v", want, got)
		}
		gotAnnotation := u.GetAnnotations()
		wantAnnotation := annotation
		if !reflect.DeepEqual(want, got) {
			t.Errorf("annotation want:%v,got:%v", wantAnnotation, gotAnnotation)
		}
	})

	t.Run("workload and trait contains overlapping keys", func(t *testing.T) {
		var u unstructured.Unstructured
		existAnnotation := map[string]string{
			"key1": "exist value1",
			"key3": "value3",
		}
		existLabels := map[string]string{
			"core.oam.dev/ns":          "kube-system",
			"core.oam.dev/kube-native": "deployment",
		}
		u.SetLabels(existLabels)
		u.SetAnnotations(existAnnotation)

		c.passThroughObjMeta(ac.ObjectMeta, &u)

		gotAnnotation := u.GetAnnotations()
		wantAnnotation := map[string]string{
			"key1": "exist value1",
			"key2": "value2",
			"key3": "value3",
		}
		if !reflect.DeepEqual(gotAnnotation, wantAnnotation) {
			t.Errorf("annotation got:%v,want:%v", gotAnnotation, wantAnnotation)
		}

		gotLabels := u.GetLabels()
		wantLabels := map[string]string{
			"core.oam.dev/ns":          "kube-system",
			"core.oam.dev/kube-native": "deployment",
			"core.oam.dev/controller":  "oam-kubernetes-runtime",
		}
		if !reflect.DeepEqual(gotLabels, wantLabels) {
			t.Errorf("labels got:%v,want:%v", gotLabels, wantLabels)
		}
		if !reflect.DeepEqual(gotLabels, wantLabels) {
			t.Errorf("labels got:%v,want:%v", gotLabels, wantLabels)
		}
	})
}
