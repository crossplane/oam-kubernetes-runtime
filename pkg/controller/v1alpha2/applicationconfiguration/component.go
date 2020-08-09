package applicationconfiguration

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/rs/xid"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	util "github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

// ComponentHandler will watch component change and generate Revision automatically.
type ComponentHandler struct {
	Client client.Client
	Logger logging.Logger
}

// Create implements EventHandler
func (c *ComponentHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	if !c.createControllerRevision(evt.Meta, evt.Object) {
		// No revision created, return
		return
	}
	for _, req := range c.getRelatedAppConfig(evt.Meta) {
		q.Add(req)
	}
}

// Update implements EventHandler
func (c *ComponentHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	if !c.createControllerRevision(evt.MetaNew, evt.ObjectNew) {
		// No revision created, return
		return
	}
	//Note(wonderflow): MetaOld => MetaNew, requeue once is enough
	for _, req := range c.getRelatedAppConfig(evt.MetaNew) {
		q.Add(req)
	}
}

// Delete implements EventHandler
func (c *ComponentHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	// controllerRevision will be deleted by ownerReference mechanism
	// so we don't need to delete controllerRevision here.
	// but trigger an event to AppConfig controller, let it know.
	for _, req := range c.getRelatedAppConfig(evt.Meta) {
		q.Add(req)
	}
}

// Generic implements EventHandler
func (c *ComponentHandler) Generic(_ event.GenericEvent, _ workqueue.RateLimitingInterface) {
	// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
	// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
	// so we need to do nothing here.
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
	err := c.Client.List(context.Background(), &appConfigs)
	if err != nil {
		c.Logger.Info(fmt.Sprintf("error list all applicationConfigurations %v", err))
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
	if curComp.Status.LatestRevision == nil {
		return true, 0
	}

	// client in controller-runtime will use infoermer cache
	// use client will be more efficient
	oldRev := &appsv1.ControllerRevision{}
	if err := c.Client.Get(context.TODO(), client.ObjectKey{Namespace: mt.GetNamespace(), Name: curComp.Status.LatestRevision.Name}, oldRev); err != nil {
		c.Logger.Info(fmt.Sprintf("get old controllerRevision %s error %v, will create new revision", curComp.Status.LatestRevision.Name, err), "componentName", mt.GetName())
		return true, curComp.Status.LatestRevision.Revision
	}
	if oldRev.Name == "" {
		c.Logger.Info(fmt.Sprintf("Not found controllerRevision %s", curComp.Status.LatestRevision.Name), "componentName", mt.GetName())
		return true, curComp.Status.LatestRevision.Revision
	}
	oldComp, err := util.UnpackRevisionData(oldRev)
	if err != nil {
		c.Logger.Info(fmt.Sprintf("Unmarshal old controllerRevision %s error %v, will create new revision", curComp.Status.LatestRevision.Name, err), "componentName", mt.GetName())
		return true, oldRev.Revision
	}

	if reflect.DeepEqual(curComp.Spec, oldComp.Spec) {
		return false, oldRev.Revision
	}
	return true, oldRev.Revision
}

func newTrue() *bool {
	b := true
	return &b
}

func (c *ComponentHandler) createControllerRevision(mt metav1.Object, obj runtime.Object) bool {
	curComp := obj.(*v1alpha2.Component)
	diff, curRevision := c.IsRevisionDiff(mt, curComp)
	if !diff {
		// No difference, no need to create new revision.
		return false
	}
	nextRevision := curRevision + 1
	revisionName := ConstructRevisionName(mt.GetName())

	curComp.Status.LatestRevision = &v1alpha2.Revision{
		Name:     revisionName,
		Revision: nextRevision,
	}
	// set annotation to component
	revision := appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      revisionName,
			Namespace: curComp.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
					Kind:       v1alpha2.ComponentKind,
					Name:       curComp.Name,
					UID:        curComp.UID,
					Controller: newTrue(),
				},
			},
		},
		Revision: nextRevision,
		Data:     runtime.RawExtension{Object: curComp},
	}
	err := c.Client.Create(context.TODO(), &revision)
	if err != nil {
		c.Logger.Info(fmt.Sprintf("error create controllerRevision %v", err), "componentName", mt.GetName())
		return false
	}
	err = c.Client.Status().Update(context.Background(), curComp)
	if err != nil {
		c.Logger.Info(fmt.Sprintf("update component status latestRevision %s err %v", revisionName, err), "componentName", mt.GetName())
		return false
	}
	c.Logger.Info(fmt.Sprintf("ControllerRevision %s created", revisionName))
	return true
}

// ConstructRevisionName will generate revisionName from componentName
// hash suffix char set added to componentName is (0-9, a-v)
func ConstructRevisionName(componentName string) string {
	return strings.Join([]string{componentName, xid.NewWithTime(time.Now()).String()}, "-")
}

// ExtractComponentName will extract componentName from revisionName
func ExtractComponentName(revisionName string) string {
	splits := strings.Split(revisionName, "-")
	return strings.Join(splits[0:len(splits)-1], "-")
}
