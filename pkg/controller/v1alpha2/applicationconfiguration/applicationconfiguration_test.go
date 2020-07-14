/*
Copyright 2019 The Crossplane Authors.

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
	"encoding/json"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/mock"
)

type acParam func(*v1alpha2.ApplicationConfiguration)

func withConditions(c ...runtimev1alpha1.Condition) acParam {
	return func(ac *v1alpha2.ApplicationConfiguration) {
		ac.SetConditions(c...)
	}
}

func withWorkloadStatuses(ws ...v1alpha2.WorkloadStatus) acParam {
	return func(ac *v1alpha2.ApplicationConfiguration) {
		ac.Status.Workloads = ws
	}
}

func withDependencyStatus(s v1alpha2.DependencyStatus) acParam {
	return func(ac *v1alpha2.ApplicationConfiguration) {
		ac.Status.Dependency = s
	}
}

func ac(p ...acParam) *v1alpha2.ApplicationConfiguration {
	ac := &v1alpha2.ApplicationConfiguration{}
	for _, fn := range p {
		fn(ac)
	}
	return ac
}

func TestReconciler(t *testing.T) {
	errBoom := errors.New("boom")
	errUnexpectedStatus := errors.New("unexpected status")

	namespace := "ns"
	componentName := "coolcomponent"

	workload := &unstructured.Unstructured{}
	workload.SetAPIVersion("v")
	workload.SetKind("workload")
	workload.SetNamespace(namespace)
	workload.SetName("workload")

	trait := &unstructured.Unstructured{}
	trait.SetAPIVersion("v")
	trait.SetKind("trait")
	trait.SetNamespace(namespace)
	trait.SetName("trait")

	depStatus := v1alpha2.DependencyStatus{
		Unsatisfied: []v1alpha2.UnstaifiedDependency{{
			From: v1alpha2.DependencyFromObject{
				TypedReference: runtimev1alpha1.TypedReference{
					APIVersion: workload.GetAPIVersion(),
					Kind:       workload.GetKind(),
					Name:       workload.GetName(),
				},
				FieldPath: "status.key",
			},
			To: v1alpha2.DependencyToObject{
				TypedReference: runtimev1alpha1.TypedReference{
					APIVersion: workload.GetAPIVersion(),
					Kind:       workload.GetKind(),
					Name:       workload.GetName(),
				},
				FieldPaths: []string{"spec.key"},
			},
		}},
	}

	type args struct {
		m manager.Manager
		o []ReconcilerOption
	}
	type want struct {
		result reconcile.Result
		err    error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"GetApplicationConfigurationError": {
			reason: "Errors getting the ApplicationConfiguration under reconciliation should be returned",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet: test.NewMockGetFn(errBoom),
					},
				},
			},
			want: want{
				err: errors.Wrap(errBoom, errGetAppConfig),
			},
		},
		"RenderComponentsError": {
			reason: "Errors rendering components should be reflected as a status condition",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil),
						MockStatusUpdate: test.NewMockStatusUpdateFn(nil, func(o runtime.Object) error {

							want := ac(withConditions(runtimev1alpha1.ReconcileError(errors.Wrap(errBoom, errRenderComponents))))
							if diff := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration)); diff != "" {
								t.Errorf("\nclient.Status().Update(): -want, +got:\n%s", diff)
								return errUnexpectedStatus
							}

							return nil
						}),
					},
				},
				o: []ReconcilerOption{
					WithRenderer(ComponentRenderFn(func(_ context.Context, _ *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error) {
						return nil, &v1alpha2.DependencyStatus{}, errBoom
					})),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: shortWait},
			},
		},
		"ApplyComponentsError": {
			reason: "Errors applying components should be reflected as a status condition",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil),
						MockStatusUpdate: test.NewMockStatusUpdateFn(nil, func(o runtime.Object) error {
							want := ac(withConditions(runtimev1alpha1.ReconcileError(errors.Wrap(errBoom, errApplyComponents))))
							if diff := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration)); diff != "" {
								t.Errorf("\nclient.Status().Update(): -want, +got:\n%s", diff)
								return errUnexpectedStatus
							}
							return nil
						}),
					},
				},
				o: []ReconcilerOption{
					WithRenderer(ComponentRenderFn(func(_ context.Context, _ *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error) {
						return []Workload{{Workload: workload}}, &v1alpha2.DependencyStatus{}, nil
					})),
					WithApplicator(WorkloadApplyFn(func(_ context.Context, _ []v1alpha2.WorkloadStatus, _ []Workload, _ ...resource.ApplyOption) error {
						return errBoom
					})),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: shortWait},
			},
		},
		"GCDeleteError": {
			reason: "Errors deleting a garbage collected resource should be reflected as a status condition",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet:    test.NewMockGetFn(nil),
						MockDelete: test.NewMockDeleteFn(errBoom),
						MockStatusUpdate: test.NewMockStatusUpdateFn(nil, func(o runtime.Object) error {
							want := ac(withConditions(runtimev1alpha1.ReconcileError(errors.Wrap(errBoom, errGCComponent))))
							if diff := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration)); diff != "" {
								t.Errorf("\nclient.Status().Update(): -want, +got:\n%s", diff)
								return errUnexpectedStatus
							}
							return nil
						}),
					},
				},
				o: []ReconcilerOption{
					WithRenderer(ComponentRenderFn(func(_ context.Context, _ *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error) {
						return []Workload{}, &v1alpha2.DependencyStatus{}, nil
					})),
					WithApplicator(WorkloadApplyFn(func(_ context.Context, _ []v1alpha2.WorkloadStatus, _ []Workload, _ ...resource.ApplyOption) error {
						return nil
					})),
					WithGarbageCollector(GarbageCollectorFn(func(_ string, _ []v1alpha2.WorkloadStatus, _ []Workload) []unstructured.Unstructured {
						return []unstructured.Unstructured{*workload}
					})),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: shortWait},
			},
		},
		"Has dependency": {
			reason: "dependency should be reflected in status and wait time should align",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet:    test.NewMockGetFn(nil),
						MockDelete: test.NewMockDeleteFn(nil),
						MockStatusUpdate: test.NewMockStatusUpdateFn(nil, func(o runtime.Object) error {
							want := ac(
								withConditions(runtimev1alpha1.ReconcileSuccess()),
								withWorkloadStatuses(v1alpha2.WorkloadStatus{
									ComponentName: componentName,
									Reference: runtimev1alpha1.TypedReference{
										APIVersion: workload.GetAPIVersion(),
										Kind:       workload.GetKind(),
										Name:       workload.GetName(),
									},
								}),
								withDependencyStatus(depStatus),
							)
							if diff := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration), cmpopts.EquateEmpty()); diff != "" {
								t.Errorf("\nclient.Status().Update(): -want, +got:\n%s", diff)
								return errUnexpectedStatus
							}
							return nil
						}),
					},
				},
				o: []ReconcilerOption{
					WithRenderer(ComponentRenderFn(func(_ context.Context, _ *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error) {
						return []Workload{{ComponentName: componentName, Workload: workload}}, &depStatus, nil
					})),
					WithApplicator(WorkloadApplyFn(func(_ context.Context, _ []v1alpha2.WorkloadStatus, _ []Workload, _ ...resource.ApplyOption) error {
						return nil
					})),
					WithGarbageCollector(GarbageCollectorFn(func(_ string, _ []v1alpha2.WorkloadStatus, _ []Workload) []unstructured.Unstructured {
						return []unstructured.Unstructured{*trait}
					})),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: dependCheckWait},
			},
		},
		"Success": {
			reason: "Rendered workloads should be reflected in status",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet:    test.NewMockGetFn(nil),
						MockDelete: test.NewMockDeleteFn(nil),
						MockStatusUpdate: test.NewMockStatusUpdateFn(nil, func(o runtime.Object) error {
							want := ac(
								withConditions(runtimev1alpha1.ReconcileSuccess()),
								withWorkloadStatuses(v1alpha2.WorkloadStatus{
									ComponentName: componentName,
									Reference: runtimev1alpha1.TypedReference{
										APIVersion: workload.GetAPIVersion(),
										Kind:       workload.GetKind(),
										Name:       workload.GetName(),
									},
								}),
							)
							if diff := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration), cmpopts.EquateEmpty()); diff != "" {
								t.Errorf("\nclient.Status().Update(): -want, +got:\n%s", diff)
								return errUnexpectedStatus
							}
							return nil
						}),
					},
				},
				o: []ReconcilerOption{
					WithRenderer(ComponentRenderFn(func(_ context.Context, _ *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error) {
						return []Workload{{ComponentName: componentName, Workload: workload}}, &v1alpha2.DependencyStatus{}, nil
					})),
					WithApplicator(WorkloadApplyFn(func(_ context.Context, _ []v1alpha2.WorkloadStatus, _ []Workload, _ ...resource.ApplyOption) error {
						return nil
					})),
					WithGarbageCollector(GarbageCollectorFn(func(_ string, _ []v1alpha2.WorkloadStatus, _ []Workload) []unstructured.Unstructured {
						return []unstructured.Unstructured{*trait}
					})),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: longWait},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := NewReconciler(tc.args.m, tc.args.o...)
			got, err := r.Reconcile(reconcile.Request{})

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nr.Reconcile(...): -want error, +got error:\n%s", tc.reason, diff)
			}

			if diff := cmp.Diff(tc.want.result, got); diff != "" {
				t.Errorf("\n%s\nr.Reconcile(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestWorkloadStatus(t *testing.T) {
	namespace := "ns"
	componentName := "coolcomponent"

	workload := &unstructured.Unstructured{}
	workload.SetAPIVersion("v")
	workload.SetKind("workload")
	workload.SetNamespace(namespace)
	workload.SetName("workload")

	trait := &unstructured.Unstructured{}
	trait.SetAPIVersion("v")
	trait.SetKind("trait")
	trait.SetNamespace(namespace)
	trait.SetName("trait")

	cases := map[string]struct {
		w    Workload
		want v1alpha2.WorkloadStatus
	}{
		"Success": {
			w: Workload{
				ComponentName: componentName,
				Workload:      workload,
				Traits:        []*Trait{{Object: *trait}},
			},
			want: v1alpha2.WorkloadStatus{
				ComponentName: componentName,
				Reference: runtimev1alpha1.TypedReference{
					APIVersion: workload.GetAPIVersion(),
					Kind:       workload.GetKind(),
					Name:       workload.GetName(),
				},
				Traits: []v1alpha2.WorkloadTrait{
					{
						Reference: runtimev1alpha1.TypedReference{
							APIVersion: trait.GetAPIVersion(),
							Kind:       trait.GetKind(),
							Name:       trait.GetName(),
						},
					},
				},
				Scopes: []v1alpha2.WorkloadScope{},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := tc.w.Status()
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("\nw.Status(): -want, +got:\n%s\n", diff)
			}
		})
	}

}

func TestEligible(t *testing.T) {
	namespace := "ns"

	workload := &unstructured.Unstructured{}
	workload.SetAPIVersion("v")
	workload.SetKind("workload")
	workload.SetNamespace(namespace)
	workload.SetName("workload")

	trait := &unstructured.Unstructured{}
	trait.SetAPIVersion("v")
	trait.SetKind("trait")
	trait.SetNamespace(namespace)
	trait.SetName("trait")

	type args struct {
		namespace string
		ws        []v1alpha2.WorkloadStatus
		w         []Workload
	}
	cases := map[string]struct {
		reason string
		args   args
		want   []unstructured.Unstructured
	}{
		"TraitNotApplied": {
			reason: "A referenced trait is eligible for garbage collection if it was not applied",
			args: args{
				namespace: namespace,
				ws: []v1alpha2.WorkloadStatus{
					{
						Reference: runtimev1alpha1.TypedReference{
							APIVersion: workload.GetAPIVersion(),
							Kind:       workload.GetKind(),
							Name:       workload.GetName(),
						},
						Traits: []v1alpha2.WorkloadTrait{
							{
								Reference: runtimev1alpha1.TypedReference{
									APIVersion: trait.GetAPIVersion(),
									Kind:       trait.GetKind(),
									Name:       trait.GetName(),
								},
							},
						},
					},
				},
				w: []Workload{{Workload: workload}},
			},
			want: []unstructured.Unstructured{*trait},
		},
		"NeitherApplied": {
			reason: "A referenced workload and its trait is eligible for garbage collection if they were not applied",
			args: args{
				namespace: namespace,
				ws: []v1alpha2.WorkloadStatus{
					{
						Reference: runtimev1alpha1.TypedReference{
							APIVersion: workload.GetAPIVersion(),
							Kind:       workload.GetKind(),
							Name:       workload.GetName(),
						},
						Traits: []v1alpha2.WorkloadTrait{
							{
								Reference: runtimev1alpha1.TypedReference{
									APIVersion: trait.GetAPIVersion(),
									Kind:       trait.GetKind(),
									Name:       trait.GetName(),
								},
							},
						},
					},
				},
			},
			want: []unstructured.Unstructured{*workload, *trait},
		},
		"BothApplied": {
			reason: "A referenced workload and its trait are not eligible for garbage collection if they were applied",
			args: args{
				namespace: namespace,
				ws: []v1alpha2.WorkloadStatus{
					{
						Reference: runtimev1alpha1.TypedReference{
							APIVersion: workload.GetAPIVersion(),
							Kind:       workload.GetKind(),
							Name:       workload.GetName(),
						},
						Traits: []v1alpha2.WorkloadTrait{
							{
								Reference: runtimev1alpha1.TypedReference{
									APIVersion: trait.GetAPIVersion(),
									Kind:       trait.GetKind(),
									Name:       trait.GetName(),
								},
							},
						},
					},
				},
				w: []Workload{{Workload: workload, Traits: []*Trait{{Object: *trait}}}},
			},
			want: []unstructured.Unstructured{},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := eligible(tc.args.namespace, tc.args.ws, tc.args.w)
			if diff := cmp.Diff(tc.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("\n%s\neligible(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestIsRevisionWorkload(t *testing.T) {
	if true != IsRevisionWorkload(v1alpha2.WorkloadStatus{ComponentName: "compName", Reference: runtimev1alpha1.TypedReference{Name: "compName-rev1"}}) {
		t.Error("workloadName has componentName as prefix is revisionWorkload")
	}
}

func TestDependency(t *testing.T) {
	unreadyWorkload := &unstructured.Unstructured{}
	unreadyWorkload.SetAPIVersion("v1")
	unreadyWorkload.SetKind("Workload")
	unreadyWorkload.SetNamespace("test-ns")
	unreadyWorkload.SetName("unready-workload")

	readyWorkload := unreadyWorkload.DeepCopy()
	readyWorkload.SetName("ready-workload")
	err := unstructured.SetNestedField(readyWorkload.Object, "test", "status", "key")
	if err != nil {
		t.Fatal(err)
	}

	unreadyTrait := &unstructured.Unstructured{}
	unreadyTrait.SetAPIVersion("v1")
	unreadyTrait.SetKind("Trait")
	unreadyTrait.SetNamespace("test-ns")
	unreadyTrait.SetName("unready-trait")

	readyTrait := unreadyTrait.DeepCopy()
	readyTrait.SetName("ready-trait")
	err = unstructured.SetNestedField(readyTrait.Object, "test", "status", "key")
	if err != nil {
		t.Fatal(err)
	}

	type args struct {
		components []v1alpha2.ApplicationConfigurationComponent
		wl         *unstructured.Unstructured
		trait      *unstructured.Unstructured
	}
	type want struct {
		err             error
		verifyWorkloads func([]Workload)
		depStatus       *v1alpha2.DependencyStatus
	}
	cases := map[string]struct {
		args args
		want want
	}{
		"Workload depends on another Workload that's unready": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					DataInputs: []v1alpha2.DataInput{{
						ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "test-output"},
						ToFieldPaths: []string{"spec.key"},
					}},
				}, {
					ComponentName: "test-component-source",
					DataOutputs: []v1alpha2.DataOutput{{
						Name:      "test-output",
						FieldPath: "status.key",
					}},
				}},
				wl:    unreadyWorkload,
				trait: unreadyTrait,
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					if !ws[0].HasDep {
						t.Error("Workload should be unready to apply")
					}
				},
				depStatus: &v1alpha2.DependencyStatus{
					Unsatisfied: []v1alpha2.UnstaifiedDependency{{
						From: v1alpha2.DependencyFromObject{
							TypedReference: runtimev1alpha1.TypedReference{
								APIVersion: unreadyWorkload.GetAPIVersion(),
								Kind:       unreadyWorkload.GetKind(),
								Name:       unreadyWorkload.GetName(),
							},
							FieldPath: "status.key",
						},
						To: v1alpha2.DependencyToObject{
							TypedReference: runtimev1alpha1.TypedReference{
								APIVersion: unreadyWorkload.GetAPIVersion(),
								Kind:       unreadyWorkload.GetKind(),
								Name:       unreadyWorkload.GetName(),
							},
							FieldPaths: []string{"spec.key"},
						},
					}},
				},
			},
		},
		"Workload depends on another Workload that's ready": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					DataInputs: []v1alpha2.DataInput{{
						ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "test-output"},
						ToFieldPaths: []string{"spec.key"},
					}},
				}, {
					ComponentName: "test-component-source",
					DataOutputs: []v1alpha2.DataOutput{{
						Name:      "test-output",
						FieldPath: "status.key",
					}},
				}},
				wl:    readyWorkload,
				trait: unreadyTrait,
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					if ws[0].HasDep {
						t.Error("Workload should be ready to apply")
					}
				},
				depStatus: &v1alpha2.DependencyStatus{},
			},
		},
		"Workload depends on a Trait that's unready": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					DataInputs: []v1alpha2.DataInput{{
						ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "test-output"},
						ToFieldPaths: []string{"spec.key"},
					}},
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{},
						DataOutputs: []v1alpha2.DataOutput{{
							Name:      "test-output",
							FieldPath: "status.key",
						}},
					}},
				}},
				wl:    unreadyWorkload,
				trait: unreadyTrait,
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					if !ws[0].HasDep {
						t.Error("Workload should be unready to apply")
					}
				},
				depStatus: &v1alpha2.DependencyStatus{
					Unsatisfied: []v1alpha2.UnstaifiedDependency{{
						From: v1alpha2.DependencyFromObject{
							TypedReference: runtimev1alpha1.TypedReference{
								APIVersion: unreadyTrait.GetAPIVersion(),
								Kind:       unreadyTrait.GetKind(),
								Name:       unreadyTrait.GetName(),
							},
							FieldPath: "status.key",
						},
						To: v1alpha2.DependencyToObject{
							TypedReference: runtimev1alpha1.TypedReference{
								APIVersion: unreadyWorkload.GetAPIVersion(),
								Kind:       unreadyWorkload.GetKind(),
								Name:       unreadyWorkload.GetName(),
							},
							FieldPaths: []string{"spec.key"},
						},
					}},
				},
			},
		},
		"Workload depends on a Trait that's ready": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					DataInputs: []v1alpha2.DataInput{{
						ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "test-output"},
						ToFieldPaths: []string{"spec.key"},
					}},
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{},
						DataOutputs: []v1alpha2.DataOutput{{
							Name:      "test-output",
							FieldPath: "status.key",
						}},
					}},
				}},
				wl:    unreadyWorkload,
				trait: readyTrait,
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					if ws[0].HasDep {
						t.Error("Workload should be ready to apply")
					}
				},
				depStatus: &v1alpha2.DependencyStatus{},
			},
		},
		"Trait depends on a Workload that's unready": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{},
						DataInputs: []v1alpha2.DataInput{{
							ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "test-output"},
							ToFieldPaths: []string{"spec.key"},
						}},
					}},
				}, {
					ComponentName: "test-component-source",
					DataOutputs: []v1alpha2.DataOutput{{
						Name:      "test-output",
						FieldPath: "status.key",
					}},
				}},
				wl:    unreadyWorkload,
				trait: unreadyTrait,
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					if !ws[0].Traits[0].HasDep {
						t.Error("Trait should be unready to apply")
					}
				},
				depStatus: &v1alpha2.DependencyStatus{
					Unsatisfied: []v1alpha2.UnstaifiedDependency{{
						From: v1alpha2.DependencyFromObject{
							TypedReference: runtimev1alpha1.TypedReference{
								APIVersion: unreadyWorkload.GetAPIVersion(),
								Kind:       unreadyWorkload.GetKind(),
								Name:       unreadyWorkload.GetName(),
							},
							FieldPath: "status.key",
						},
						To: v1alpha2.DependencyToObject{
							TypedReference: runtimev1alpha1.TypedReference{
								APIVersion: unreadyTrait.GetAPIVersion(),
								Kind:       unreadyTrait.GetKind(),
								Name:       unreadyTrait.GetName(),
							},
							FieldPaths: []string{"spec.key"},
						},
					}},
				},
			},
		},
		"Trait depends on a Workload that's ready": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{},
						DataInputs: []v1alpha2.DataInput{{
							ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "test-output"},
							ToFieldPaths: []string{"spec.key"},
						}},
					}},
				}, {
					ComponentName: "test-component-source",
					DataOutputs: []v1alpha2.DataOutput{{
						Name:      "test-output",
						FieldPath: "status.key",
					}},
				}},
				wl:    readyWorkload,
				trait: unreadyTrait,
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					if ws[0].Traits[0].HasDep {
						t.Error("Trait should be ready to apply")
					}
				},
				depStatus: &v1alpha2.DependencyStatus{},
			},
		},
		"Trait depends on another unreadyTrait that's unready": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{},
						DataInputs: []v1alpha2.DataInput{{
							ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "test-output"},
							ToFieldPaths: []string{"spec.key"},
						}},
					}, {
						Trait: runtime.RawExtension{},
						DataOutputs: []v1alpha2.DataOutput{{
							Name:      "test-output",
							FieldPath: "status.key",
						}},
					}},
				}},
				wl:    unreadyWorkload,
				trait: unreadyTrait,
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					if !ws[0].Traits[0].HasDep {
						t.Error("Trait should be unready to apply")
					}
				},
				depStatus: &v1alpha2.DependencyStatus{
					Unsatisfied: []v1alpha2.UnstaifiedDependency{{
						From: v1alpha2.DependencyFromObject{
							TypedReference: runtimev1alpha1.TypedReference{
								APIVersion: unreadyTrait.GetAPIVersion(),
								Kind:       unreadyTrait.GetKind(),
								Name:       unreadyTrait.GetName(),
							},
							FieldPath: "status.key",
						},
						To: v1alpha2.DependencyToObject{
							TypedReference: runtimev1alpha1.TypedReference{
								APIVersion: unreadyTrait.GetAPIVersion(),
								Kind:       unreadyTrait.GetKind(),
								Name:       unreadyTrait.GetName(),
							},
							FieldPaths: []string{"spec.key"},
						},
					}},
				},
			},
		},
		"Trait depends on another unreadyTrait that's ready": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{},
						DataInputs: []v1alpha2.DataInput{{
							ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "test-output"},
							ToFieldPaths: []string{"spec.key"},
						}},
					}, {
						Trait: runtime.RawExtension{},
						DataOutputs: []v1alpha2.DataOutput{{
							Name:      "test-output",
							FieldPath: "status.key",
						}},
					}},
				}},
				wl:    unreadyWorkload,
				trait: readyTrait,
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					if ws[0].Traits[0].HasDep {
						t.Error("Trait should be ready to apply")
					}
				},
				depStatus: &v1alpha2.DependencyStatus{},
			}},
		"DataOutputName doesn't exist": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					DataInputs: []v1alpha2.DataInput{{
						ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "wrong-output"},
						ToFieldPaths: []string{"spec.key"},
					}},
				}},
				wl:    unreadyWorkload,
				trait: unreadyTrait,
			},
			want: want{
				err: ErrDataOutputNotExist,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := components{
				client: &test.MockClient{
					MockGet: test.MockGetFn(func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
						if obj.GetObjectKind().GroupVersionKind().Kind == "Workload" {
							b, err := json.Marshal(tc.args.wl)
							if err != nil {
								t.Fatal(err)
							}
							err = json.Unmarshal(b, obj)
							if err != nil {
								t.Fatal(err)
							}
						}
						if obj.GetObjectKind().GroupVersionKind().Kind == "Trait" {
							b, err := json.Marshal(tc.args.trait)
							if err != nil {
								t.Fatal(err)
							}
							err = json.Unmarshal(b, obj)
							if err != nil {
								t.Fatal(err)
							}
						}
						return nil
					}),
				},
				params: ParameterResolveFn(resolve),
				workload: ResourceRenderFn(func(data []byte, p ...Parameter) (*unstructured.Unstructured, error) {
					return tc.args.wl, nil
				}),
				trait: ResourceRenderFn(func(data []byte, p ...Parameter) (*unstructured.Unstructured, error) {
					return tc.args.trait, nil
				}),
			}

			ac := &v1alpha2.ApplicationConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "test-ns",
				},
				Spec: v1alpha2.ApplicationConfigurationSpec{
					Components: tc.args.components,
				},
			}

			ws, ds, err := c.Render(context.Background(), ac)
			if err != nil {
				if errors.Is(err, tc.want.err) {
					return
				}
				t.Error(err)
				return
			}
			if diff := cmp.Diff(tc.want.err, err); diff != "" {
				t.Error(diff)
				return
			}
			tc.want.verifyWorkloads(ws)
			if diff := cmp.Diff(tc.want.depStatus, ds); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestMatchValue(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("v1")
	obj.SetKind("Workload")
	obj.SetNamespace("test-ns")
	obj.SetName("unready-workload")
	if err := unstructured.SetNestedField(obj.Object, "test", "key"); err != nil {
		t.Fatal(err)
	}
	paved, err := fieldpath.PaveObject(obj)
	if err != nil {
		t.Fatal(err)
	}

	type args struct {
		conds []v1alpha2.ConditionRequirement
		val   string
		paved *fieldpath.Paved
	}
	type want struct {
		matched bool
	}
	cases := map[string]struct {
		args args
		want want
	}{
		"No conditions with nonempty value should match": {},
		"No conditions with empty value should not match": {
			args: args{
				val: "test",
			},
			want: want{
				matched: true,
			},
		},
		"eq condition with same value should match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator: v1alpha2.ConditionEqual,
					Value:    "test",
				}},
				val: "test",
			},
			want: want{
				matched: true,
			},
		},
		"eq condition with different value should not match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator: v1alpha2.ConditionEqual,
					Value:    "test",
				}},
				val: "different",
			},
			want: want{
				matched: false,
			},
		},
		"notEq condition with different value should match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator: v1alpha2.ConditionNotEqual,
					Value:    "test",
				}},
				val: "different",
			},
			want: want{
				matched: true,
			},
		},
		"notEq condition with same value should not match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator: v1alpha2.ConditionNotEqual,
					Value:    "test",
				}},
				val: "test",
			},
			want: want{
				matched: false,
			},
		},
		"notEmpty condition with nonempty value should match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator: v1alpha2.ConditionNotEmpty,
				}},
				val: "test",
			},
			want: want{
				matched: true,
			},
		},
		"notEmpty condition with empty value should not match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator: v1alpha2.ConditionNotEmpty,
				}},
				val: "",
			},
			want: want{
				matched: false,
			},
		},
		"eq condition with same value from FieldPath should match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator:  v1alpha2.ConditionEqual,
					Value:     "test",
					FieldPath: "key",
				}},
				paved: paved,
			},
			want: want{
				matched: true,
			},
		},
		"eq condition with different value from FieldPath should not match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator:  v1alpha2.ConditionEqual,
					Value:     "different",
					FieldPath: "key",
				}},
				paved: paved,
			},
			want: want{
				matched: false,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			matched, err := matchValue(tc.args.conds, tc.args.val, tc.args.paved)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want.matched, matched); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestAddDataOutputsToDAG(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("v1")
	obj.SetKind("TestKind")
	obj.SetNamespace("test-ns")
	obj.SetName("test-name")

	dag := newDAG()
	outs := []v1alpha2.DataOutput{{
		Name:      "test-output",
		FieldPath: "spec.replica",
		Conditions: []v1alpha2.ConditionRequirement{{
			Operator:  v1alpha2.ConditionEqual,
			Value:     "abc",
			FieldPath: "status.state",
		}},
	}}
	addDataOutputsToDAG(dag, outs, obj)

	s, ok := dag.Sources["test-output"]
	if !ok {
		t.Fatal("didn't add source correctly")
	}

	r := &corev1.ObjectReference{
		APIVersion: obj.GetAPIVersion(),
		Kind:       obj.GetKind(),
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
		FieldPath:  outs[0].FieldPath,
	}

	if diff := cmp.Diff(s.ObjectRef, r); diff != "" {
		t.Errorf("didn't add objectRef to source correctly: %s", diff)
	}

	if diff := cmp.Diff(s.Conditions, outs[0].Conditions); diff != "" {
		t.Errorf("didn't add conditions to source correctly: %s", diff)
	}
}
