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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/pkg/errors"
)

// Reconcile error strings.
const (
	errFmtApplyWorkload  = "cannot apply workload %q"
	errFmtSetWorkloadRef = "cannot set trait %q reference to %q"
	errFmtApplyTrait     = "cannot apply trait %q %q"
	errFmtApplyScope     = "cannot apply scope %q"
)

// A WorkloadApplicator creates or updates workloads and their traits.
type WorkloadApplicator interface {
	// Apply a workload and its traits.
	Apply(ctx context.Context, namespace string, status []v1alpha2.WorkloadStatus, w []Workload, ao ...resource.ApplyOption) error
}

// A WorkloadApplyFn creates or updates workloads and their traits.
type WorkloadApplyFn func(ctx context.Context, namespace string, status []v1alpha2.WorkloadStatus, w []Workload, ao ...resource.ApplyOption) error

// Apply a workload and its traits.
func (fn WorkloadApplyFn) Apply(ctx context.Context, namespace string, status []v1alpha2.WorkloadStatus, w []Workload, ao ...resource.ApplyOption) error {
	return fn(ctx, namespace, status, w, ao...)
}

type workloads struct {
	client    resource.Applicator
	rawClient client.Client
}

func (a *workloads) Apply(ctx context.Context, namespace string, status []v1alpha2.WorkloadStatus, w []Workload, ao ...resource.ApplyOption) error {
	for _, wl := range w {
		if err := a.client.Apply(ctx, wl.Workload, ao...); err != nil {
			return errors.Wrapf(err, errFmtApplyWorkload, wl.Workload.GetName())
		}

		workloadRef := runtimev1alpha1.TypedReference{
			APIVersion: wl.Workload.GetAPIVersion(),
			Kind:       wl.Workload.GetKind(),
			Name:       wl.Workload.GetName(),
		}

		for i := range wl.Traits {
			t := &wl.Traits[i]

			// TODO(rz): Need to find a way to make sure that this is compatible with the trait structure.
			if err := fieldpath.Pave(t.UnstructuredContent()).SetValue("spec.workloadRef", workloadRef); err != nil {
				return errors.Wrapf(err, errFmtSetWorkloadRef, t.GetName(), wl.Workload.GetName())
			}

			if err := a.client.Apply(ctx, t, ao...); err != nil {
				return errors.Wrapf(err, errFmtApplyTrait, t.GetKind(), t.GetName())
			}
		}

		for _, s := range wl.Scopes {
			return a.applyScope(ctx, wl, s)
		}
	}

	return a.dereferenceScope(ctx, namespace, status, w)
}

func (a *workloads) dereferenceScope(ctx context.Context, namespace string, status []v1alpha2.WorkloadStatus, w []Workload) error {
	for _, st := range status {
		toBeDeferenced := st.Scopes
		for _, wl := range w {
			if (st.Reference.APIVersion == wl.Workload.GetAPIVersion()) &&
				(st.Reference.Kind == wl.Workload.GetKind()) &&
				(st.Reference.Name == wl.Workload.GetName()) {
				toBeDeferenced = findDereferencedScopes(st.Scopes, wl.Scopes)
			}
		}

		for _, s := range toBeDeferenced {
			if err := a.applyScopeRemoval(ctx, namespace, st, s); err != nil {
				return err
			}
		}
	}

	return nil
}

func findDereferencedScopes(statusScopes []v1alpha2.WorkloadScope, scopes []unstructured.Unstructured) []v1alpha2.WorkloadScope {
	toBeDeferenced := []v1alpha2.WorkloadScope{}
	for _, ss := range statusScopes {
		found := false
		for _, s := range scopes {
			if (s.GetAPIVersion() == ss.Reference.APIVersion) &&
				(s.GetKind() == ss.Reference.Kind) &&
				(s.GetName() == ss.Reference.Name) {
				found = true
				break
			}
		}

		if !found {
			toBeDeferenced = append(toBeDeferenced, ss)
		}
	}

	return toBeDeferenced
}

func (a *workloads) applyScope(ctx context.Context, wl Workload, s unstructured.Unstructured) error {
	workloadRef := runtimev1alpha1.TypedReference{
		APIVersion: wl.Workload.GetAPIVersion(),
		Kind:       wl.Workload.GetKind(),
		Name:       wl.Workload.GetName(),
	}

	// Get Scope again to make sure we work with the most up-to date object.
	scopeObject := unstructured.Unstructured{}
	scopeObject.SetAPIVersion(s.GetAPIVersion())
	scopeObject.SetKind(s.GetKind())
	scopeObjectRef := types.NamespacedName{Namespace: s.GetNamespace(), Name: s.GetName()}
	if err := a.rawClient.Get(ctx, scopeObjectRef, &scopeObject); err != nil {
		return errors.Wrapf(err, errFmtApplyScope, s.GetName())
	}

	refs := []interface{}{}
	// TODO(asouza): Need to find a way to make sure that this is compatible with the scope structure.
	if value, err := fieldpath.Pave(scopeObject.UnstructuredContent()).GetValue("spec.workloadRefs"); err == nil {
		refs = value.([]interface{})

		for _, item := range refs {
			ref := item.(map[string]interface{})
			if (workloadRef.APIVersion == ref["apiVersion"]) &&
				(workloadRef.Kind == ref["kind"]) &&
				(workloadRef.Name == ref["name"]) {
				// workloadRef is already present, so no need to add it.
				return nil
			}
		}
	}

	refs = append(refs, workloadRef)
	// TODO(asouza): Need to find a way to make sure that this is compatible with the scope structure.
	if err := fieldpath.Pave(scopeObject.UnstructuredContent()).SetValue("spec.workloadRefs", refs); err != nil {
		return errors.Wrapf(err, errFmtSetWorkloadRef, s.GetName(), wl.Workload.GetName())
	}

	if err := a.rawClient.Update(ctx, &scopeObject); err != nil {
		return errors.Wrapf(err, errFmtApplyScope, s.GetName())
	}

	return nil
}

func (a *workloads) applyScopeRemoval(ctx context.Context, namespace string, ws v1alpha2.WorkloadStatus, s v1alpha2.WorkloadScope) error {
	workloadRef := runtimev1alpha1.TypedReference{
		APIVersion: ws.Reference.APIVersion,
		Kind:       ws.Reference.Kind,
		Name:       ws.Reference.Name,
	}

	// Get Scope again to make sure we work with the most up-to date object.
	scopeObject := unstructured.Unstructured{}
	scopeObject.SetAPIVersion(s.Reference.APIVersion)
	scopeObject.SetKind(s.Reference.Kind)
	scopeObjectRef := types.NamespacedName{Namespace: namespace, Name: s.Reference.Name}
	if err := a.rawClient.Get(ctx, scopeObjectRef, &scopeObject); err != nil {
		return errors.Wrapf(err, errFmtApplyScope, s.Reference.Name)
	}

	// TODO(asouza): Need to find a way to make sure that this is compatible with the scope structure.
	if value, err := fieldpath.Pave(scopeObject.UnstructuredContent()).GetValue("spec.workloadRefs"); err == nil {
		refs := value.([]interface{})

		workloadRefIndex := -1
		for i, item := range refs {
			ref := item.(map[string]interface{})
			if (workloadRef.APIVersion == ref["apiVersion"]) &&
				(workloadRef.Kind == ref["kind"]) &&
				(workloadRef.Name == ref["name"]) {
				workloadRefIndex = i
				break
			}
		}

		if workloadRefIndex >= 0 {
			// Remove the element at index i.
			refs[workloadRefIndex] = refs[len(refs)-1]
			refs = refs[:len(refs)-1]

			// TODO(asouza): Need to find a way to make sure that this is compatible with the scope structure.
			if err := fieldpath.Pave(scopeObject.UnstructuredContent()).SetValue("spec.workloadRefs", refs); err != nil {
				return errors.Wrapf(err, errFmtSetWorkloadRef, s.Reference.Name, ws.Reference.Name)
			}

			if err := a.rawClient.Update(ctx, &scopeObject); err != nil {
				return errors.Wrapf(err, errFmtApplyScope, s.Reference.Name)
			}
		}
	}

	return nil
}
