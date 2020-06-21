package applicationconfiguration

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	json "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllertest"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestComponentHandler(t *testing.T) {
	q := controllertest.Queue{Interface: workqueue.New()}
	fakeAppClient := fake.NewSimpleClientset().AppsV1()
	var curComp = &v1alpha2.Component{}
	var instance = ComponentHandler{
		client: &test.MockClient{
			MockList: test.NewMockListFn(nil, func(obj runtime.Object) error {
				lists := v1alpha2.ApplicationConfigurationList{
					Items: []v1alpha2.ApplicationConfiguration{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "app1",
							},
							Spec: v1alpha2.ApplicationConfigurationSpec{
								Components: []v1alpha2.ApplicationConfigurationComponent{{
									ComponentName: "comp1",
								}},
							},
						},
					},
				}
				lBytes, _ := json.Marshal(lists)
				json.Unmarshal(lBytes, obj)
				return nil
			}),
			MockStatusUpdate: test.NewMockStatusUpdateFn(nil, func(obj runtime.Object) error {
				cur, ok := obj.(*v1alpha2.Component)
				if ok {
					cur.DeepCopyInto(curComp)
				}
				return nil
			}),
		},
		appsClient: fakeAppClient,
		l:          logging.NewLogrLogger(ctrl.Log.WithName("test")),
	}
	comp := &v1alpha2.Component{
		ObjectMeta: metav1.ObjectMeta{Namespace: "biz", Name: "comp1"},
		Spec:       v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &v1.Deployment{Spec: v1.DeploymentSpec{Template: v12.PodTemplateSpec{Spec: v12.PodSpec{Containers: []v12.Container{{Image: "nginx:v1"}}}}}}}},
	}

	// ============ Test Create Event Start ===================
	evt := event.CreateEvent{
		Object: comp,
		Meta:   comp.GetObjectMeta(),
	}
	instance.Create(evt, q)
	if q.Len() != 1 {
		t.Fatal("no event created, but suppose have one")
	}
	item, _ := q.Get()
	req := item.(reconcile.Request)
	// AppConfig event triggered, and compare revision created
	assert.Equal(t, req.Name, "app1")
	revisions, err := fakeAppClient.ControllerRevisions("biz").List(context.Background(), metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(revisions.Items))
	assert.Equal(t, true, strings.HasPrefix(revisions.Items[0].Name, "comp1-"))
	gotComp := revisions.Items[0].Data.Object.(*v1alpha2.Component)
	assert.Equal(t, comp.Spec, gotComp.Spec)
	q.Done(item)
	// ============ Test Create Event End ===================

	// ============ Test Update Event Start===================
	comp2 := &v1alpha2.Component{
		ObjectMeta: metav1.ObjectMeta{Namespace: "biz", Name: "comp1"},
		// change image
		Spec: v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &v1.Deployment{Spec: v1.DeploymentSpec{Template: v12.PodTemplateSpec{Spec: v12.PodSpec{Containers: []v12.Container{{Image: "nginx:v2"}}}}}}}},
	}
	curComp.Status.DeepCopyInto(&comp2.Status)
	updateEvt := event.UpdateEvent{
		ObjectOld: comp,
		MetaOld:   comp.GetObjectMeta(),
		ObjectNew: comp2,
		MetaNew:   comp2.GetObjectMeta(),
	}
	instance.Update(updateEvt, q)
	if q.Len() != 1 {
		t.Fatal("no event created, but suppose have one")
	}
	item, _ = q.Get()
	req = item.(reconcile.Request)
	// AppConfig event triggered, and compare revision created
	assert.Equal(t, req.Name, "app1")
	revisions, err = fakeAppClient.ControllerRevisions("biz").List(context.Background(), metav1.ListOptions{})
	assert.NoError(t, err)
	// Component changed, we have two revision now.
	assert.Equal(t, 2, len(revisions.Items))
	for _, v := range revisions.Items {
		assert.Equal(t, true, strings.HasPrefix(v.Name, "comp1-"))
		if v.Revision == 2 {
			gotComp := v.Data.Object.(*v1alpha2.Component)
			assert.Equal(t, comp2.Spec, gotComp.Spec)
		}
	}
	q.Done(item)
	// test no changes with component spec
	comp3 := &v1alpha2.Component{
		ObjectMeta: metav1.ObjectMeta{Namespace: "biz", Name: "comp1", Labels: map[string]string{"bar": "foo"}},
		Spec:       v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &v1.Deployment{Spec: v1.DeploymentSpec{Template: v12.PodTemplateSpec{Spec: v12.PodSpec{Containers: []v12.Container{{Image: "nginx:v2"}}}}}}}},
	}
	curComp.Status.DeepCopyInto(&comp3.Status)
	updateEvt = event.UpdateEvent{
		ObjectOld: comp2,
		MetaOld:   comp2.GetObjectMeta(),
		ObjectNew: comp3,
		MetaNew:   comp3.GetObjectMeta(),
	}
	instance.Update(updateEvt, q)
	if q.Len() != 0 {
		t.Fatal("should not trigger event with nothing changed no change")
	}
	// ============ Test Update Event End ===================
}

func TestConstructExtract(t *testing.T) {
	tests := []string{"tam1", "test-comp", "xx", "tt-x-x-c"}
	for _, componentName := range tests {
		for i := 0; i < 30; i++ {
			t.Run(fmt.Sprintf("tests %d for component[%s]", i, componentName), func(t *testing.T) {
				revisionName := ConstructRevisionName(componentName)
				got := ExtractComponentName(revisionName)
				if got != componentName {
					t.Errorf("want to get %s from %s but got %s", componentName, revisionName, got)
				}
			})
		}
	}
}

func TestIsMatch(t *testing.T) {
	var appConfigs v1alpha2.ApplicationConfigurationList
	appConfigs.Items = []v1alpha2.ApplicationConfiguration{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "foo-app", Namespace: "foo-namespace"},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{{ComponentName: "foo"}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "bar-app", Namespace: "bar-namespace"},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{{ComponentName: "bar"}},
			},
		},
	}
	got, namespaceNamed := isMatch(&appConfigs, "foo")
	assert.Equal(t, true, got)
	assert.Equal(t, types.NamespacedName{Name: "foo-app", Namespace: "foo-namespace"}, namespaceNamed)
	got, _ = isMatch(&appConfigs, "foo1")
	assert.Equal(t, false, got)
	got, namespaceNamed = isMatch(&appConfigs, "bar")
	assert.Equal(t, true, got)
	assert.Equal(t, types.NamespacedName{Name: "bar-app", Namespace: "bar-namespace"}, namespaceNamed)
	appConfigs.Items = nil
	got, _ = isMatch(&appConfigs, "foo")
	assert.Equal(t, false, got)
}

func TestUnpackRevisionData(t *testing.T) {
	comp1 := v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp1"}}
	comp1Raw, _ := json.Marshal(comp1)
	tests := map[string]struct {
		rev     *appsv1.ControllerRevision
		expComp *v1alpha2.Component
		expErr  error
		reason  string
	}{
		"controllerRevision with Component Obj": {
			rev:     &appsv1.ControllerRevision{Data: runtime.RawExtension{Object: &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp1"}}}},
			expComp: &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp1"}},
			reason:  "controllerRevision should align with component object",
		},
		"controllerRevision with Unknown Obj": {
			rev:    &appsv1.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev1"}, Data: runtime.RawExtension{Object: &runtime.Unknown{Raw: comp1Raw}}},
			reason: "controllerRevision must be decode into component object",
			expErr: fmt.Errorf("invalid type of revision rev1, type should not be *runtime.Unknown"),
		},
		"unmarshal with component data": {
			rev:     &appsv1.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev1"}, Data: runtime.RawExtension{Raw: comp1Raw}},
			reason:  "controllerRevision should unmarshal data and align with component object",
			expComp: &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp1"}},
		},
	}
	for name, ti := range tests {
		t.Run(name, func(t *testing.T) {
			comp, err := UnpackRevisionData(ti.rev)
			if ti.expErr != nil {
				assert.Equal(t, ti.expErr, err, ti.reason)
			} else {
				assert.NoError(t, err, ti.reason)
				assert.Equal(t, ti.expComp, comp, ti.reason)
			}
		})
	}
}
