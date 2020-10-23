package mock

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ meta.RESTMapper = &Mapper{}

// nolint
type KindFor func(resource schema.GroupVersionResource) (schema.GroupVersionKind, error)

// nolint
type KindsFor func(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error)

// nolint
type ResourceFor func(input schema.GroupVersionResource) (schema.GroupVersionResource, error)

// nolint
type ResourcesFor func(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error)

// nolint
type RESTMapping func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error)

// nolint
type RESTMappings func(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error)

// nolint
type ResourceSingularizer func(resource string) (singular string, err error)

// NewMockMapper for mock
func NewMockMapper() *Mapper {
	return &Mapper{
		MockRESTMapping: NewMockRESTMapping(""),
	}
}

// NewMockRESTMapping for mock
func NewMockRESTMapping(resource string) RESTMapping {
	return func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
		return &meta.RESTMapping{Resource: schema.GroupVersionResource{Resource: resource}}, nil
	}
}

// Mapper for mock
type Mapper struct {
	MockKindFor              KindFor
	MockKindsFor             KindsFor
	MockResourceFor          ResourceFor
	MockResourcesFor         ResourcesFor
	MockRESTMapping          RESTMapping
	MockRESTMappings         RESTMappings
	MockResourceSingularizer ResourceSingularizer
}

// KindFor for mock
func (m *Mapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return m.MockKindFor(resource)
}

// KindsFor for mock
func (m *Mapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return m.MockKindsFor(resource)
}

// ResourceFor for mock
func (m *Mapper) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return m.MockResourceFor(input)
}

// ResourcesFor for mock
func (m *Mapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return m.MockResourcesFor(input)
}

// RESTMapping for mock
func (m *Mapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	return m.MockRESTMapping(gk, versions...)
}

// RESTMappings for mock
func (m *Mapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	return m.MockRESTMappings(gk, versions...)
}

// ResourceSingularizer for mock
func (m *Mapper) ResourceSingularizer(resource string) (singular string, err error) {
	return m.MockResourceSingularizer(resource)
}
