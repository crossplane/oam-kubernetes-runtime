package applicationconfiguration

import (
	"context"
	"fmt"
	"net/http"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var appConfigResource = v1alpha2.SchemeGroupVersion.WithResource("applicationconfigurations")

// ValidatingHandler handles CloneSet
type ValidatingHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &ValidatingHandler{}

//Handle validate ApplicationConfiguration Spec here
func (h *ValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &v1alpha2.ApplicationConfiguration{}
	klog.Info("admitting Application Configuration")

	if req.Resource.String() != appConfigResource.String() {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("expect resource to be %s", appConfigResource))
	}
	switch req.Operation {
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
	}

	if pass, reason := checkRevisionName(obj); !pass {
		return admission.ValidationResponse(false, reason)
	}
	// TODO(wonderflow): Add more validation logic here.

	return admission.ValidationResponse(true, "")
}

func checkRevisionName(appConfig *v1alpha2.ApplicationConfiguration) (bool, string) {
	for _, v := range appConfig.Spec.Components {
		if v.ComponentName != "" && v.RevisionName != "" {
			return false, "componentName and revisionName are mutually exclusive, you can only specify one of them"
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

// Register will regsiter application configuration validation to webhook
func Register(server *webhook.Server) {
	server.Register("/validating-applicationconfigurations", &webhook.Admission{Handler: &ValidatingHandler{}})
}
