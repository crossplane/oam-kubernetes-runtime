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
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientappv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"

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
	errFmtGetComponent           = "cannot get component %q"
	errFmtGetScope               = "cannot get scope %q"
	errFmtGetComponentRevision   = "cannot get component revision %q"
	errFmtResolveParams          = "cannot resolve parameter values for component %q"
	errFmtRenderWorkload         = "cannot render workload for component %q"
	errFmtRenderTrait            = "cannot render trait for component %q"
	errFmtSetParam               = "cannot set parameter %q"
	errFmtUnsupportedParam       = "unsupported parameter %q"
	errFmtRequiredParam          = "required parameter %q not specified"
	errFmtControllerRevisionData = "cannot get valid component data from controllerRevision %q"
	errFmtGetTraitDefinition     = "cannot find trait definition %q"
	errSetValueForField          = "can not set value %q for fieldPath %q"
)

const instanceNamePath = "metadata.name"

// A ComponentRenderer renders an ApplicationConfiguration's Components into
// workloads and traits.
type ComponentRenderer interface {
	Render(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) ([]Workload, error)
}

// A ComponentRenderFn renders an ApplicationConfiguration's Components into
// workloads and traits.
type ComponentRenderFn func(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) ([]Workload, error)

// Render an ApplicationConfiguration's Components into workloads and traits.
func (fn ComponentRenderFn) Render(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) ([]Workload, error) {
	return fn(ctx, ac)
}

type components struct {
	client     client.Reader
	appsClient clientappv1.AppsV1Interface
	params     ParameterResolver
	workload   ResourceRenderer
	trait      ResourceRenderer
}

func (r *components) Render(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) ([]Workload, error) {
	workloads := make([]Workload, len(ac.Spec.Components))
	for i, acc := range ac.Spec.Components {

		if acc.RevisionName != "" {
			acc.ComponentName = ExtractComponentName(acc.RevisionName)
		}
		c, componentRevisionName, err := r.getComponent(ctx, acc, ac.GetNamespace())
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

		ref := metav1.NewControllerRef(ac, v1alpha2.ApplicationConfigurationGroupVersionKind)
		w.SetOwnerReferences([]metav1.OwnerReference{*ref})
		w.SetNamespace(ac.GetNamespace())

		traits := make([]unstructured.Unstructured, len(acc.Traits))
		traitDefs := make([]v1alpha2.TraitDefinition, len(acc.Traits))
		for i, ct := range acc.Traits {
			t, err := r.trait.Render(ct.Trait.Raw)
			if err != nil {
				return nil, errors.Wrapf(err, errFmtRenderTrait, acc.ComponentName)
			}

			setTraitProperties(t, acc.ComponentName, ac.GetNamespace(), ref)

			traits[i] = *t

			traitDef, err := r.getTraitDefinition(ctx, t)
			if err != nil {
				return nil, err
			}
			traitDefs = append(traitDefs, traitDef)
		}
		if err := SetWorkloadInstanceName(traitDefs, w, c); err != nil {
			return nil, err
		}

		scopes := make([]unstructured.Unstructured, len(acc.Scopes))
		for i, cs := range acc.Scopes {
			// Get Scope instance from k8s, since it is global and not a child resource of workflow.
			scopeObject := unstructured.Unstructured{}
			scopeObject.SetAPIVersion(cs.ScopeReference.APIVersion)
			scopeObject.SetKind(cs.ScopeReference.Kind)
			scopeObjectRef := types.NamespacedName{Namespace: ac.GetNamespace(), Name: cs.ScopeReference.Name}
			if err := r.client.Get(ctx, scopeObjectRef, &scopeObject); err != nil {
				return nil, errors.Wrapf(err, errFmtGetScope, cs.ScopeReference.Name)
			}

			scopes[i] = scopeObject
		}

		workloads[i] = Workload{ComponentName: acc.ComponentName, ComponentRevisionName: componentRevisionName, Workload: w, Traits: traits, Scopes: scopes}
	}
	return workloads, nil
}

func setTraitProperties(t *unstructured.Unstructured, componentName, namespace string, ref *metav1.OwnerReference) {
	// Set metadata name for `Trait` if the metadata name is NOT set.
	if t.GetName() == "" {
		t.SetName(componentName)
	}

	t.SetOwnerReferences([]metav1.OwnerReference{*ref})
	t.SetNamespace(namespace)
}

func (r *components) getTraitDefinition(ctx context.Context, t *unstructured.Unstructured) (v1alpha2.TraitDefinition, error) {
	traitDefinitionName := getCRDName(t)
	traitDef := v1alpha2.TraitDefinition{}
	err := r.client.Get(ctx, types.NamespacedName{
		Name: traitDefinitionName,
	}, &traitDef)
	if err != nil {
		return traitDef, errors.Wrapf(err, errFmtGetTraitDefinition, traitDefinitionName)
	}
	return traitDef, nil
}

// SetWorkloadInstanceName will set metadata.name for workload CR according to createRevision flag in traitDefinition
func SetWorkloadInstanceName(traitDefs []v1alpha2.TraitDefinition, w *unstructured.Unstructured, c *v1alpha2.Component) error {
	//Don't override the specified name
	if w.GetName() != "" {
		return nil
	}
	pv := fieldpath.Pave(w.Object)
	if IsRevisionEnabled(traitDefs) {
		// if revisionEnabled, use revisionName as workload name
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

// IsRevisionEnabled will check if any trait has createRevision flag, the appconfig should create a new workload instance
func IsRevisionEnabled(traitDefs []v1alpha2.TraitDefinition) bool {
	for _, td := range traitDefs {
		if td.Spec.RevisionEnabled {
			return true
		}
	}
	return false
}

func getCRDName(u *unstructured.Unstructured) string {
	group, _ := util.APIVersion2GroupVersion(u.GetAPIVersion())
	resources := []string{util.Kind2Resource(u.GetKind())}
	if group != "" {
		resources = append(resources, group)
	}
	return strings.Join(resources, ".")
}

func (r *components) getComponent(ctx context.Context, acc v1alpha2.ApplicationConfigurationComponent, namespace string) (*v1alpha2.Component, string, error) {
	c := &v1alpha2.Component{}
	var revisionName string
	if acc.RevisionName != "" {
		revision, err := r.appsClient.ControllerRevisions(namespace).Get(ctx, acc.RevisionName, metav1.GetOptions{})
		if err != nil {
			return nil, "", errors.Wrapf(err, errFmtGetComponentRevision, acc.RevisionName)
		}
		c, err := UnpackRevisionData(revision)
		if err != nil {
			return nil, "", errors.Wrapf(err, errFmtControllerRevisionData, acc.RevisionName)
		}
		revisionName = acc.RevisionName
		return c, revisionName, nil
	}
	nn := types.NamespacedName{Namespace: namespace, Name: acc.ComponentName}
	if err := r.client.Get(ctx, nn, c); err != nil {
		return nil, "", errors.Wrapf(err, errFmtGetComponent, acc.ComponentName)
	}
	if c.Status.LatestRevision != nil {
		revisionName = c.Status.LatestRevision.Name
	}
	return c, revisionName, nil
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
