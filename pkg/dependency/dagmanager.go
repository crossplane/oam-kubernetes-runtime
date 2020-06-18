package dependency

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DAGManager manages the dependency graphs (DAG) of all AppConfigs.
// Each AppConfig has its own DAG where its components and traits depends on others unidirectionally.
type DAGManager interface {
	// Start is blocking call that bootstraps the manager code.
	// It should be called in initialization stage.
	Start(context.Context)

	AddDAG(appKey string, dag *DAG)
}

// NewDAGManager constructs dagManager and returns it.
func NewDAGManager(l logging.Logger) DAGManager {
	return &dagManager{
		log:     l.WithValues("manager", "dag"),
		app2dag: make(map[string]*DAG),
	}
}

// DAG is the dependency graph for an AppConfig.
type DAG struct {
	sources map[string]*Source
	sinks   map[string]map[string]*Sink
}

// Source represents the object information with DataOutput
type Source struct {
	ObjectRef *corev1.ObjectReference
}

// Sink represents the object information with DataInput
type Sink struct {
	ObjectRef  *corev1.ObjectReference
	FieldPaths []string
	Raw        runtime.RawExtension
	OwnerUUID  types.UID
}

type dagManager struct {
	mu      sync.Mutex
	log     logging.Logger
	client  client.Client
	app2dag map[string]*DAG
}

func (dm *dagManager) Start(ctx context.Context) {
	for {
		select {
		case <-time.After(5 * time.Second):
			dm.scan(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (dm *dagManager) scan(ctx context.Context) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	for app, dag := range dm.app2dag {
		for sourceName, source := range dag.sources {
			// TODO: avoid repeating processing the same source by marking done in AppConfig status
			val, err := dm.checkSourceReady(ctx, source)
			if err != nil {
				dm.log.Info("checkSourceReady failed", "err", err)
				continue
			}
			if val == "" { // not ready
				continue
			}

			for sinkName, sink := range dag.sinks[sourceName] {
				dm.log.Debug("triggering sinks", "app", app, "source", sourceName, "sink", sinkName)
				dm.trigger(ctx, sink, val)
			}

			delete(dag.sources, sourceName)
		}

		if len(dag.sources) == 0 {
			dm.log.Debug("all dependencies satisfied", "app", app)
			delete(dm.app2dag, app)
		}
	}
}

func (dm *dagManager) checkSourceReady(ctx context.Context, s *Source) (string, error) {
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

	// TODO: Currently only string value supported. Need to support more types.
	val, err := paved.GetString(obj.FieldPath)
	if err != nil {
		return "", fmt.Errorf("failed to get field value (%s): %w", obj.FieldPath, err)
	}

	return val, nil
}

func (dm *dagManager) trigger(ctx context.Context, s *Sink, val string) error {
	// TODO: avoid repeating processing the same sink by marking done in AppConfig status
	obj := s.ObjectRef
	key := types.NamespacedName{
		Namespace: obj.Namespace,
		Name:      obj.Name,
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

	paved := fieldpath.Pave(u.Object)
	for _, fp := range s.FieldPaths {
		paved.SetString(fp, val)
	}
	u.Object = paved.UnstructuredContent()

	return errors.Wrap(dm.client.Create(ctx, u), "create sink object failed")
}

func (dm *dagManager) AddDAG(appKey string, dag *DAG) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	dm.app2dag[appKey] = dag
}
