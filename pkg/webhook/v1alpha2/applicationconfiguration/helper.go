package applicationconfiguration

import (
	"context"
	"encoding/json"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	acCtrl "github.com/crossplane/oam-kubernetes-runtime/pkg/controller/v1alpha2/applicationconfiguration"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

const (
	errUnmarshalTrait        = "cannot unmarshal trait"
	errUnmarshalWorkload     = "cannot unmarshal workload"
	errFmtGetTraitDefinition = "cannot find trait definition %q %q %q"
	errFmtSetParam           = "cannot set parameter %q"
	errFmtUnsupportedParam   = "unsupported parameter %q"
	errFmtRequiredParam      = "required parameter %q not specified"
)

// checkComponentVersionEnabled check whethter a component is versioning mechanism enabled
func checkComponentVersionEnabled(ctx context.Context, client client.Reader, acc *v1alpha2.ApplicationConfigurationComponent) (bool, error) {
	if acc.RevisionName != "" {
		// if revisionName is assigned
		// then the component is versioning enabled
		return true, nil
	}
	for _, ct := range acc.Traits {
		ut := &unstructured.Unstructured{}
		if err := json.Unmarshal(ct.Trait.Raw, ut); err != nil {
			return false, errors.Wrap(err, errUnmarshalTrait)
		}
		td, err := util.FetchTraitDefinition(ctx, client, ut)
		if err != nil {
			return false, errors.Wrapf(err, errFmtGetTraitDefinition, ut.GetAPIVersion(), ut.GetKind(), ut.GetName())
		}
		if td.Spec.RevisionEnabled {
			// if any traitDefinition's RevisionEnabled is true
			// then the component is versioning enabled
			return true, nil
		}
	}
	return false, nil
}

func getComponent(ctx context.Context, client client.Reader, acc v1alpha2.ApplicationConfigurationComponent, namespace string) (*v1alpha2.Component, error) {
	if acc.RevisionName != "" {
		acc.ComponentName = acCtrl.ExtractComponentName(acc.RevisionName)
		controllerRevision := &appsv1.ControllerRevision{}
		nk := types.NamespacedName{Namespace: namespace, Name: acc.RevisionName}
		if err := client.Get(ctx, nk, controllerRevision); err != nil {
			return nil, err
		}
		c, err := acCtrl.UnpackRevisionData(controllerRevision)
		if err != nil {
			return nil, err
		}
		return c, nil
	}
	c := &v1alpha2.Component{}
	nk := types.NamespacedName{Namespace: namespace, Name: acc.ComponentName}
	if err := client.Get(ctx, nk, c); err != nil {
		return nil, err
	}
	return c, nil
}

type parameter acCtrl.Parameter

func resolveParams(cp []v1alpha2.ComponentParameter, cpv []v1alpha2.ComponentParameterValue) ([]parameter, error) {
	//TODO(roywang) Maybe other validation will need resolve all params
	// Just keep it to resolve all params instead of only 'metadata.name'
	supported := make(map[string]bool)
	for _, v := range cp {
		supported[v.Name] = true
	}

	set := make(map[string]*parameter)
	for _, v := range cpv {
		if !supported[v.Name] {
			return nil, errors.Errorf(errFmtUnsupportedParam, v.Name)
		}
		set[v.Name] = &parameter{Name: v.Name, Value: v.Value}
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

	params := make([]parameter, 0, len(set))
	for _, p := range set {
		params = append(params, *p)
	}

	return params, nil
}

func renderWorkload(data []byte, p ...parameter) (*unstructured.Unstructured, error) {
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
