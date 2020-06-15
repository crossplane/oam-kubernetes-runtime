/*
Copyright 2020 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package applicationconfiguration

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

func TestApplyWorkloads(t *testing.T) {
	errBoom := errors.New("boom")

	namespace := "ns"

	workload := &unstructured.Unstructured{}
	workload.SetAPIVersion("v")
	workload.SetKind("workload")
	workload.SetNamespace(namespace)
	workload.SetName("workload")
	workload.SetUID(types.UID("workload-uid"))

	trait := &unstructured.Unstructured{}
	trait.SetAPIVersion("v")
	trait.SetKind("trait")
	trait.SetNamespace(namespace)
	trait.SetName("trait")
	trait.SetUID(types.UID("trait-uid"))

	scope := &unstructured.Unstructured{}
	scope.SetAPIVersion("v")
	scope.SetKind("scope")
	scope.SetNamespace(namespace)
	scope.SetName("scope")

	type args struct {
		ctx context.Context
		ws  []v1alpha2.WorkloadStatus
		w   []Workload
	}

	cases := map[string]struct {
		reason    string
		client    resource.Applicator
		rawClient client.Client
		args      args
		want      error
	}{
		"ApplyWorkloadError": {
			reason: "Errors applying a workload should be reflected as a status condition",
			client: resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error {
				if w, ok := o.(*unstructured.Unstructured); ok && w.GetUID() == workload.GetUID() {
					return errBoom
				}
				return nil
			}),
			rawClient: nil,
			args:      args{w: []Workload{{Workload: workload}}, ws: []v1alpha2.WorkloadStatus{}},
			want:      errors.Wrapf(errBoom, errFmtApplyWorkload, workload.GetName()),
		},
		"ApplyTraitError": {
			reason: "Errors applying a trait should be reflected as a status condition",
			client: resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error {
				if t, ok := o.(*unstructured.Unstructured); ok && t.GetUID() == trait.GetUID() {
					return errBoom
				}
				return nil
			}),
			rawClient: nil,
			args: args{
				w:  []Workload{{Workload: workload, Traits: []unstructured.Unstructured{*trait}}},
				ws: []v1alpha2.WorkloadStatus{}},
			want: errors.Wrapf(errBoom, errFmtApplyTrait, trait.GetKind(), trait.GetName()),
		},
		"Success": {
			reason:    "Applied workloads and traits should be returned as a set of UIDs.",
			client:    resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error { return nil }),
			rawClient: nil,
			args: args{
				w:  []Workload{{Workload: workload, Traits: []unstructured.Unstructured{*trait}}},
				ws: []v1alpha2.WorkloadStatus{},
			},
		},
		"SuccessWithScope": {
			reason: "Applied workloads refs to scopes.",
			client: resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error { return nil }),
			rawClient: &test.MockClient{
				MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
					return nil
				},
				MockUpdate: func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
					return nil
				},
			},
			args: args{
				w: []Workload{{
					Workload: workload,
					Traits:   []unstructured.Unstructured{*trait},
					Scopes:   []unstructured.Unstructured{*scope},
				}},
				ws: []v1alpha2.WorkloadStatus{
					{
						Reference: v1alpha1.TypedReference{
							APIVersion: workload.GetAPIVersion(),
							Kind:       workload.GetKind(),
							Name:       workload.GetName(),
						},
						Scopes: []v1alpha2.WorkloadScope{
							{
								Reference: v1alpha1.TypedReference{
									APIVersion: scope.GetAPIVersion(),
									Kind:       scope.GetKind(),
									Name:       scope.GetName(),
								},
							},
						},
					},
				},
			},
		},
		"SuccessWithScopeNoOp": {
			reason: "Scope already has workloadRef.",
			client: resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error { return nil }),
			rawClient: &test.MockClient{
				MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
					if key.Name == scope.GetName() {
						scope := obj.(*unstructured.Unstructured)

						refs := []interface{}{
							map[string]interface{}{
								"apiVersion": workload.GetAPIVersion(),
								"kind":       workload.GetKind(),
								"name":       workload.GetName(),
							},
						}

						if err := fieldpath.Pave(scope.UnstructuredContent()).SetValue("spec.workloadRefs", refs); err == nil {
							return err
						}

						return nil
					}

					return nil
				},
				MockUpdate: func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
					return fmt.Errorf("update is not expected in this test")
				},
			},
			args: args{
				w: []Workload{{
					Workload: workload,
					Traits:   []unstructured.Unstructured{*trait},
					Scopes:   []unstructured.Unstructured{*scope},
				}},
				ws: []v1alpha2.WorkloadStatus{
					{
						Reference: v1alpha1.TypedReference{
							APIVersion: workload.GetAPIVersion(),
							Kind:       workload.GetKind(),
							Name:       workload.GetName(),
						},
						Scopes: []v1alpha2.WorkloadScope{
							{
								Reference: v1alpha1.TypedReference{
									APIVersion: scope.GetAPIVersion(),
									Kind:       scope.GetKind(),
									Name:       scope.GetName(),
								},
							},
						},
					},
				},
			},
		},
		"SuccessRemoving": {
			reason: "Removes workload refs from scopes.",
			client: resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error { return nil }),
			rawClient: &test.MockClient{
				MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
					if key.Name == scope.GetName() {
						scope := obj.(*unstructured.Unstructured)

						refs := []interface{}{
							map[string]interface{}{
								"apiVersion": workload.GetAPIVersion(),
								"kind":       workload.GetKind(),
								"name":       workload.GetName(),
							},
						}

						if err := fieldpath.Pave(scope.UnstructuredContent()).SetValue("spec.workloadRefs", refs); err == nil {
							return err
						}

						return nil
					}

					return nil
				},
				MockUpdate: func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
					return nil
				},
			},
			args: args{
				w: []Workload{{
					Workload: workload,
					Traits:   []unstructured.Unstructured{*trait},
					Scopes:   []unstructured.Unstructured{},
				}},
				ws: []v1alpha2.WorkloadStatus{
					{
						Reference: v1alpha1.TypedReference{
							APIVersion: workload.GetAPIVersion(),
							Kind:       workload.GetKind(),
							Name:       workload.GetName(),
						},
						Scopes: []v1alpha2.WorkloadScope{
							{
								Reference: v1alpha1.TypedReference{
									APIVersion: scope.GetAPIVersion(),
									Kind:       scope.GetKind(),
									Name:       scope.GetName(),
								},
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			a := workloads{client: tc.client, rawClient: tc.rawClient}
			err := a.Apply(tc.args.ctx, "", tc.args.ws, tc.args.w)

			if diff := cmp.Diff(tc.want, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\na.Apply(...): -want error, +got error:\n%s", tc.reason, diff)
			}
		})
	}
}
