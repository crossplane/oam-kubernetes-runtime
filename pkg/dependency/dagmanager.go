package dependency

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
	AddDAG(appKey string, dag DAG)
}

// SetupGlobalDAGManager sets up the global dagManager.
func SetupGlobalDAGManager(l logging.Logger, c client.Client) {
	GlobalManager = &dagManagerImpl{
		client:  c,
		log:     l.WithValues("manager", "dag"),
		app2dag: make(map[string]DAG),
	}
}

type dagManagerImpl struct {
	mu     sync.Mutex
	log    logging.Logger
	client client.Client

	app2dag map[string]DAG
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
		for sourceName, sps := range dag {
			// TODO: avoid repeating processing the same source by marking done in AppConfig status
			val, err := dm.checkSourceReady(ctx, sps.Source)
			if err != nil {
				dm.log.Info("checkSourceReady failed", "errmsg", err)
				continue
			}

			for sinkName, sink := range sps.Sinks {
				if !matchValue(sink.Matchers, val) {
					continue // not ready
				}

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
				delete(dag, sourceName)
			}
		}

		// Only handles startup dependency scenario now.
		if len(dag) == 0 {
			dm.log.Debug("all dependencies satisfied", "app", app)
			// TODO: report status and send event.
			delete(dm.app2dag, app)
		}
	}
}

func matchValue(ms []v1alpha2.DataMatcherRequirement, val string) bool {
	// If no matcher is specified, it is by default to check value not empty.
	if len(ms) == 0 {
		if val == "" {
			return false
		}
		return true
	}

	for _, m := range ms {
		switch m.Operator {
		case v1alpha2.DataMatcherOperatorEqual:
			if m.Value != val {
				return false
			}
		case v1alpha2.DataMatcherOperatorNotEqual:
			if m.Value == val {
				return false
			}
		case v1alpha2.DataMatcherOperatorNotEmpty:
			if val == "" {
				return false
			}
		}
	}

	return true
}

// TODO: This logic had better belongs to the source itself.
func (dm *dagManagerImpl) checkSourceReady(ctx context.Context, s *Source) (string, error) {
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
		return "", fmt.Errorf("failed to get object (%s): %w", key.String(), err)
	}
	paved := fieldpath.Pave(u.Object)

	// TODO: Currently only string value supported. Support more types in the future.
	val, err := paved.GetString(obj.FieldPath)
	if err != nil {
		return "", fmt.Errorf("failed to get field value (%s) in object (%s): %w", obj.FieldPath, key.String(), err)
	}

	return val, nil
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

	paved := fieldpath.Pave(obj.Object)

	// fill values
	for _, fp := range s.ToFieldPaths {
		err := paved.SetString(fp, val)
		if err != nil {
			return fmt.Errorf("paved.SetString() failed: %w", err)
		}
	}

	return errors.Wrap(dm.client.Create(ctx, &unstructured.Unstructured{Object: paved.UnstructuredContent()}), "create sink object failed")
}

func (dm *dagManagerImpl) AddDAG(appKey string, dag DAG) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	dm.app2dag[appKey] = dag
}
