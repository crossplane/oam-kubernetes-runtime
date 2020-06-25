package dependency

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

// DAG is the dependency graph for an AppConfig.
type DAG struct {
	Sources map[string]*Source
	Sinks   map[string]map[string]*Sink
}

// Source represents the object information with DataOutput
type Source struct {
	// ObjectRef refers to the object this source come from.
	ObjectRef *corev1.ObjectReference
}

// Sink represents the object information with DataInput
type Sink struct {
	// Object contains the whole object data used by the DAGManager to create objects
	// once the source is ready.
	Object *unstructured.Unstructured

	// ToFieldPaths specifies the field paths the passed value to fill into.
	ToFieldPaths []string

	Matchers []v1alpha2.DataMatcherRequirement
}

// NewDAG creates a fresh DAG.
func NewDAG() *DAG {
	return &DAG{
		Sources: make(map[string]*Source),
		Sinks:   make(map[string]map[string]*Sink),
	}
}

// AddSource adds a data output source into the DAG.
func (d *DAG) AddSource(sourceName string, ref *corev1.ObjectReference) {
	d.Sources[sourceName] = &Source{ObjectRef: ref}
}

// AddSink adds a data input sink into the DAG.
func (d *DAG) AddSink(sourceName string, obj *unstructured.Unstructured, f []string, m []v1alpha2.DataMatcherRequirement) {
	sm, ok := d.Sinks[sourceName]
	if !ok {
		sm = make(map[string]*Sink)
		d.Sinks[sourceName] = sm
	}
	// TODO: This is not uniqie across different resources. Need to pick another method.
	key := obj.GetNamespace() + "/" + obj.GetName()
	sm[key] = &Sink{
		Object:       obj,
		ToFieldPaths: f,
		Matchers:     m,
	}
}
