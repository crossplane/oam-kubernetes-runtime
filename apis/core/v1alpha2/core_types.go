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

package v1alpha2

import (
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// A DefinitionReference refers to a CustomResourceDefinition by name.
type DefinitionReference struct {
	// Name of the referenced CustomResourceDefinition.
	Name string `json:"name"`
}

// A ChildResourceKind defines a child Kubernetes resource kind with a selector
type ChildResourceKind struct {
	// APIVersion of the child resource
	APIVersion string `json:"apiVersion"`

	// Kind of the child resource
	Kind string `json:"kind"`

	// Selector to select the child resources that the workload wants to expose to traits
	Selector map[string]string `json:"selector,omitempty"`
}

// A WorkloadDefinitionSpec defines the desired state of a WorkloadDefinition.
type WorkloadDefinitionSpec struct {
	// Reference to the CustomResourceDefinition that defines this workload kind.
	Reference DefinitionReference `json:"definitionRef"`

	// ChildResourceKinds are the list of GVK of the child resources this workload generates
	ChildResourceKinds []ChildResourceKind `json:"childResourceKinds,omitempty"`

	// Extension is used for extension needs by OAM platform builders
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Extension *runtime.RawExtension `json:"extension,omitempty"`
}

// +kubebuilder:object:root=true

// A WorkloadDefinition registers a kind of Kubernetes custom resource as a
// valid OAM workload kind by referencing its CustomResourceDefinition. The CRD
// is used to validate the schema of the workload when it is embedded in an OAM
// Component.
// +kubebuilder:printcolumn:JSONPath=".spec.definitionRef.name",name=DEFINITION-NAME,type=string
// +kubebuilder:resource:scope=Cluster,categories={crossplane,oam}
type WorkloadDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec WorkloadDefinitionSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// WorkloadDefinitionList contains a list of WorkloadDefinition.
type WorkloadDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkloadDefinition `json:"items"`
}

// A TraitDefinitionSpec defines the desired state of a TraitDefinition.
type TraitDefinitionSpec struct {
	// Reference to the CustomResourceDefinition that defines this trait kind.
	Reference DefinitionReference `json:"definitionRef"`

	// Revision indicates whether a trait is aware of component revision
	// +optional
	RevisionEnabled bool `json:"revisionEnabled,omitempty"`

	// WorkloadRefPath indicates where/if a trait accepts a workloadRef object
	// +optional
	WorkloadRefPath string `json:"workloadRefPath,omitempty"`

	// AppliesToWorkloads specifies the list of workload kinds this trait
	// applies to. Workload kinds are specified in kind.group/version format,
	// e.g. server.core.oam.dev/v1alpha2. Traits that omit this field apply to
	// all workload kinds.
	// +optional
	AppliesToWorkloads []string `json:"appliesToWorkloads,omitempty"`

	// Extension is used for extension needs by OAM platform builders
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Extension *runtime.RawExtension `json:"extension,omitempty"`
}

// +kubebuilder:object:root=true

// A TraitDefinition registers a kind of Kubernetes custom resource as a valid
// OAM trait kind by referencing its CustomResourceDefinition. The CRD is used
// to validate the schema of the trait when it is embedded in an OAM
// ApplicationConfiguration.
// +kubebuilder:printcolumn:JSONPath=".spec.definitionRef.name",name=DEFINITION-NAME,type=string
// +kubebuilder:resource:scope=Cluster,categories={crossplane,oam}
type TraitDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec TraitDefinitionSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// TraitDefinitionList contains a list of TraitDefinition.
type TraitDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TraitDefinition `json:"items"`
}

// A ScopeDefinitionSpec defines the desired state of a ScopeDefinition.
type ScopeDefinitionSpec struct {
	// Reference to the CustomResourceDefinition that defines this scope kind.
	Reference DefinitionReference `json:"definitionRef"`

	// WorkloadRefsPath indicates if/where a scope accepts workloadRef objects
	WorkloadRefsPath string `json:"workloadRefsPath,omitempty"`

	// AllowComponentOverlap specifies whether an OAM component may exist in
	// multiple instances of this kind of scope.
	AllowComponentOverlap bool `json:"allowComponentOverlap"`

	// Extension is used for extension needs by OAM platform builders
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Extension *runtime.RawExtension `json:"extension,omitempty"`
}

// +kubebuilder:object:root=true

// A ScopeDefinition registers a kind of Kubernetes custom resource as a valid
// OAM scope kind by referencing its CustomResourceDefinition. The CRD is used
// to validate the schema of the scope when it is embedded in an OAM
// ApplicationConfiguration.
// +kubebuilder:printcolumn:JSONPath=".spec.definitionRef.name",name=DEFINITION-NAME,type=string
// +kubebuilder:resource:scope=Cluster,categories={crossplane,oam}
type ScopeDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ScopeDefinitionSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// ScopeDefinitionList contains a list of ScopeDefinition.
type ScopeDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScopeDefinition `json:"items"`
}

// A ComponentParameter defines a configurable parameter of a component.
type ComponentParameter struct {
	// Name of this parameter. OAM ApplicationConfigurations will specify
	// parameter values using this name.
	Name string `json:"name"`

	// FieldPaths specifies an array of fields within this Component's workload
	// that will be overwritten by the value of this parameter. The type of the
	// parameter (e.g. int, string) is inferred from the type of these fields;
	// All fields must be of the same type. Fields are specified as JSON field
	// paths without a leading dot, for example 'spec.replicas'.
	FieldPaths []string `json:"fieldPaths"`

	// TODO(negz): Use +kubebuilder:default marker to default Required to false
	// once we're generating v1 CRDs.

	// Required specifies whether or not a value for this parameter must be
	// supplied when authoring an ApplicationConfiguration.
	// +optional
	Required *bool `json:"required,omitempty"`

	// Description of this parameter.
	// +optional
	Description *string `json:"description,omitempty"`
}

// A ComponentSpec defines the desired state of a Component.
type ComponentSpec struct {
	// A Workload that will be created for each ApplicationConfiguration that
	// includes this Component. Workloads must be defined by a
	// WorkloadDefinition.
	// +kubebuilder:validation:EmbeddedResource
	// +kubebuilder:pruning:PreserveUnknownFields
	Workload runtime.RawExtension `json:"workload"`

	// Parameters exposed by this component. ApplicationConfigurations that
	// reference this component may specify values for these parameters, which
	// will in turn be injected into the embedded workload.
	// +optional
	Parameters []ComponentParameter `json:"parameters,omitempty"`
}

// A ComponentStatus represents the observed state of a Component.
type ComponentStatus struct {
	runtimev1alpha1.ConditionedStatus `json:",inline"`

	// LatestRevision of component
	// +optional
	LatestRevision *Revision `json:"latestRevision,omitempty"`

	// TODO(negz): Maintain references to any ApplicationConfigurations that
	// reference this component? Doing so would allow us to queue a reconcile
	// for consuming ApplicationConfigurations when this Component changed.
}

// Revision has name and revision number
type Revision struct {
	Name     string `json:"name"`
	Revision int64  `json:"revision"`
}

// +kubebuilder:object:root=true

// A Component describes how an OAM workload kind may be instantiated.
// +kubebuilder:resource:categories={crossplane,oam}
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".spec.workload.kind",name=WORKLOAD-KIND,type=string
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
type Component struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentSpec   `json:"spec,omitempty"`
	Status ComponentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComponentList contains a list of Component.
type ComponentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Component `json:"items"`
}

// A ComponentParameterValue specifies a value for a named parameter. The
// associated component must publish a parameter with this name.
type ComponentParameterValue struct {
	// Name of the component parameter to set.
	Name string `json:"name"`

	// Value to set.
	Value intstr.IntOrString `json:"value"`
}

// A ComponentTrait specifies a trait that should be applied to a component.
type ComponentTrait struct {
	// A Trait that will be created for the component
	// +kubebuilder:validation:EmbeddedResource
	// +kubebuilder:pruning:PreserveUnknownFields
	Trait runtime.RawExtension `json:"trait"`

	// DataOutputs specify the data output sources from this trait.
	// +optional
	DataOutputs []DataOutput `json:"dataOutputs,omitempty"`

	// DataInputs specify the data input sinks into this trait.
	// +optional
	DataInputs []DataInput `json:"dataInputs,omitempty"`
}

// A ComponentScope specifies a scope in which a component should exist.
type ComponentScope struct {
	// A ScopeReference must refer to an OAM scope resource.
	ScopeReference runtimev1alpha1.TypedReference `json:"scopeRef"`
}

// An ApplicationConfigurationComponent specifies a component of an
// ApplicationConfiguration. Each component is used to instantiate a workload.
type ApplicationConfigurationComponent struct {
	// ComponentName specifies a component whose latest revision will be bind
	// with ApplicationConfiguration. When the spec of the referenced component
	// changes, ApplicationConfiguration will automatically migrate all trait
	// affect from the prior revision to the new one. This is mutually exclusive
	// with RevisionName.
	// +optional
	ComponentName string `json:"componentName,omitempty"`

	// RevisionName of a specific component revision to which to bind
	// ApplicationConfiguration. This is mutually exclusive with componentName.
	// +optional
	RevisionName string `json:"revisionName,omitempty"`

	// DataOutputs specify the data output sources from this component.
	DataOutputs []DataOutput `json:"dataOutputs,omitempty"`

	// DataInputs specify the data input sinks into this component.
	DataInputs []DataInput `json:"dataInputs,omitempty"`

	// ParameterValues specify values for the the specified component's
	// parameters. Any parameter required by the component must be specified.
	// +optional
	ParameterValues []ComponentParameterValue `json:"parameterValues,omitempty"`

	// Traits of the specified component.
	// +optional
	Traits []ComponentTrait `json:"traits,omitempty"`

	// Scopes in which the specified component should exist.
	// +optional
	Scopes []ComponentScope `json:"scopes,omitempty"`
}

// An ApplicationConfigurationSpec defines the desired state of a
// ApplicationConfiguration.
type ApplicationConfigurationSpec struct {
	// Components of which this ApplicationConfiguration consists. Each
	// component will be used to instantiate a workload.
	Components []ApplicationConfigurationComponent `json:"components"`
}

// A TraitStatus represents the state of a trait.
type TraitStatus string

// A WorkloadTrait represents a trait associated with a workload and its status
type WorkloadTrait struct {
	// Status is a place holder for a customized controller to fill
	// if it needs a single place to summarize the status of the trait
	Status TraitStatus `json:"status,omitempty"`

	// Reference to a trait created by an ApplicationConfiguration.
	Reference runtimev1alpha1.TypedReference `json:"traitRef"`
}

// A ScopeStatus represents the state of a scope.
type ScopeStatus string

// A WorkloadScope represents a scope associated with a workload and its status
type WorkloadScope struct {
	// Status is a place holder for a customized controller to fill
	// if it needs a single place to summarize the status of the scope
	Status ScopeStatus `json:"status,omitempty"`

	// Reference to a scope created by an ApplicationConfiguration.
	Reference runtimev1alpha1.TypedReference `json:"scopeRef"`
}

// A WorkloadStatus represents the status of a workload.
type WorkloadStatus struct {
	// Status is a place holder for a customized controller to fill
	// if it needs a single place to summarize the entire status of the workload
	Status string `json:"status,omitempty"`

	// HistoryWorkingRevision is a flag showing if it's history revision but still working
	HistoryWorkingRevision bool `json:"currentWorkingRevision,omitempty"`

	// ComponentName that produced this workload.
	ComponentName string `json:"componentName,omitempty"`

	//ComponentRevisionName of current component
	ComponentRevisionName string `json:"componentRevisionName,omitempty"`

	// Reference to a workload created by an ApplicationConfiguration.
	Reference runtimev1alpha1.TypedReference `json:"workloadRef,omitempty"`

	// Traits associated with this workload.
	Traits []WorkloadTrait `json:"traits,omitempty"`

	// Scopes associated with this workload.
	Scopes []WorkloadScope `json:"scopes,omitempty"`
}

// A ApplicationStatus represents the state of the entire application.
type ApplicationStatus string

// An ApplicationConfigurationStatus represents the observed state of a
// ApplicationConfiguration.
type ApplicationConfigurationStatus struct {
	runtimev1alpha1.ConditionedStatus `json:",inline"`

	// Status is a place holder for a customized controller to fill
	// if it needs a single place to summarize the status of the entire application
	Status ApplicationStatus `json:"status,omitempty"`

	Dependency DependencyStatus `json:"dependency"`

	// Workloads created by this ApplicationConfiguration.
	Workloads []WorkloadStatus `json:"workloads,omitempty"`
}

// DependencyStatus represents the observed state of the dependency of
// an ApplicationConfiguration.
type DependencyStatus struct {
	Unsatisfied []UnstaifiedDependency `json:"unsatisfied,omitempty"`
}

// UnstaifiedDependency describes unsatisfied dependency flow between
// one pair of objects.
type UnstaifiedDependency struct {
	From DependencyFromObject `json:"from"`
	To   DependencyToObject   `json:"to"`
}

// DependencyFromObject represents the object that dependency data comes from.
type DependencyFromObject struct {
	runtimev1alpha1.TypedReference `json:",inline"`
	FieldPath                      string `json:"fieldPath,omitempty"`
}

// DependencyToObject represents the object that dependency data goes to.
type DependencyToObject struct {
	runtimev1alpha1.TypedReference `json:",inline"`
	FieldPaths                     []string `json:"fieldPaths,omitempty"`
}

// +kubebuilder:object:root=true

// An ApplicationConfiguration represents an OAM application.
// +kubebuilder:resource:shortName=appconfig,categories={crossplane,oam}
// +kubebuilder:subresource:status
type ApplicationConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationConfigurationSpec   `json:"spec,omitempty"`
	Status ApplicationConfigurationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ApplicationConfigurationList contains a list of ApplicationConfiguration.
type ApplicationConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApplicationConfiguration `json:"items"`
}

// DataOutput specifies a data output source from an object.
type DataOutput struct {
	// Name is the unique name of a DataOutput in an ApplicationConfiguration.
	Name string `json:"name,omitempty"`

	// FieldPath refers to the value of an object's field.
	FieldPath string `json:"fieldPath,omitempty"`

	// Conditions specify the conditions that should be satisfied before emitting a data output.
	// Different conditions are AND-ed together.
	// If no conditions is specified, it is by default to check output value not empty.
	// +optional
	Conditions []ConditionRequirement `json:"conditions,omitempty"`
}

// DataInput specifies a data input sink to an object.
// If input is array, it will be appended to the target field paths.
type DataInput struct {
	// ValueFrom specifies the value source.
	ValueFrom DataInputValueFrom `json:"valueFrom,omitempty"`

	// ToFieldPaths specifies the field paths of an object to fill passed value.
	ToFieldPaths []string `json:"toFieldPaths,omitempty"`
}

// DataInputValueFrom specifies the value source for a data input.
type DataInputValueFrom struct {
	// DataOutputName matches a name of a DataOutput in the same AppConfig.
	DataOutputName string `json:"dataOutputName"`
}

// ConditionRequirement specifies the requirement to match a value.
type ConditionRequirement struct {
	Operator ConditionOperator `json:"op"`
	Value    string            `json:"value"`
	// +optional
	FieldPath string `json:"fieldPath,omitempty"`
}

// ConditionOperator specifies the operator to match a value.
type ConditionOperator string

const (
	// ConditionEqual indicates equal to given value
	ConditionEqual ConditionOperator = "eq"
	// ConditionNotEqual indicates not equal to given value
	ConditionNotEqual ConditionOperator = "notEq"
	// ConditionNotEmpty indicates given value not empty
	ConditionNotEmpty ConditionOperator = "notEmpty"
)
