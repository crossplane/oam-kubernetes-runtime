package dependency

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

// DAG is the dependency graph for an AppConfig.
type DAG struct {
	Sources map[string]*Source
}

// Source represents the object information with DataOutput
type Source struct {
	// ObjectRef refers to the object this source come from.
	ObjectRef *corev1.ObjectReference

	Matchers []v1alpha2.ConditionRequirement
}

// NewDAG creates a fresh DAG.
func NewDAG(ac *v1alpha2.ApplicationConfiguration) *DAG {
	return &DAG{
		Sources: make(map[string]*Source),
	}
}

// AddSource adds a data output source into the DAG.
func (d *DAG) AddSource(sourceName string, ref *corev1.ObjectReference, m []v1alpha2.ConditionRequirement) {
	d.Sources[sourceName] = &Source{
		ObjectRef: ref,
		Matchers:  m,
	}
}
