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

package trait

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	"github.com/crossplane/oam-runtime/pkg/oam"
	"github.com/crossplane/oam-runtime/pkg/oam/fake"
)

var _ reconcile.Reconciler = &Reconciler{}

func TestReconciler(t *testing.T) {
	type args struct {
		m manager.Manager
		t oam.TraitKind
		w oam.WorkloadKind
		p schema.GroupVersionKind
		o []ReconcilerOption
	}

	type want struct {
		result reconcile.Result
		err    error
	}

	errBoom := errors.New("boom")

	workloadName := "cool-workload"
	workloadKind := "CoolWorkload"
	workloadAPIVersion := "example/v1"

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"GetTraitError": {
			reason: "Any error (except not found) encountered while getting the resource under reconciliation should be returned.",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{MockGet: test.NewMockGetFn(errBoom)},
					Scheme: fake.SchemeWith(&fake.Trait{}),
				},
				t: oam.TraitKind(fake.GVK(&fake.Trait{})),
				w: oam.WorkloadKind(fake.GVK(&fake.Workload{})),
				p: fake.GVK(&fake.Object{}),
			},
			want: want{err: errors.Wrap(errBoom, errGetTrait)},
		},
		"TraitNotFound": {
			reason: "Not found errors encountered while getting the resource under reconciliation should be ignored.",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{MockGet: test.NewMockGetFn(kerrors.NewNotFound(schema.GroupResource{}, ""))},
					Scheme: fake.SchemeWith(&fake.Trait{}),
				},
				t: oam.TraitKind(fake.GVK(&fake.Trait{})),
				w: oam.WorkloadKind(fake.GVK(&fake.Workload{})),
				p: fake.GVK(&fake.Object{}),
			},
			want: want{result: reconcile.Result{}},
		},
		"WorkloadNotFound": {
			reason: "Status should report successful reconcile and we should requeue after short wait if workload is not found.",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
							if t, ok := obj.(oam.Trait); ok {
								t.SetWorkloadReference(v1alpha1.TypedReference{
									APIVersion: workloadAPIVersion,
									Kind:       workloadKind,
									Name:       workloadName,
								})
								return nil
							}
							if _, ok := obj.(oam.Workload); ok {
								return kerrors.NewNotFound(schema.GroupResource{}, "")
							}
							return errBoom
						},
						MockStatusUpdate: func(_ context.Context, obj runtime.Object, _ ...client.UpdateOption) error {
							got := obj.(oam.Trait)

							if diff := cmp.Diff(v1alpha1.ReasonReconcileSuccess, got.GetCondition(v1alpha1.TypeSynced).Reason); diff != "" {
								return errors.Errorf("MockStatusUpdate: -want, +got: %s", diff)
							}

							return nil
						},
					},
					Scheme: fake.SchemeWith(&fake.Trait{}, &fake.Workload{}),
				},
				t: oam.TraitKind(fake.GVK(&fake.Trait{})),
				w: oam.WorkloadKind(fake.GVK(&fake.Workload{})),
				p: fake.GVK(&fake.Object{}),
			},
			want: want{result: reconcile.Result{RequeueAfter: shortWait}},
		},
		"GetWorkloadError": {
			reason: "Status should report reconcile error and we should requeue after short wait if we encounter an error getting the workload.",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
							if t, ok := obj.(oam.Trait); ok {
								t.SetWorkloadReference(v1alpha1.TypedReference{
									APIVersion: workloadAPIVersion,
									Kind:       workloadKind,
									Name:       workloadName,
								})
								return nil
							}
							if _, ok := obj.(oam.Workload); ok {
								return errBoom
							}
							return nil
						},
						MockStatusUpdate: func(_ context.Context, obj runtime.Object, _ ...client.UpdateOption) error {
							got := obj.(oam.Trait)

							if diff := cmp.Diff(v1alpha1.ReasonReconcileError, got.GetCondition(v1alpha1.TypeSynced).Reason); diff != "" {
								return errors.Errorf("MockStatusUpdate: -want, +got: %s", diff)
							}

							if diff := cmp.Diff(errors.Wrap(errBoom, errGetWorkload).Error(), got.GetCondition(v1alpha1.TypeSynced).Message); diff != "" {
								return errors.Errorf("MockStatusUpdate: -want, +got: %s", diff)
							}

							return nil
						},
					},
					Scheme: fake.SchemeWith(&fake.Trait{}, &fake.Workload{}),
				},
				t: oam.TraitKind(fake.GVK(&fake.Trait{})),
				w: oam.WorkloadKind(fake.GVK(&fake.Workload{})),
				p: fake.GVK(&fake.Object{}),
			},
			want: want{result: reconcile.Result{RequeueAfter: shortWait}},
		},
		"TranslationNotFound": {
			reason: "Status should report successful reconcile and we should requeue after short wait if translation is not found.",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
							if t, ok := obj.(oam.Trait); ok {
								t.SetWorkloadReference(v1alpha1.TypedReference{
									APIVersion: workloadAPIVersion,
									Kind:       workloadKind,
									Name:       workloadName,
								})
								return nil
							}
							return kerrors.NewNotFound(schema.GroupResource{}, "")
						},
						MockStatusUpdate: func(_ context.Context, obj runtime.Object, _ ...client.UpdateOption) error {
							got := obj.(oam.Trait)

							if diff := cmp.Diff(v1alpha1.ReasonReconcileSuccess, got.GetCondition(v1alpha1.TypeSynced).Reason); diff != "" {
								return errors.Errorf("MockStatusUpdate: -want, +got: %s", diff)
							}

							return nil
						},
					},
					Scheme: fake.SchemeWith(&fake.Trait{}, &fake.Workload{}, &fake.Object{}),
				},
				t: oam.TraitKind(fake.GVK(&fake.Trait{})),
				w: oam.WorkloadKind(fake.GVK(&fake.Workload{})),
				p: fake.GVK(&fake.Object{}),
			},
			want: want{result: reconcile.Result{RequeueAfter: shortWait}},
		},
		"GetTranslationError": {
			reason: "Status should report reconcile error and we should requeue after short wait if we encounter an error getting the translation.",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
							if t, ok := obj.(oam.Trait); ok {
								t.SetWorkloadReference(v1alpha1.TypedReference{
									APIVersion: workloadAPIVersion,
									Kind:       workloadKind,
									Name:       workloadName,
								})
								return nil
							}
							if w, ok := obj.(oam.Workload); ok {
								w.SetName(workloadName)
								return nil
							}
							return errBoom
						},
						MockStatusUpdate: func(_ context.Context, obj runtime.Object, _ ...client.UpdateOption) error {
							got := obj.(oam.Trait)

							if diff := cmp.Diff(v1alpha1.ReasonReconcileError, got.GetCondition(v1alpha1.TypeSynced).Reason); diff != "" {
								return errors.Errorf("MockStatusUpdate: -want, +got: %s", diff)
							}

							if diff := cmp.Diff(errors.Wrap(errBoom, errGetTranslation).Error(), got.GetCondition(v1alpha1.TypeSynced).Message); diff != "" {
								return errors.Errorf("MockStatusUpdate: -want, +got: %s", diff)
							}

							return nil
						},
					},
					Scheme: fake.SchemeWith(&fake.Trait{}, &fake.Workload{}, &fake.Object{}),
				},
				t: oam.TraitKind(fake.GVK(&fake.Trait{})),
				w: oam.WorkloadKind(fake.GVK(&fake.Workload{})),
				p: fake.GVK(&fake.Object{}),
			},
			want: want{result: reconcile.Result{RequeueAfter: shortWait}},
		},
		"ModifyError": {
			reason: "Status should report reconcile error and we should requeue after short wait if we encounter an error modifying the translation.",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
							if t, ok := obj.(oam.Trait); ok {
								t.SetWorkloadReference(v1alpha1.TypedReference{
									APIVersion: workloadAPIVersion,
									Kind:       workloadKind,
									Name:       workloadName,
								})
								return nil
							}
							if _, ok := obj.(oam.Object); ok {
								return nil
							}
							return errBoom
						},
						MockStatusUpdate: func(_ context.Context, obj runtime.Object, _ ...client.UpdateOption) error {
							got := obj.(oam.Trait)

							if diff := cmp.Diff(v1alpha1.ReasonReconcileError, got.GetCondition(v1alpha1.TypeSynced).Reason); diff != "" {
								return errors.Errorf("MockStatusUpdate: -want, +got: %s", diff)
							}

							if diff := cmp.Diff(errors.Wrap(errBoom, errTraitModify).Error(), got.GetCondition(v1alpha1.TypeSynced).Message); diff != "" {
								return errors.Errorf("MockStatusUpdate: -want, +got: %s", diff)
							}

							return nil
						},
					},
					Scheme: fake.SchemeWith(&fake.Trait{}, &fake.Workload{}, &fake.Object{}),
				},
				t: oam.TraitKind(fake.GVK(&fake.Trait{})),
				w: oam.WorkloadKind(fake.GVK(&fake.Workload{})),
				p: fake.GVK(&fake.Object{}),
				o: []ReconcilerOption{WithModifier(ModifyFn(func(_ context.Context, _ runtime.Object, _ oam.Trait) error {
					return errBoom
				}))},
			},
			want: want{result: reconcile.Result{RequeueAfter: shortWait}},
		},
		"ApplyError": {
			reason: "Failure to apply Workload translate should be returned.",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
							if t, ok := obj.(oam.Trait); ok {
								t.SetWorkloadReference(v1alpha1.TypedReference{
									APIVersion: workloadAPIVersion,
									Kind:       workloadKind,
									Name:       workloadName,
								})
								return nil
							}
							if _, ok := obj.(oam.Object); ok {
								return nil
							}
							return errBoom
						},
						MockStatusUpdate: func(_ context.Context, obj runtime.Object, _ ...client.UpdateOption) error {
							got := obj.(oam.Trait)

							if diff := cmp.Diff(v1alpha1.ReasonReconcileError, got.GetCondition(v1alpha1.TypeSynced).Reason); diff != "" {
								return errors.Errorf("MockStatusUpdate: -want, +got: %s", diff)
							}

							if diff := cmp.Diff(errors.Wrap(errBoom, errApplyTraitModification).Error(), got.GetCondition(v1alpha1.TypeSynced).Message); diff != "" {
								return errors.Errorf("MockStatusUpdate: -want, +got: %s", diff)
							}

							return nil
						},
					},
					Scheme: fake.SchemeWith(&fake.Trait{}, &fake.Workload{}, &fake.Object{}),
				},
				t: oam.TraitKind(fake.GVK(&fake.Trait{})),
				w: oam.WorkloadKind(fake.GVK(&fake.Workload{})),
				p: fake.GVK(&fake.Object{}),
				o: []ReconcilerOption{WithApplicator(resource.ApplyFn(func(_ context.Context, _ runtime.Object, _ ...resource.ApplyOption) error {
					return errBoom
				}))},
			},
			want: want{result: reconcile.Result{RequeueAfter: shortWait}},
		},
		"Successful": {
			reason: "Successful reconciliaton should result in requeue after long wait.",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
							if t, ok := obj.(oam.Trait); ok {
								t.SetWorkloadReference(v1alpha1.TypedReference{
									APIVersion: workloadAPIVersion,
									Kind:       workloadKind,
									Name:       workloadName,
								})
								return nil
							}
							if _, ok := obj.(oam.Object); ok {
								return nil
							}
							return errBoom
						},
						MockStatusUpdate: func(_ context.Context, obj runtime.Object, _ ...client.UpdateOption) error {
							got := obj.(oam.Trait)

							if diff := cmp.Diff(v1alpha1.ReasonReconcileSuccess, got.GetCondition(v1alpha1.TypeSynced).Reason); diff != "" {
								return errors.Errorf("MockStatusUpdate: -want, +got: %s", diff)
							}

							if diff := cmp.Diff("", got.GetCondition(v1alpha1.TypeSynced).Message); diff != "" {
								return errors.Errorf("MockStatusUpdate: -want, +got: %s", diff)
							}

							return nil
						},
					},
					Scheme: fake.SchemeWith(&fake.Trait{}, &fake.Workload{}, &fake.Object{}),
				},
				t: oam.TraitKind(fake.GVK(&fake.Trait{})),
				w: oam.WorkloadKind(fake.GVK(&fake.Workload{})),
				p: fake.GVK(&fake.Object{}),
				o: []ReconcilerOption{WithApplicator(resource.ApplyFn(func(_ context.Context, _ runtime.Object, _ ...resource.ApplyOption) error {
					return nil
				}))},
			},
			want: want{result: reconcile.Result{RequeueAfter: longWait}},
		},
		"SuccessfulStatusUpdateError": {
			reason: "Successful reconciliaton should result in requeue after long wait, but status update error will trigger immediate requeue.",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
							if t, ok := obj.(oam.Trait); ok {
								t.SetWorkloadReference(v1alpha1.TypedReference{
									APIVersion: workloadAPIVersion,
									Kind:       workloadKind,
									Name:       workloadName,
								})
								return nil
							}
							if _, ok := obj.(oam.Object); ok {
								return nil
							}
							return errBoom
						},
						MockStatusUpdate: test.NewMockStatusUpdateFn(errBoom),
					},
					Scheme: fake.SchemeWith(&fake.Trait{}, &fake.Workload{}, &fake.Object{}),
				},
				t: oam.TraitKind(fake.GVK(&fake.Trait{})),
				w: oam.WorkloadKind(fake.GVK(&fake.Workload{})),
				p: fake.GVK(&fake.Object{}),
				o: []ReconcilerOption{WithApplicator(resource.ApplyFn(func(_ context.Context, _ runtime.Object, _ ...resource.ApplyOption) error {
					return nil
				}))},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: longWait},
				err:    errors.Wrap(errBoom, errUpdateTraitStatus),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := NewReconciler(tc.args.m, tc.args.t, tc.args.w, tc.args.p, tc.args.o...)
			got, err := r.Reconcile(reconcile.Request{})

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\nReason: %s\nr.Reconcile(...): -want error, +got error:\n%s", tc.reason, diff)
			}

			if diff := cmp.Diff(tc.want.result, got); diff != "" {
				t.Errorf("\nReason: %s\nr.Reconcile(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
