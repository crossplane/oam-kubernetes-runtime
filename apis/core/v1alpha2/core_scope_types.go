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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
)

// HealthStatus represents health status strings.
type HealthStatus string

const (
	// StatusHealthy represents healthy status.
	StatusHealthy HealthStatus = "HEALTHY"
	// StatusUnhealthy represents unhealthy status.
	StatusUnhealthy = "UNHEALTHY"
	// StatusUnknown represents unknown status.
	StatusUnknown = "UNKNOWN"
)

var _ oam.Scope = &HealthScope{}

// A HealthScopeSpec defines the desired state of a HealthScope.
type HealthScopeSpec struct {
	// ProbeTimeout is the amount of time in seconds to wait when receiving a response before marked failure.
	ProbeTimeout *int32 `json:"probe-timeout,omitempty"`

	// ProbeInterval is the amount of time in seconds between probing tries.
	ProbeInterval *int32 `json:"probe-interval,omitempty"`

	// WorkloadReferences to the workloads that are in this scope.
	WorkloadReferences []runtimev1alpha1.TypedReference `json:"workloadRefs"`
}

// A HealthScopeStatus represents the observed state of a HealthScope.
type HealthScopeStatus struct {
	runtimev1alpha1.ConditionedStatus `json:",inline"`

	// ScopeHealthCondition represents health condition summary of the scope
	ScopeHealthCondition ScopeHealthCondition `json:"scopeHealthCondition"`

	// WorkloadHealthConditions represents health condition of workloads in the scope
	WorkloadHealthConditions []*WorkloadHealthCondition `json:"healthConditions,omitempty"`
}

// ScopeHealthCondition represents health condition summary of a scope.
type ScopeHealthCondition struct {
	HealthStatus       HealthStatus `json:"healthStatus"`
	Total              int64        `json:"total,omitempty"`
	HealthyWorkloads   int64        `json:"healthyWorkloads,omitempty"`
	UnhealthyWorkloads int64        `json:"unhealthyWorkloads,omitempty"`
	UnknownWorkloads   int64        `json:"unknownWorkloads,omitempty"`
}

// WorkloadHealthCondition represents informative health condition.
type WorkloadHealthCondition struct {
	// ComponentName represents the component name if target is a workload
	ComponentName  string                         `json:"componentName,omitempty"`
	TargetWorkload runtimev1alpha1.TypedReference `json:"targetWorkload,omitempty"`
	HealthStatus   HealthStatus                   `json:"healthStatus"`
	Diagnosis      string                         `json:"diagnosis,omitempty"`
	// WorkloadStatus represents status of workloads whose HealthStatus is UNKNOWN.
	WorkloadStatus string `json:"workloadStatus,omitempty"`
}

// +kubebuilder:object:root=true

// A HealthScope determines an aggregate health status based of the health of components.
// +kubebuilder:resource:categories={crossplane,oam}
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".status.health",name=HEALTH,type=string
type HealthScope struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HealthScopeSpec   `json:"spec,omitempty"`
	Status HealthScopeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HealthScopeList contains a list of HealthScope.
type HealthScopeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HealthScope `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster

// A CategoryScope to control label/annotation mark.
type CategoryScope struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec CategoryScopeSpec `json:"spec,omitempty"`
}

var _ oam.Scope = &CategoryScope{}

// +kubebuilder:object:root=true

// CategoryScopeList contains a list of CategoryScope.
type CategoryScopeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CategoryScope `json:"items"`
}

// A CategoryScopeSpec defines the desired state of a CategoryScope.
type CategoryScopeSpec struct {

	// Annotations is an unstructured key value map stored with a resource that may be
	// set by external tools to store and retrieve arbitrary metadata.
	Annotation []ScopeVar `json:"annotation,omitempty"`

	// Map of string keys and values that can be used to organize and categorize
	// (scope and select) objects. May match selectors of replication controllers
	// and services.
	Labels []ScopeVar `json:"labels,omitempty"`
}

// ScopeVar represents an annotation/labels variable present in a Scope.
type ScopeVar struct {
	// key of the annotation/labels variable. Must be a C_IDENTIFIER.
	Key string `json:"key"`

	Value string `json:"value,omitempty"`
	// Source for the annotation/labels variable's value. Cannot be used if value is not empty.
	// +optional
	ValueFrom *ScopeSource `json:"valueFrom,omitempty"`
}

// ScopeSource represents a source for the value of an ScopeVar.
type ScopeSource struct {
	// Selects a field of the pod: supports metadata.name, metadata.namespace, metadata.uid.
	// +optional
	FieldRef *corev1.ObjectFieldSelector `json:"fieldRef,omitempty"`
	// Selects a resource of the container: only resources limits and requests
	// (limits.cpu, limits.memory, limits.ephemeral-storage, requests.cpu, requests.memory and requests.ephemeral-storage) are currently supported.
	// +optional
	ResourceFieldRef *corev1.ResourceFieldSelector `json:"resourceFieldRef,omitempty"`
	// Selects a key of a ConfigMap.
	// +optional
	ConfigMapKeyRef *corev1.ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`
	// Selects a key of a secret in the pod's namespace
	// +optional
	SecretKeyRef *corev1.SecretKeySelector `json:"secretKeyRef,omitempty"`
}
