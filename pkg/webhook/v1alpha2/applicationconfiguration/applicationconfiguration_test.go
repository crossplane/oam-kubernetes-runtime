package applicationconfiguration

import (
	"context"
	"testing"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/mock"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	json "github.com/json-iterator/go"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestApplicationConfigurationValidation(t *testing.T) {
	var handler admission.Handler = &ValidatingHandler{}
	mgr := &mock.Manager{
		Client: &test.MockClient{},
	}
	resource := metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applicationconfigurations"}
	injc := handler.(inject.Client)
	injc.InjectClient(mgr.GetClient())
	decoder := handler.(admission.DecoderInjector)
	var scheme = runtime.NewScheme()
	_ = core.AddToScheme(scheme)
	dec, _ := admission.NewDecoder(scheme)
	decoder.InjectDecoder(dec)

	app1, _ := json.Marshal(v1alpha2.ApplicationConfiguration{Spec: v1alpha2.ApplicationConfigurationSpec{Components: []v1alpha2.ApplicationConfigurationComponent{
		{
			RevisionName:  "r1",
			ComponentName: "c1",
		},
	}}})
	app2, _ := json.Marshal(v1alpha2.ApplicationConfiguration{Spec: v1alpha2.ApplicationConfigurationSpec{Components: []v1alpha2.ApplicationConfigurationComponent{
		{
			RevisionName: "r1",
		},
	}}})

	tests := []struct {
		req    admission.Request
		pass   bool
		reason string
	}{
		{
			req: admission.Request{
				AdmissionRequest: admissionv1beta1.AdmissionRequest{
					Resource: resource,
					Object:   runtime.RawExtension{Raw: app1},
				},
			},
			pass:   false,
			reason: "componentName and revisionName are mutually exclusive, you can only specify one of them",
		},
		{
			req: admission.Request{
				AdmissionRequest: admissionv1beta1.AdmissionRequest{
					Resource: resource,
					Object:   runtime.RawExtension{Raw: app2},
				},
			},
			pass: true,
		},
	}
	for _, tv := range tests {
		resp := handler.Handle(context.Background(), tv.req)
		if tv.pass != resp.Allowed {
			t.Errorf("expect %v but got %v from validation", tv.pass, resp.Allowed)
		}
		if tv.reason != "" {
			if tv.reason != string(resp.Result.Reason) {
				t.Errorf("\nvalidation should fail by reason: %v \ninstead of by reason: %v ", tv.reason, resp.Result.Reason)
			}
		}
	}
}
