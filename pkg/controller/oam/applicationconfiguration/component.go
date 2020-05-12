package applicationconfiguration

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/rs/xid"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientappv1 "k8s.io/client-go/kubernetes/typed/apps/v1"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ComponentHandler will watch component change and generate Revision automatically.
type ComponentHandler struct {
	client     client.Client
	appsClient clientappv1.AppsV1Interface
	l          logging.Logger
}

// Create implements EventHandler
func (c *ComponentHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	c.createControllerRevision(evt.Meta, evt.Object)
	for _, req := range c.getRelatedAppConfig(evt.Meta) {
		q.Add(req)
	}
}

// Update implements EventHandler
func (c *ComponentHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	c.createControllerRevision(evt.MetaNew, evt.ObjectNew)
	//Note(wonderflow): MetaOld => MetaNew, requeue once is enough
	for _, req := range c.getRelatedAppConfig(evt.MetaOld) {
		q.Add(req)
	}
}

// Delete implements EventHandler
func (c *ComponentHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	for _, req := range c.getRelatedAppConfig(evt.Meta) {
		q.Add(req)
	}
}

// Generic implements EventHandler
func (c *ComponentHandler) Generic(_ event.GenericEvent, _ workqueue.RateLimitingInterface) {
}

func isMatch(appConfigs *v1alpha2.ApplicationConfigurationList, compName string) (bool, types.NamespacedName) {
	for _, app := range appConfigs.Items {
		for _, comp := range app.Spec.Components {
			if comp.ComponentName == compName {
				return true, types.NamespacedName{Namespace: app.Namespace, Name: app.Name}
			}
		}
	}
	return false, types.NamespacedName{}
}

func (c *ComponentHandler) getRelatedAppConfig(object metav1.Object) []reconcile.Request {
	var appConfigs v1alpha2.ApplicationConfigurationList
	err := c.client.List(context.Background(), &appConfigs)
	if err != nil {
		c.l.Info(fmt.Sprintf("error list all applicationConfigurations %v", err))
		return nil
	}
	var reqs []reconcile.Request
	if match, namespaceName := isMatch(&appConfigs, object.GetName()); match {
		reqs = append(reqs, reconcile.Request{NamespacedName: namespaceName})
	}
	return reqs
}

//IsRevisionDiff check whether there's any different between two component revision
func (c *ComponentHandler) IsRevisionDiff(mt metav1.Object, curComp *v1alpha2.Component) (bool, int64) {
	if curComp.Status.LatestRevision == "" {
		return true, 1
	}
	oldRev, err := c.appsClient.ControllerRevisions(mt.GetNamespace()).Get(context.Background(), curComp.Status.LatestRevision, metav1.GetOptions{})
	if err != nil {
		c.l.Info(fmt.Sprintf("get old controllerRevision %s error %v, will create new revision", curComp.Status.LatestRevision, err), "componentName", mt.GetName())
		// Note(wonderflow) Use generation as revision number when fail to get old revision
		return true, mt.GetGeneration()
	}
	var oldComp v1alpha2.Component
	err = json.Unmarshal(oldRev.Data.Raw, &oldComp)
	if err != nil {
		c.l.Info(fmt.Sprintf("Unmarshal old controllerRevision %s error %v, will create new revision", curComp.Status.LatestRevision, err), "componentName", mt.GetName())
		return true, oldRev.Revision + 1
	}
	if reflect.DeepEqual(curComp.Spec, oldComp.Spec) {
		return false, -1
	}
	return true, oldRev.Revision + 1
}

func (c *ComponentHandler) createControllerRevision(mt metav1.Object, obj runtime.Object) {
	curComp := obj.(*v1alpha2.Component)
	diff, newRevision := c.IsRevisionDiff(mt, curComp)
	if !diff {
		// No difference, no need to create new revision.
		return
	}
	// hash suffix char set is (0-9, a-v)
	revisionName := mt.GetName() + "-" + xid.NewWithTime(time.Now()).String()
	// set annotation to component
	revision := appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name: revisionName,
		},
		Revision: newRevision,
		Data:     runtime.RawExtension{Object: obj},
	}
	_, err := c.appsClient.ControllerRevisions(mt.GetNamespace()).Create(context.Background(), &revision, metav1.CreateOptions{})
	if err != nil {
		c.l.Info(fmt.Sprintf("error create controllerRevision %v", err), "componentName", mt.GetName())
		return
	}
	curComp.Status.LatestRevision = revisionName
	err = c.client.Status().Update(context.Background(), curComp)
	if err != nil {
		c.l.Info(fmt.Sprintf("update component status latestRevision %s err %v", revisionName, err), "componentName", mt.GetName())
		return
	}
}
