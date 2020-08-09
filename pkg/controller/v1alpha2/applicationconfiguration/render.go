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
	"encoding/json"
	"fmt"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

// Render error strings.
const (
	errUnmarshalWorkload = "cannot unmarshal workload"
	errUnmarshalTrait    = "cannot unmarshal trait"
)

// Render error format strings.
const (
	errFmtGetComponent     = "cannot get component %q"
	errFmtGetScope         = "cannot get scope %q"
	errFmtResolveParams    = "cannot resolve parameter values for component %q"
	errFmtRenderWorkload   = "cannot render workload for component %q"
	errFmtRenderTrait      = "cannot render trait for component %q"
	errFmtSetParam         = "cannot set parameter %q"
	errFmtUnsupportedParam = "unsupported parameter %q"
	errFmtRequiredParam    = "required parameter %q not specified"
	errSetValueForField    = "can not set value %q for fieldPath %q"
)

var (
	// ErrDataOutputNotExist is an error indicating the DataOutput specified doesn't not exist
	ErrDataOutputNotExist = errors.New("DataOutput does not exist")
)

const instanceNamePath = "metadata.name"

// A ComponentRenderer renders an ApplicationConfiguration's Components into
// workloads and traits.
type ComponentRenderer interface {
	Render(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error)
}

// A ComponentRenderFn renders an ApplicationConfiguration's Components into
// workloads and traits.
type ComponentRenderFn func(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error)

// Render an ApplicationConfiguration's Components into workloads and traits.
func (fn ComponentRenderFn) Render(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error) {
	return fn(ctx, ac)
}

type components struct {
	client   client.Reader
	params   ParameterResolver
	workload ResourceRenderer
	trait    ResourceRenderer
}

func (r *components) Render(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error) {
	workloads := make([]*Workload, 0, len(ac.Spec.Components))
	dag := newDAG()

	for _, acc := range ac.Spec.Components {
		w, err := r.renderComponent(ctx, acc, ac, dag)
		if err != nil {
			return nil, nil, err
		}

		workloads = append(workloads, w)
	}

	ds := &v1alpha2.DependencyStatus{}
	res := make([]Workload, 0, len(ac.Spec.Components))
	for i, acc := range ac.Spec.Components {
		unsatisfied, err := r.handleDependency(ctx, workloads[i], acc, dag)
		if err != nil {
			return nil, nil, err
		}
		ds.Unsatisfied = append(ds.Unsatisfied, unsatisfied...)
		res = append(res, *workloads[i])
	}

	return res, ds, nil
}

func (r *components) renderComponent(ctx context.Context, acc v1alpha2.ApplicationConfigurationComponent, ac *v1alpha2.ApplicationConfiguration, dag *dag) (*Workload, error) {
	if acc.RevisionName != "" {
		acc.ComponentName = ExtractComponentName(acc.RevisionName)
	}
	c, componentRevisionName, err := util.GetComponent(ctx, r.client, acc, ac.GetNamespace())
	if err != nil {
		return nil, err
	}
	p, err := r.params.Resolve(c.Spec.Parameters, acc.ParameterValues)
	if err != nil {
		return nil, errors.Wrapf(err, errFmtResolveParams, acc.ComponentName)
	}

	w, err := r.workload.Render(c.Spec.Workload.Raw, p...)
	if err != nil {
		return nil, errors.Wrapf(err, errFmtRenderWorkload, acc.ComponentName)
	}

	// pass through labels and annotation from app-config to workload
	util.PassLabelAndAnnotation(ac, w)

	ref := metav1.NewControllerRef(ac, v1alpha2.ApplicationConfigurationGroupVersionKind)
	w.SetOwnerReferences([]metav1.OwnerReference{*ref})
	w.SetNamespace(ac.GetNamespace())

	traits := make([]*Trait, 0, len(acc.Traits))
	traitDefs := make([]v1alpha2.TraitDefinition, 0, len(acc.Traits))
	for _, ct := range acc.Traits {
		t, traitDef, err := r.renderTrait(ctx, ct, ac, acc.ComponentName, ref, dag)
		if err != nil {
			return nil, err
		}

		// pass through labels and annotation from app-config to trait
		util.PassLabelAndAnnotation(ac, t)
		traits = append(traits, &Trait{Object: *t})
		traitDefs = append(traitDefs, *traitDef)
	}
	if err := SetWorkloadInstanceName(acc.RevisionName, traitDefs, w, c); err != nil {
		return nil, err
	}

	scopes := make([]unstructured.Unstructured, 0, len(acc.Scopes))
	for _, cs := range acc.Scopes {
		scopeObject, err := r.renderScope(ctx, cs, ac.GetNamespace())
		if err != nil {
			return nil, err
		}

		scopes = append(scopes, *scopeObject)
	}

	addDataOutputsToDAG(dag, acc.DataOutputs, w)

	return &Workload{ComponentName: acc.ComponentName, ComponentRevisionName: componentRevisionName, Workload: w, Traits: traits, Scopes: scopes}, nil
}

func (r *components) renderTrait(ctx context.Context, ct v1alpha2.ComponentTrait, ac *v1alpha2.ApplicationConfiguration,
	componentName string, ref *metav1.OwnerReference, dag *dag) (*unstructured.Unstructured, *v1alpha2.TraitDefinition, error) {
	t, err := r.trait.Render(ct.Trait.Raw)
	if err != nil {
		return nil, nil, errors.Wrapf(err, errFmtRenderTrait, componentName)
	}

	traitName := getTraitName(ac, componentName, &ct, t)

	setTraitProperties(t, traitName, ac.GetNamespace(), ref)

	traitDef, err := util.FetchTraitDefinition(ctx, r.client, t)
	if err != nil {
		return nil, nil, errors.Wrapf(err, errFmtGetTraitDefinition, t.GetAPIVersion(), t.GetKind(), t.GetName())
	}

	addDataOutputsToDAG(dag, ct.DataOutputs, t)

	return t, traitDef, nil
}

func (r *components) renderScope(ctx context.Context, cs v1alpha2.ComponentScope, ns string) (*unstructured.Unstructured, error) {
	// Get Scope instance from k8s, since it is global and not a child resource of workflow.
	scopeObject := &unstructured.Unstructured{}
	scopeObject.SetAPIVersion(cs.ScopeReference.APIVersion)
	scopeObject.SetKind(cs.ScopeReference.Kind)
	scopeObjectRef := types.NamespacedName{Namespace: ns, Name: cs.ScopeReference.Name}
	if err := r.client.Get(ctx, scopeObjectRef, scopeObject); err != nil {
		return nil, errors.Wrapf(err, errFmtGetScope, cs.ScopeReference.Name)
	}
	return scopeObject, nil
}

func setTraitProperties(t *unstructured.Unstructured, traitName, namespace string, ref *metav1.OwnerReference) {
	// Set metadata name for `Trait` if the metadata name is NOT set.
	if t.GetName() == "" {
		t.SetName(traitName)
	}

	t.SetOwnerReferences([]metav1.OwnerReference{*ref})
	t.SetNamespace(namespace)
}

// SetWorkloadInstanceName will set metadata.name for workload CR according to createRevision flag in traitDefinition
func SetWorkloadInstanceName(revisionName string, traitDefs []v1alpha2.TraitDefinition, w *unstructured.Unstructured, c *v1alpha2.Component) error {
	//Don't override the specified name
	if w.GetName() != "" {
		return nil
	}
	pv := fieldpath.Pave(w.UnstructuredContent())

	// if specified revisionName assigned, use revisionName as the workload name
	if revisionName != "" {
		if err := pv.SetString(instanceNamePath, revisionName); err != nil {
			return errors.Wrapf(err, errSetValueForField, instanceNamePath, revisionName)
		}
		return nil
	}
	if isRevisionEnabled(traitDefs) {
		// if revisionEnabled, use revisionName as the workload name
		if err := pv.SetString(instanceNamePath, c.Status.LatestRevision.Name); err != nil {
			return errors.Wrapf(err, errSetValueForField, instanceNamePath, c.Status.LatestRevision)
		}
		return nil
	}
	// use component name as workload name, which means we will always use one workload for different revisions
	if err := pv.SetString(instanceNamePath, c.GetName()); err != nil {
		return errors.Wrapf(err, errSetValueForField, instanceNamePath, c.GetName())
	}
	w.Object = pv.UnstructuredContent()
	return nil
}

// isRevisionEnabled will check if any of the traitDefinitions has a createRevision flag
func isRevisionEnabled(traitDefs []v1alpha2.TraitDefinition) bool {
	for _, td := range traitDefs {
		if td.Spec.RevisionEnabled {
			return true
		}
	}
	return false
}

// A ResourceRenderer renders a Kubernetes-compliant YAML resource into an
// Unstructured object, optionally setting the supplied parameters.
type ResourceRenderer interface {
	Render(data []byte, p ...Parameter) (*unstructured.Unstructured, error)
}

// A ResourceRenderFn renders a Kubernetes-compliant YAML resource into an
// Unstructured object, optionally setting the supplied parameters.
type ResourceRenderFn func(data []byte, p ...Parameter) (*unstructured.Unstructured, error)

// Render the supplied Kubernetes YAML resource.
func (fn ResourceRenderFn) Render(data []byte, p ...Parameter) (*unstructured.Unstructured, error) {
	return fn(data, p...)
}

func renderWorkload(data []byte, p ...Parameter) (*unstructured.Unstructured, error) {
	// TODO(negz): Is there a better decoder to use here?
	w := &fieldpath.Paved{}
	if err := json.Unmarshal(data, w); err != nil {
		return nil, errors.Wrap(err, errUnmarshalWorkload)
	}

	for _, param := range p {
		for _, path := range param.FieldPaths {
			// TODO(negz): Infer parameter type from workload OpenAPI schema.
			switch param.Value.Type {
			case intstr.String:
				if err := w.SetString(path, param.Value.StrVal); err != nil {
					return nil, errors.Wrapf(err, errFmtSetParam, param.Name)
				}
			case intstr.Int:
				if err := w.SetNumber(path, float64(param.Value.IntVal)); err != nil {
					return nil, errors.Wrapf(err, errFmtSetParam, param.Name)
				}
			}
		}
	}

	return &unstructured.Unstructured{Object: w.UnstructuredContent()}, nil
}

func renderTrait(data []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
	// TODO(negz): Is there a better decoder to use here?
	u := &unstructured.Unstructured{}
	if err := json.Unmarshal(data, u); err != nil {
		return nil, errors.Wrap(err, errUnmarshalTrait)
	}
	return u, nil
}

// A Parameter may be used to set the supplied paths to the supplied value.
type Parameter struct {
	// Name of this parameter.
	Name string

	// Value of this parameter.
	Value intstr.IntOrString

	// FieldPaths that should be set to this parameter's value.
	FieldPaths []string
}

// A ParameterResolver resolves the parameters accepted by a component and the
// parameter values supplied to a component into configured parameters.
type ParameterResolver interface {
	Resolve([]v1alpha2.ComponentParameter, []v1alpha2.ComponentParameterValue) ([]Parameter, error)
}

// A ParameterResolveFn resolves the parameters accepted by a component and the
// parameter values supplied to a component into configured parameters.
type ParameterResolveFn func([]v1alpha2.ComponentParameter, []v1alpha2.ComponentParameterValue) ([]Parameter, error)

// Resolve the supplied parameters.
func (fn ParameterResolveFn) Resolve(cp []v1alpha2.ComponentParameter, cpv []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
	return fn(cp, cpv)
}

func resolve(cp []v1alpha2.ComponentParameter, cpv []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
	supported := make(map[string]bool)
	for _, v := range cp {
		supported[v.Name] = true
	}

	set := make(map[string]*Parameter)
	for _, v := range cpv {
		if !supported[v.Name] {
			return nil, errors.Errorf(errFmtUnsupportedParam, v.Name)
		}
		set[v.Name] = &Parameter{Name: v.Name, Value: v.Value}
	}

	for _, p := range cp {
		_, ok := set[p.Name]
		if !ok && p.Required != nil && *p.Required {
			// This parameter is required, but not set.
			return nil, errors.Errorf(errFmtRequiredParam, p.Name)
		}
		if !ok {
			// This parameter is not required, and not set.
			continue
		}

		set[p.Name].FieldPaths = p.FieldPaths
	}

	params := make([]Parameter, 0, len(set))
	for _, p := range set {
		params = append(params, *p)
	}

	return params, nil
}

func addDataOutputsToDAG(dag *dag, outs []v1alpha2.DataOutput, obj *unstructured.Unstructured) {
	for _, out := range outs {
		r := &corev1.ObjectReference{
			APIVersion: obj.GetAPIVersion(),
			Kind:       obj.GetKind(),
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
			FieldPath:  out.FieldPath,
		}
		dag.AddSource(out.Name, r, out.Conditions)
	}
}

func (r *components) handleDependency(ctx context.Context, w *Workload, acc v1alpha2.ApplicationConfigurationComponent, dag *dag) ([]v1alpha2.UnstaifiedDependency, error) {
	uds := make([]v1alpha2.UnstaifiedDependency, 0)

	unsatisfied, err := r.handleDataInput(ctx, acc.DataInputs, dag, w.Workload)
	if err != nil {
		return nil, errors.Wrapf(err, "handleDataInput for workload (%s/%s) failed", w.Workload.GetNamespace(), w.Workload.GetName())
	}
	if len(unsatisfied) != 0 {
		uds = append(uds, unsatisfied...)
		w.HasDep = true
	}

	for i, ct := range acc.Traits {
		trait := w.Traits[i]
		unsatisfied, err := r.handleDataInput(ctx, ct.DataInputs, dag, &trait.Object)
		if err != nil {
			return nil, errors.Wrapf(err, "handleDataInput for trait (%s/%s) failed", trait.Object.GetNamespace(), trait.Object.GetName())
		}
		if len(unsatisfied) != 0 {
			uds = append(uds, unsatisfied...)
			trait.HasDep = true
		}
	}
	return uds, nil
}

func makeUnsatisfiedDependency(obj *unstructured.Unstructured, s *dagSource, in v1alpha2.DataInput) v1alpha2.UnstaifiedDependency {
	return v1alpha2.UnstaifiedDependency{
		From: v1alpha2.DependencyFromObject{
			TypedReference: runtimev1alpha1.TypedReference{
				APIVersion: s.ObjectRef.APIVersion,
				Kind:       s.ObjectRef.Kind,
				Name:       s.ObjectRef.Name,
			},
			FieldPath: s.ObjectRef.FieldPath,
		},
		To: v1alpha2.DependencyToObject{
			TypedReference: runtimev1alpha1.TypedReference{
				APIVersion: obj.GetAPIVersion(),
				Kind:       obj.GetKind(),
				Name:       obj.GetName(),
			},
			FieldPaths: in.ToFieldPaths,
		},
	}
}

func (r *components) handleDataInput(ctx context.Context, ins []v1alpha2.DataInput, dag *dag, obj *unstructured.Unstructured) ([]v1alpha2.UnstaifiedDependency, error) {
	uds := make([]v1alpha2.UnstaifiedDependency, 0)
	for _, in := range ins {
		s, ok := dag.Sources[in.ValueFrom.DataOutputName]
		if !ok {
			return nil, errors.Wrapf(ErrDataOutputNotExist, "DataOutputName (%s)", in.ValueFrom.DataOutputName)
		}
		val, ready, err := r.checkSourceReady(ctx, s)
		if err != nil {
			return nil, errors.Wrap(err, "checkSourceReady failed")
		}
		if !ready {
			uds = append(uds, makeUnsatisfiedDependency(obj, s, in))
			return uds, nil
		}

		err = fillValue(obj, in.ToFieldPaths, val)
		if err != nil {
			return nil, errors.Wrap(err, "fillValue failed")
		}
	}
	return uds, nil
}

func fillValue(obj *unstructured.Unstructured, fs []string, val string) error {
	paved := fieldpath.Pave(obj.Object)
	for _, fp := range fs {
		err := paved.SetString(fp, val)
		if err != nil {
			return fmt.Errorf("paved.SetString() failed: %w", err)
		}
	}
	return nil
}

func (r *components) checkSourceReady(ctx context.Context, s *dagSource) (string, bool, error) {
	obj := s.ObjectRef
	key := types.NamespacedName{
		Namespace: obj.Namespace,
		Name:      obj.Name,
	}
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(obj.GroupVersionKind())
	err := r.client.Get(ctx, key, u)
	if err != nil {
		return "", false, errors.Wrap(resource.IgnoreNotFound(err), fmt.Sprintf("failed to get object (%s)", key.String()))
	}
	paved := fieldpath.Pave(u.UnstructuredContent())

	rawval, err := paved.GetValue(obj.FieldPath)
	if err != nil {
		if fieldpath.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to get field value (%s) in object (%s): %w", obj.FieldPath, key.String(), err)
	}

	switch val := rawval.(type) {
	case string:
		ok, err := matchValue(s.Conditions, val, paved)
		if err != nil {
			return val, false, err
		}
		if !ok {
			return val, false, nil
		}

		return val, true, nil
	default:
		// TODO: Currently only string value supported. Support more types in the future.
		return "", false, fmt.Errorf("unsupported field (%s) type: %T", obj.FieldPath, rawval)
	}

}

func matchValue(conds []v1alpha2.ConditionRequirement, val string, paved *fieldpath.Paved) (bool, error) {
	// If no matcher is specified, it is by default to check value not empty.
	if len(conds) == 0 {
		return val != "", nil
	}

	for _, m := range conds {
		var checkVal string
		if m.FieldPath != "" {
			var err error
			checkVal, err = paved.GetString(m.FieldPath)
			if err != nil {
				return false, err
			}
		} else {
			checkVal = val
		}

		switch m.Operator {
		case v1alpha2.ConditionEqual:
			if m.Value != checkVal {
				return false, nil
			}
		case v1alpha2.ConditionNotEqual:
			if m.Value == checkVal {
				return false, nil
			}
		case v1alpha2.ConditionNotEmpty:
			if checkVal == "" {
				return false, nil
			}
		}
	}
	return true, nil
}

// GetTraitName return trait name
func getTraitName(ac *v1alpha2.ApplicationConfiguration, componentName string,
	ct *v1alpha2.ComponentTrait, t *unstructured.Unstructured) string {
	var (
		traitName  string
		apiVersion string
		kind       string
	)

	if len(t.GetName()) > 0 {
		return t.GetName()
	}

	apiVersion = t.GetAPIVersion()
	kind = t.GetKind()

	for _, w := range ac.Status.Workloads {
		if w.ComponentName != componentName {
			continue
		}
		for _, trait := range w.Traits {
			if trait.Reference.APIVersion == apiVersion && trait.Reference.Kind == kind {
				traitName = trait.Reference.Name
			}
		}
	}

	if len(traitName) == 0 {
		traitName = util.GenTraitName(componentName, ct.DeepCopy())
	}

	return traitName
}
