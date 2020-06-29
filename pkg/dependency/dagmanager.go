package dependency

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cpv1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/logging"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

// GlobalManager is the singleton instance of DAGManager.
var GlobalManager DAGManager

// DAGManager manages the dependency graphs (DAG) of all AppConfigs.
// Each AppConfig has its own DAG where its components and traits depends on others unidirectionally.
type DAGManager interface {
	// Start is blocking call that bootstraps the manager code.
	// It should be called in initialization stage.
	Start(context.Context)

	// AddDAG adds a dag of an AppConfig into DAGManager.
	AddDAG(appKey string, dag *DAG)
}

// SetupGlobalDAGManager sets up the global dagManager.
func SetupGlobalDAGManager(l logging.Logger, c client.Client) {
	GlobalManager = &dagManagerImpl{
		client:  c,
		log:     l.WithValues("manager", "dag"),
		app2dag: make(map[string]*DAG),
	}
}

type dagManagerImpl struct {
	mu     sync.Mutex
	log    logging.Logger
	client client.Client

	app2dag map[string]*DAG
}

func (dm *dagManagerImpl) Start(ctx context.Context) {
	dm.log.Info("starting DAG manager...")
	for {
		select {
		case <-time.After(5 * time.Second):
			dm.scan(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (dm *dagManagerImpl) scan(ctx context.Context) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	for app, dag := range dm.app2dag {
		for sourceName, sps := range dag.SourceMap {
			// TODO: avoid repeating processing the same source by marking done in AppConfig status
			val, ready, err := dm.checkSourceReady(ctx, sps.Source)
			if err != nil {
				dm.log.Info("checkSourceReady failed", "errmsg", err)
				continue
			}
			if !ready {
				continue
			}

			for sinkName, sink := range sps.Sinks {

				dm.log.Debug("triggering sinks", "app", app, "source", sourceName, "sink", sinkName)

				err := dm.trigger(ctx, sink, val)
				if err != nil {
					dm.log.Info("triggering sink failed", "errmsg", err)
					continue
				}

				// TODO: report status and send event.
				delete(sps.Sinks, sinkName)
			}

			if len(sps.Sinks) == 0 {
				// TODO: report status and send event.
				delete(dag.SourceMap, sourceName)
			}
		}

		// Only handles startup dependency scenario now.
		if len(dag.SourceMap) == 0 {
			err := dm.markDependencyDone(ctx, dag.AppConfig)
			if err != nil {
				dm.log.Info("markDependencyDone failed", "err", err)
				continue
			}
			dm.log.Debug("all dependencies satisfied", "app", app)
			delete(dm.app2dag, app)
		}
	}
}

func (dm *dagManagerImpl) markDependencyDone(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) error {
	key := types.NamespacedName{
		Namespace: ac.Namespace,
		Name:      ac.Name,
	}
	err := dm.client.Get(ctx, key, ac)
	if err != nil {
		return err
	}
	acPatch := client.MergeFrom(ac.DeepCopyObject())
	depDone := cpv1alpha1.Condition{
		Type:               cpv1alpha1.TypeReady,
		Status:             corev1.ConditionTrue,
		Reason:             v1alpha2.ReasonDependencyDone,
		LastTransitionTime: metav1.Now(),
	}
	ac.SetConditions(depDone)
	return errors.Wrap(
		dm.client.Status().Patch(ctx, ac, acPatch, client.FieldOwner(ac.GetUID())),
		"cannot apply status")
}

func matchValue(ms []v1alpha2.DataMatcherRequirement, val string, paved *fieldpath.Paved) bool {
	// If no matcher is specified, it is by default to check value not empty.
	if len(ms) == 0 {
		return val != ""
	}

	for _, m := range ms {
		var checkVal string
		var err error
		if m.FieldPath != "" {
			checkVal, err = paved.GetString(m.FieldPath)
			if err != nil {
				// can not get value from field path, it's not ready
				return false
			}
		} else {
			checkVal = val
		}
		switch m.Operator {
		case v1alpha2.DataMatcherOperatorEqual:
			if m.Value != checkVal {
				return false
			}
		case v1alpha2.DataMatcherOperatorNotEqual:
			if m.Value == checkVal {
				return false
			}
		case v1alpha2.DataMatcherOperatorNotEmpty:
			if checkVal == "" {
				return false
			}
		}
	}
	return true
}

// TODO: This logic had better belongs to the source itself.
func (dm *dagManagerImpl) checkSourceReady(ctx context.Context, s *Source) (string, bool, error) {
	// TODO: avoid repeating check by marking ready in AppConfig status
	obj := s.ObjectRef
	key := types.NamespacedName{
		Namespace: obj.Namespace,
		Name:      obj.Name,
	}
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(obj.GroupVersionKind())
	err := dm.client.Get(ctx, key, u)
	if err != nil {
		return "", false, fmt.Errorf("failed to get object (%s): %w", key.String(), err)
	}
	paved := fieldpath.Pave(u.UnstructuredContent())

	// TODO: Currently only string value supported. Support more types in the future.
	val, err := paved.GetString(obj.FieldPath)
	if err != nil {
		return "", false, fmt.Errorf("failed to get field value (%s) in object (%s): %w", obj.FieldPath, key.String(), err)
	}

	if !matchValue(s.Matchers, val, paved) {
		return val, false, nil // not ready
	}

	return val, true, nil
}

func (dm *dagManagerImpl) trigger(ctx context.Context, s *Sink, val string) error {
	// TODO: avoid repeating processing the same sink by marking done in AppConfig status
	obj := s.Object
	key := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(obj.GroupVersionKind())
	err := dm.client.Get(ctx, key, u)
	if err == nil {
		// resource has been created. No need to do anything.
		return nil
	} else if !kerrors.IsNotFound(err) {
		return err
	}

	paved := fieldpath.Pave(obj.UnstructuredContent())

	// fill values
	for _, fp := range s.ToFieldPaths {
		err := paved.SetString(fp, val)
		if err != nil {
			return fmt.Errorf("paved.SetString() failed: %w", err)
		}
	}

	return errors.Wrap(dm.client.Create(ctx, &unstructured.Unstructured{Object: paved.UnstructuredContent()}), "create sink object failed")
}

func (dm *dagManagerImpl) AddDAG(appKey string, dag *DAG) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	dm.app2dag[appKey] = dag
}
