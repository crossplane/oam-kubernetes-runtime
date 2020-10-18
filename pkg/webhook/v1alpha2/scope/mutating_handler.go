/*
Copyright 2019 The Kruise Authors.

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

package scope

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

const (
	// TypeField is the special field indicate the type of the workloadDefinition
	TypeField    = "type"
	DefaultScope = "scope.oam.dev"
)

// MutatingHandler handles Component
type MutatingHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

// log is for logging in this package.
var mutatelog = logf.Log.WithName("category scope mutate webhook")

var _ admission.Handler = &MutatingHandler{}

// Handle handles admission requests.
func (h *MutatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &unstructured.Unstructured{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// mutate the object
	if err := h.Mutate(obj); err != nil {
		mutatelog.Error(err, "failed to mutate the category scope", "name", obj.GetName())
		return admission.Errored(http.StatusBadRequest, err)
	}
	mutatelog.Info("Print the mutated obj", "obj name", obj.GetName(), "mutated obj", obj.GetLabels())

	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	resp := admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshalled)
	if len(resp.Patches) > 0 {
		mutatelog.Info("admit category scope",
			"namespace", obj.GetNamespace(), "name", obj.GetName(), "patches", util.JSONMarshal(resp.Patches))
	}
	return resp
}

// Mutate sets all the default value for the Component
func (h *MutatingHandler) Mutate(obj *unstructured.Unstructured) error {

	scopeName := getCategoryScope(obj)
	namespace := obj.GetNamespace()

	scopeDefinition := &v1alpha2.CategoryScope{}
	if err := h.Client.Get(context.TODO(), types.NamespacedName{Name: scopeName}, scopeDefinition); err != nil {
		return err
	}

	newLabels := mergerMap(obj.GetLabels(), h.ParseScopeVars(scopeDefinition.Spec.Labels, namespace))
	newAnnotations := mergerMap(obj.GetAnnotations(), h.ParseScopeVars(scopeDefinition.Spec.Annotation, namespace))

	obj.SetLabels(newLabels)
	obj.SetAnnotations(newAnnotations)

	return nil
}

// ParseScopeVars parse scope vars
func (h *MutatingHandler) ParseScopeVars(scopes []v1alpha2.ScopeVar, namespace string) map[string]string {
	labels := make(map[string]string)
	for _, scopeVar := range scopes {

		if len(scopeVar.Key) != 0 && len(scopeVar.Value) != 0 {
			labels[scopeVar.Key] = scopeVar.Value
			continue
		}

		if scopeVar.ValueFrom == nil {
			mutatelog.Info("skip parse valueFrom", "key", scopeVar.Key, "value", scopeVar.Value)
			continue
		}

		mutatelog.Info("valueFrom", "value", scopeVar.ValueFrom)

		value, err := GetScopeVarRefValue(h.Client, namespace, scopeVar.ValueFrom)
		if err != nil {
			mutatelog.Error(err, "skip parse valueFrom", "key", scopeVar.Key, "value", value)
			continue
		}

		labels[scopeVar.Key] = value

		mutatelog.Info("mutate", "valueFrom", value)
	}
	return labels
}

func mergerMap(labels, sLabels map[string]string) map[string]string {
	newLabels := make(map[string]string)
	for key, value := range labels {
		for k, v := range sLabels {
			if k == key && v == value {
				continue
			}
			newLabels[k] = v
		}
		newLabels[key] = value
	}
	return newLabels
}

func getCategoryScope(obj *unstructured.Unstructured) string {
	var scope string
	for key, value := range obj.GetLabels() {
		if key == DefaultScope {
			scope = value
		}
	}
	return strings.Split(scope, ".")[0]
}

// getSecretRefValue returns the value of a secret in the supplied namespace
func getSecretRefValue(client client.Client, namespace string, secretSelector *corev1.SecretKeySelector) (string, error) {
	secret := &corev1.Secret{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: secretSelector.Name, Namespace: namespace}, secret)
	if err != nil {
		return "", err
	}

	if data, ok := secret.Data[secretSelector.Key]; ok {
		return string(data), nil
	}
	return "", fmt.Errorf("key %s not found in secret %s", secretSelector.Key, secretSelector.Name)

}

// getConfigMapRefValue returns the value of a configmap in the supplied namespace
func getConfigMapRefValue(client client.Client, namespace string, configMapSelector *corev1.ConfigMapKeySelector) (string, error) {
	configMap := &corev1.ConfigMap{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: configMapSelector.Name, Namespace: namespace}, configMap)
	if err != nil {
		return "", err
	}

	if data, ok := configMap.Data[configMapSelector.Key]; ok {
		return data, nil
	}
	return "", fmt.Errorf("key %s not found in config map %s", configMapSelector.Key, configMapSelector.Name)
}

// GetScopeVarRefValue returns the value referenced by the supplied ScopeSource given the other supplied information.
func GetScopeVarRefValue(kc client.Client, ns string, from *v1alpha2.ScopeSource) (string, error) {
	if from.SecretKeyRef != nil {
		return getSecretRefValue(kc, ns, from.SecretKeyRef)
	}

	if from.ConfigMapKeyRef != nil {
		return getConfigMapRefValue(kc, ns, from.ConfigMapKeyRef)
	}

	return "", fmt.Errorf("invalid valueFrom")
}

var _ inject.Client = &MutatingHandler{}

// InjectClient injects the client into the ComponentMutatingHandler
func (h *MutatingHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

var _ admission.DecoderInjector = &MutatingHandler{}

// InjectDecoder injects the decoder into the ComponentMutatingHandler
func (h *MutatingHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}

// RegisterMutatingHandler will register component mutation handler to the webhook
func RegisterMutatingHandler(mgr manager.Manager) {
	server := mgr.GetWebhookServer()
	server.Register("/mutating-core-oam-dev-v1alpha2-category-scope", &webhook.Admission{Handler: &MutatingHandler{}})
}
