package dependency

import (
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

// DAG is the dependency graph for an AppConfig.
// The key is the SourceName (aka DataOutputName)
type DAG map[string]*SinksPerSource

// SinksPerSource represents the sinks that belong to a source.
type SinksPerSource struct {
	Source *Source
	// The key is of format 'kind:namespace/name' which should be unique for any object in an AppConfig.
	Sinks map[string]*Sink
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
func NewDAG() DAG {
	return DAG{}
}

func newSinksPerSource() *SinksPerSource {
	return &SinksPerSource{
		Sinks: make(map[string]*Sink),
	}
}

// AddSource adds a data output source into the DAG.
func (d DAG) AddSource(sourceName string, ref *corev1.ObjectReference) {
	sps := d.getOrCreateSinksPerSource(sourceName)
	sps.Source = &Source{ObjectRef: ref}
}

// AddSink adds a data input sink into the DAG.
func (d DAG) AddSink(sourceName string, obj *unstructured.Unstructured, f []string, m []v1alpha2.DataMatcherRequirement) {
	sps := d.getOrCreateSinksPerSource(sourceName)

	// Assuming 'kind:namespace/name' is unique. E.g, 'trait:default/app1-ingress'.
	key := obj.GetKind() + ":" + path.Join(obj.GetNamespace(), obj.GetName())
	sps.Sinks[key] = &Sink{
		Object:       obj,
		ToFieldPaths: f,
		Matchers:     m,
	}
}

func (d DAG) getOrCreateSinksPerSource(sourceName string) *SinksPerSource {
	sps, ok := d[sourceName]
	if !ok {
		sps = newSinksPerSource()
		d[sourceName] = sps
	}
	return sps
}
