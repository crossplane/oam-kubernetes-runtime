package mock

import (
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/discoverymapper"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ discoverymapper.DiscoveryMapper = &DiscoveryMapper{}

// nolint
type GetMapper func() (meta.RESTMapper, error)

// nolint
type Refresh func() (meta.RESTMapper, error)

// nolint
type RESTMapping func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error)

// NewMockDiscoveryMapper for mock
func NewMockDiscoveryMapper() *DiscoveryMapper {
	return &DiscoveryMapper{
		MockRESTMapping: NewMockRESTMapping(""),
	}
}

// NewMockRESTMapping for unit test only
func NewMockRESTMapping(resource string) RESTMapping {
	return func(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
		return &meta.RESTMapping{Resource: schema.GroupVersionResource{Resource: resource, Version: versions[0], Group: gk.Group}}, nil
	}
}

// DiscoveryMapper for unit test only, use GetMapper and refresh will panic
type DiscoveryMapper struct {
	MockGetMapper   GetMapper
	MockRefresh     Refresh
	MockRESTMapping RESTMapping
}

// GetMapper for mock
func (m *DiscoveryMapper) GetMapper() (meta.RESTMapper, error) {
	return m.MockGetMapper()
}

// Refresh for mock
func (m *DiscoveryMapper) Refresh() (meta.RESTMapper, error) {
	return m.MockRefresh()
}

// RESTMapping for mock
func (m *DiscoveryMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	return m.MockRESTMapping(gk, versions...)
}
