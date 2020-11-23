package applicationconfiguration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/pkg/errors"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/controller/v1alpha2/applicationconfiguration"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/discoverymapper"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

const (
	reasonFmtWorkloadNameNotEmpty = "Versioning-enabled component's workload name MUST NOT be assigned. Expect workload name %q to be empty."

	errFmtCheckWorkloadName = "Error occurs when checking workload name. %q"

	errFmtUnmarshalWorkload = "Error occurs when unmarshal workload of component %q error: %q"

	// WorkloadNamePath indicates field path of workload name
	WorkloadNamePath = "metadata.name"

	// ErrConflictsWith marks as the identifier of Trait conflict error
	ErrConflictsWith = "TraitConflictError"

	// ErrFmtConflictsTrait formats the error message for Traits conflict
	ErrFmtConflictsTrait = "cannot apply trait %q %q %q whose conflictsWith is %q"
)

var appConfigResource = v1alpha2.SchemeGroupVersion.WithResource("applicationconfigurations")

// ValidatingHandler handles CloneSet
type ValidatingHandler struct {
	Client client.Client
	Mapper discoverymapper.DiscoveryMapper

	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &ValidatingHandler{}

// Handle validate ApplicationConfiguration Spec here
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &v1alpha2.ApplicationConfiguration{}
	if req.Resource.String() != appConfigResource.String() {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("expect resource to be %s", appConfigResource))
	}
	switch req.Operation { //nolint:exhaustive
	case admissionv1beta1.Delete:
		if len(req.OldObject.Raw) != 0 {
			if err := h.Decoder.DecodeRaw(req.OldObject, obj); err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}
		} else {
			// TODO(wonderflow): we can audit delete or something else here.
			klog.Info("deleting Application Configuration", req.Name)
		}
	default:
		err := h.Decoder.Decode(req, obj)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if allErrs := ValidateTraitObject(ctx, h.Client, h.Mapper, obj); len(allErrs) > 0 {
			klog.Info("create or update failed", "name", obj.Name, "errMsg", allErrs.ToAggregate().Error())
			return admission.Denied(allErrs.ToAggregate().Error())
		}
		if pass, reason := checkRevisionName(obj); !pass {
			return admission.ValidationResponse(false, reason)
		}
		if pass, reason := checkWorkloadNameForVersioning(ctx, h.Client, h.Mapper, obj); !pass {
			return admission.ValidationResponse(false, reason)
		}
		// TODO(wonderflow): Add more validation logic here.
	}
	return admission.ValidationResponse(true, "")
}

// ValidateTraitObject validates the ApplicationConfiguration on creation/update
func ValidateTraitObject(ctx context.Context, client client.Reader, dm discoverymapper.DiscoveryMapper, obj *v1alpha2.ApplicationConfiguration) field.ErrorList {
	klog.Info("validate applicationConfiguration", "name", obj.Name)
	var allErrs field.ErrorList
	for cidx, comp := range obj.Spec.Components {
		componentName := comp.ComponentName
		compatibleTraits, err := getAppliedTraits(obj, componentName)
		if err != nil {
			fldPath := field.NewPath("spec").Child("components").Index(cidx).Child("traits")
			allErrs = append(allErrs, field.NotFound(fldPath, componentName))
			return allErrs
		}
		var preAppliedTraits = make([]unstructured.Unstructured, 0)
		for idx, tr := range comp.Traits {
			fldPath := field.NewPath("spec").Child("components").Index(cidx).Child("traits").Index(idx).Child("trait")
			var content map[string]interface{}
			if err := json.Unmarshal(tr.Trait.Raw, &content); err != nil {
				allErrs = append(allErrs, field.Invalid(fldPath, string(tr.Trait.Raw),
					"the trait is malformed"))
				return allErrs
			}
			if content[TraitTypeField] != nil {
				allErrs = append(allErrs, field.Invalid(fldPath, string(tr.Trait.Raw),
					"the trait contains 'name' info that should be mutated to GVK"))
			}
			if content[TraitSpecField] != nil {
				allErrs = append(allErrs, field.Invalid(fldPath, string(tr.Trait.Raw),
					"the trait contains 'properties' info that should be mutated to spec"))
			}
			trait := unstructured.Unstructured{
				Object: content,
			}
			if len(trait.GetAPIVersion()) == 0 || len(trait.GetKind()) == 0 {
				allErrs = append(allErrs, field.Invalid(fldPath, content,
					fmt.Sprintf("the trait data missing GVK, api = %s, kind = %s,", trait.GetAPIVersion(), trait.GetKind())))
			}
			compatibleTraits = append(compatibleTraits, preAppliedTraits...)
			t, err := CheckTraitsConflict(ctx, client, dm, tr, compatibleTraits)
			if err != nil {
				allErrs = append(allErrs, field.Invalid(fldPath, err, "the trait is conflicted"))
				return allErrs
			}
			if t != nil {
				preAppliedTraits = append(preAppliedTraits, *t)
			}
		}
	}

	return allErrs
}

func checkRevisionName(appConfig *v1alpha2.ApplicationConfiguration) (bool, string) {
	for _, v := range appConfig.Spec.Components {
		if v.ComponentName != "" && v.RevisionName != "" {
			return false, "componentName and revisionName are mutually exclusive, you can only specify one of them"
		}
	}
	return true, ""
}

// checkWorkloadNameForVersioning check whether versioning-enabled component workload name is empty
func checkWorkloadNameForVersioning(ctx context.Context, client client.Reader, dm discoverymapper.DiscoveryMapper,
	appConfig *v1alpha2.ApplicationConfiguration) (bool, string) {
	for _, v := range appConfig.Spec.Components {
		acc := v
		vEnabled, err := checkComponentVersionEnabled(ctx, client, dm, &acc)
		if err != nil {
			return false, fmt.Sprintf(errFmtCheckWorkloadName, err.Error())
		}
		if !vEnabled {
			continue
		}
		c, _, err := util.GetComponent(ctx, client, acc, appConfig.GetNamespace())
		if err != nil {
			return false, fmt.Sprintf(errFmtCheckWorkloadName, err.Error())
		}

		if ok, workloadName := checkParams(c.Spec.Parameters, acc.ParameterValues); !ok {
			return false, fmt.Sprintf(reasonFmtWorkloadNameNotEmpty, workloadName)
		}

		w := &fieldpath.Paved{}
		if err := json.Unmarshal(c.Spec.Workload.Raw, w); err != nil {
			return false, fmt.Sprintf(errFmtUnmarshalWorkload, c.GetName(), err.Error())
		}
		workload := unstructured.Unstructured{Object: w.UnstructuredContent()}
		workloadName := workload.GetName()

		if len(workloadName) != 0 {
			return false, fmt.Sprintf(reasonFmtWorkloadNameNotEmpty, workloadName)
		}
	}
	return true, ""
}

var _ inject.Client = &ValidatingHandler{}

// InjectClient injects the client into the ValidatingHandler
func (h *ValidatingHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

var _ admission.DecoderInjector = &ValidatingHandler{}

// InjectDecoder injects the decoder into the ValidatingHandler
func (h *ValidatingHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}

// RegisterValidatingHandler will register application configuration validation to webhook
func RegisterValidatingHandler(mgr manager.Manager) error {
	server := mgr.GetWebhookServer()
	mapper, err := discoverymapper.New(mgr.GetConfig())
	if err != nil {
		return err
	}
	server.Register("/validating-core-oam-dev-v1alpha2-applicationconfigurations", &webhook.Admission{Handler: &ValidatingHandler{
		Mapper: mapper,
	}})
	return nil
}

// CheckTraitsConflict checks whether traits are conflicted
func CheckTraitsConflict(ctx context.Context, client client.Reader, dm discoverymapper.DiscoveryMapper,
	ct v1alpha2.ComponentTrait, compatibleTraits []unstructured.Unstructured) (*unstructured.Unstructured, error) {
	t, err := applicationconfiguration.RenderTrait(ct.Trait.Raw)
	if err != nil {
		return nil, nil
	}
	traitDef, err := util.FetchTraitDefinition(ctx, client, dm, t)
	if err != nil {
		return nil, err
	}

	for _, conflict := range traitDef.Spec.ConflictsWith {
		for j := range compatibleTraits {
			compatibleTraitDef, err := util.FetchTraitDefinition(ctx, client, dm, &compatibleTraits[j])
			if err != nil {
				return nil, err
			}
			if conflict == compatibleTraitDef.Name {
				err := errors.New(ErrConflictsWith)
				return nil, errors.Wrapf(err, ErrFmtConflictsTrait, t.GetAPIVersion(), t.GetKind(), t.GetName(), conflict)
			}
		}
	}
	return t, nil
}

// GetAppliedTraits gets all the traits which is already applied to a specified component
func getAppliedTraits(ac *v1alpha2.ApplicationConfiguration, componentName string) ([]unstructured.Unstructured, error) {
	var traits []v1alpha2.ComponentTrait
	var appliedTraits = make([]unstructured.Unstructured, 0)
	deployedComponents := make([]string, 0)
	for _, w := range ac.Status.Workloads {
		deployedComponents = append(deployedComponents, w.ComponentName)
	}
	for _, c := range ac.Spec.Components {
		if c.ComponentName != componentName || !util.Contains(deployedComponents, c.ComponentName) {
			continue
		}
		traits = c.Traits
		break
	}
	for _, t := range traits {
		unstructuredTrait, err := applicationconfiguration.RenderTrait(t.Trait.Raw)
		if err != nil {
			return nil, err
		}
		appliedTraits = append(appliedTraits, *unstructuredTrait)
	}
	return appliedTraits, nil
}
