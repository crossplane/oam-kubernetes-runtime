package applicationconfiguration

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

const (
	errUnmarshalTrait        = "cannot unmarshal trait"
	errFmtGetTraitDefinition = "cannot find trait definition %q %q %q"
)

// checkComponentVersionEnabled check whethter a component is versioning mechanism enabled
func checkComponentVersionEnabled(ctx context.Context, client client.Reader, acc *v1alpha2.ApplicationConfigurationComponent) (bool, error) {
	if acc.RevisionName != "" {
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

// checkParams will check whethter exist parameter assigning value to workload name
func checkParams(cp []v1alpha2.ComponentParameter, cpv []v1alpha2.ComponentParameterValue) (bool, string) {
	targetParams := make(map[string]bool)
	for _, v := range cp {
		for _, fp := range v.FieldPaths {
			// only check metadata.name field parameter
			if strings.Contains(fp, WorkloadNamePath) {
				targetParams[v.Name] = true
			}
		}
	}
	for _, v := range cpv {
		if targetParams[v.Name] {
			// check fails if get parameter to overwrite workload name
			return false, v.Value.StrVal
		}
	}
	return true, ""
}
