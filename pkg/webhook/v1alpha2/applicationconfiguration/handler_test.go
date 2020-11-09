package applicationconfiguration

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/mock"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

var _ = Describe("ApplicationConfiguration Admission controller Test", func() {
	var appConfig v1alpha2.ApplicationConfiguration
	var appConfigName string
	var label map[string]string
	BeforeEach(func() {
		label = map[string]string{"test": "test-appConfig"}
		// Create a appConfig definition
		appConfigName = "example-app"
		appConfig = v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:   appConfigName,
				Labels: label,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{
					{
						ComponentName: "example-comp",
						Traits:        make([]v1alpha2.ComponentTrait, 1),
					},
				},
			},
		}
	})

	Context("Test Mutation Webhook", func() {
		var handler admission.Handler = &MutatingHandler{}
		var traitDef v1alpha2.TraitDefinition
		var traitTypeName = "test-trait"
		var baseTrait unstructured.Unstructured

		BeforeEach(func() {
			decoderInjector := handler.(admission.DecoderInjector)
			decoderInjector.InjectDecoder(decoder)
			// define workloadDefinition
			traitDef = v1alpha2.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:   traitTypeName,
					Labels: label,
				},
				Spec: v1alpha2.TraitDefinitionSpec{
					Reference: v1alpha2.DefinitionReference{
						Name: "foos.example.com",
					},
				},
			}
			// the base trait
			baseTrait = unstructured.Unstructured{}
			baseTrait.SetAPIVersion("example.com/v1")
			baseTrait.SetKind("Foo")
			baseTrait.SetName("traitName")
			unstructured.SetNestedField(baseTrait.Object, "test", "spec", "key")
			Expect(len(crd.Spec.Versions)).Should(Equal(1))
			Expect(appConfig.Spec).NotTo(BeNil())
		})

		It("Test bad admission request format", func() {
			req := admission.Request{
				AdmissionRequest: admissionv1beta1.AdmissionRequest{
					Operation: admissionv1beta1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: []byte("bad request")},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeFalse())
		})

		It("Test noop mutate admission handle", func() {
			appConfig.Spec.Components[0].Traits[0].Trait = runtime.RawExtension{Raw: util.JSONMarshal(baseTrait)}

			req := admission.Request{
				AdmissionRequest: admissionv1beta1.AdmissionRequest{
					Operation: admissionv1beta1.Create,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: util.JSONMarshal(appConfig)},
				},
			}
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(BeTrue())
		})

		It("Test mutate function", func() {
			// the trait that uses type to refer to the traitDefinition
			traitWithType := unstructured.Unstructured{}
			typeContent := make(map[string]interface{})
			typeContent[TraitTypeField] = traitTypeName
			typeContent[TraitSpecField] = map[string]interface{}{
				"key": "test",
			}
			traitWithType.SetUnstructuredContent(typeContent)
			traitWithType.SetName("traitName")
			traitWithType.SetLabels(label)
			// set up the bad type
			traitWithBadType := traitWithType.DeepCopy()
			traitWithBadType.Object[TraitTypeField] = traitDef
			// set up the result
			mutatedTrait := baseTrait.DeepCopy()
			mutatedTrait.SetNamespace(appConfig.GetNamespace())
			mutatedTrait.SetLabels(util.MergeMapOverrideWithDst(label, map[string]string{oam.TraitTypeLabel: traitTypeName}))
			tests := map[string]struct {
				client client.Client
				trait  interface{}
				errMsg string
				wanted []byte
			}{
				"bad trait case": {
					trait:  "bad trait",
					errMsg: "cannot unmarshal string",
				},
				"bad trait type case": {
					trait:  traitWithBadType,
					errMsg: "name of trait should be string instead of map[string]interface {}",
				},
				"no op case": {
					trait:  baseTrait,
					wanted: util.JSONMarshal(baseTrait),
				},
				"update gvk get failed case": {
					client: &test.MockClient{
						MockGet: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
							switch obj.(type) {
							case *v1alpha2.TraitDefinition:
								return fmt.Errorf("does not exist")
							}
							return nil
						},
					},
					trait:  traitWithType.DeepCopyObject(),
					errMsg: "does not exist",
				},
				"update gvk and label case": {
					client: &test.MockClient{
						MockGet: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
							switch o := obj.(type) {
							case *v1alpha2.TraitDefinition:
								Expect(key.Name).Should(BeEquivalentTo(typeContent[TraitTypeField]))
								*o = traitDef
							case *crdv1.CustomResourceDefinition:
								Expect(key.Name).Should(Equal(traitDef.Spec.Reference.Name))
								*o = crd
							}
							return nil
						},
					},
					trait:  traitWithType.DeepCopyObject(),
					wanted: util.JSONMarshal(mutatedTrait),
				},
			}
			for testCase, test := range tests {
				By(fmt.Sprintf("start test : %s", testCase))
				appConfig.Spec.Components[0].Traits[0].Trait = runtime.RawExtension{Raw: util.JSONMarshal(test.trait)}
				injc := handler.(inject.Client)
				injc.InjectClient(test.client)
				mutatingHandler := handler.(*MutatingHandler)
				err := mutatingHandler.Mutate(&appConfig)
				if len(test.errMsg) == 0 {
					Expect(err).Should(BeNil())
					Expect(appConfig.Spec.Components[0].Traits[0].Trait.Raw).Should(BeEquivalentTo(test.wanted))
				} else {
					Expect(err.Error()).Should(ContainSubstring(test.errMsg))
				}
			}
		})
	})

	It("Test validating handler", func() {
		mapper := mock.NewMockDiscoveryMapper()
		var handler admission.Handler = &ValidatingHandler{Mapper: mapper}
		decoderInjector := handler.(admission.DecoderInjector)
		decoderInjector.InjectDecoder(decoder)
		By("Creating valid trait")
		validTrait := unstructured.Unstructured{}
		validTrait.SetAPIVersion("validAPI")
		validTrait.SetKind("validKind")
		By("Creating invalid trait with type")
		traitWithType := validTrait.DeepCopy()
		typeContent := make(map[string]interface{})
		typeContent[TraitTypeField] = "should not be here"
		traitWithType.SetUnstructuredContent(typeContent)
		By("Creating invalid trait without kind")
		noKindTrait := validTrait.DeepCopy()
		noKindTrait.SetKind("")
		var traitTypeName = "test-trait"
		traitDef := v1alpha2.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:   traitTypeName,
				Labels: label,
			},
			Spec: v1alpha2.TraitDefinitionSpec{
				Reference: v1alpha2.DefinitionReference{
					Name: "foos.example.com",
				},
			},
		}
		clientInstance := &test.MockClient{
			MockGet: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				switch o := obj.(type) {
				case *v1alpha2.TraitDefinition:
					*o = traitDef
				case *crdv1.CustomResourceDefinition:
					Expect(key.Name).Should(Equal(traitDef.Spec.Reference.Name))
					*o = crd
				}
				return nil
			},
		}
		tests := map[string]struct {
			trait     interface{}
			client    client.Client
			operation admissionv1beta1.Operation
			pass      bool
			reason    string
		}{
			"valid create case": {
				trait:     validTrait.DeepCopyObject(),
				operation: admissionv1beta1.Create,
				pass:      true,
				reason:    "",
				client:    clientInstance,
			},
			"valid update case": {
				trait:     validTrait.DeepCopyObject(),
				operation: admissionv1beta1.Update,
				pass:      true,
				reason:    "",
				client:    clientInstance,
			},
			"malformat appConfig": {
				trait:     "bad format",
				operation: admissionv1beta1.Create,
				pass:      false,
				reason:    "the trait is malformed",
				client:    clientInstance,
			},
			"trait still has type": {
				trait:     traitWithType.DeepCopyObject(),
				operation: admissionv1beta1.Create,
				pass:      false,
				reason:    "the trait contains 'name' info",
				client:    clientInstance,
			},
			"no kind trait appConfig": {
				trait:     noKindTrait.DeepCopyObject(),
				operation: admissionv1beta1.Update,
				pass:      false,
				reason:    "the trait data missing GVK",
				client:    clientInstance,
			},
		}
		for testCase, test := range tests {
			By(fmt.Sprintf("start test : %s", testCase))
			appConfig.Spec.Components[0].Traits[0].Trait = runtime.RawExtension{Raw: util.JSONMarshal(test.trait)}
			req := admission.Request{
				AdmissionRequest: admissionv1beta1.AdmissionRequest{
					Operation: test.operation,
					Resource:  reqResource,
					Object:    runtime.RawExtension{Raw: util.JSONMarshal(appConfig)},
				},
			}
			injc := handler.(inject.Client)
			injc.InjectClient(test.client)
			resp := handler.Handle(context.TODO(), req)
			Expect(resp.Allowed).Should(Equal(test.pass))
			if !test.pass {
				Expect(string(resp.Result.Reason)).Should(ContainSubstring(test.reason))
			}
		}
		By("Test bad admission request format")
		req := admission.Request{
			AdmissionRequest: admissionv1beta1.AdmissionRequest{
				Operation: admissionv1beta1.Create,
				Resource:  reqResource,
				Object:    runtime.RawExtension{Raw: []byte("bad request")},
			},
		}
		resp := handler.Handle(context.TODO(), req)
		Expect(resp.Allowed).Should(BeFalse())
	})

})
