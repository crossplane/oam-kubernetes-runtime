package dependency

import (
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

// DAG is the dependency graph for an AppConfig.
type DAG struct {
	AppConfig *v1alpha2.ApplicationConfiguration

	// The key is the SourceName (aka DataOutputName)
	SourceMap map[string]*SinksPerSource
}

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

	Matchers []v1alpha2.ConditionRequirement
}

// Sink represents the object information with DataInput
type Sink struct {
	// Object contains the whole object data used by the DAGManager to create objects
	// once the source is ready.
	Object *unstructured.Unstructured

	attaches []unstructured.Unstructured

	// ToFieldPaths specifies the field paths the passed value to fill into.
	ToFieldPaths []string
}

// NewDAG creates a fresh DAG.
func NewDAG(ac *v1alpha2.ApplicationConfiguration) *DAG {
	return &DAG{
		AppConfig: ac,
		SourceMap: make(map[string]*SinksPerSource),
	}
}

func newSinksPerSource() *SinksPerSource {
	return &SinksPerSource{
		Sinks: make(map[string]*Sink),
	}
}

// IsEmpty checks whether the DAG empty or not
func (d *DAG) IsEmpty() bool {
	return len(d.SourceMap) == 0
}

// AddSource adds a data output source into the DAG.
func (d *DAG) AddSource(sourceName string, ref *corev1.ObjectReference, m []v1alpha2.ConditionRequirement) {
	sps := d.getOrCreateSinksPerSource(sourceName)
	sps.Source = &Source{ObjectRef: ref, Matchers: m}
}

// AddSink adds a data input sink into the DAG.
func (d *DAG) AddSink(sourceName string, obj *unstructured.Unstructured, attaches []unstructured.Unstructured, f []string) {
	sps := d.getOrCreateSinksPerSource(sourceName)

	// Assuming 'kind:namespace/name' is unique. E.g, 'trait:default/app1-ingress'.
	key := obj.GetKind() + ":" + path.Join(obj.GetNamespace(), obj.GetName())
	sps.Sinks[key] = &Sink{
		Object:       obj,
		attaches:     attaches,
		ToFieldPaths: f,
	}
}

func (d *DAG) getOrCreateSinksPerSource(sourceName string) *SinksPerSource {
	sps, ok := d.SourceMap[sourceName]
	if !ok {
		sps = newSinksPerSource()
		d.SourceMap[sourceName] = sps
	}
	return sps
}
