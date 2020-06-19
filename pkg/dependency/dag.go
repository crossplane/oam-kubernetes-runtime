package dependency

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// DAG is the dependency graph for an AppConfig.
type DAG struct {
	sources map[string]*Source
	sinks   map[string]map[string]*Sink
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
}

// NewDAG creates a fresh DAG.
func NewDAG() *DAG {
	return &DAG{
		sources: make(map[string]*Source),
		sinks:   make(map[string]map[string]*Sink),
	}
}

// AddSource adds a data output source into the DAG.
func (d *DAG) AddSource(sourceName string, ref *corev1.ObjectReference) {
	d.sources[sourceName] = &Source{ObjectRef: ref}
}

// AddSink adds a data input sink into the DAG.
func (d *DAG) AddSink(sourceName string, obj *unstructured.Unstructured, toFieldPaths []string) {
	m, ok := d.sinks[sourceName]
	if !ok {
		m = make(map[string]*Sink)
		d.sinks[sourceName] = m
	}
	key := obj.GetNamespace() + "/" + obj.GetName()
	m[key] = &Sink{
		Object:       obj,
		ToFieldPaths: toFieldPaths,
	}
}
