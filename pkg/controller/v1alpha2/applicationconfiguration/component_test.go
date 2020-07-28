package applicationconfiguration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sjson "k8s.io/apimachinery/pkg/util/json"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllertest"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = core.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func TestComponentHandler(t *testing.T) {
	q := controllertest.Queue{Interface: workqueue.New()}
	fakeClient := fake.NewFakeClientWithScheme(scheme)
	var instance = ComponentHandler{
		Client:               fakeClient,
		Logger:               logging.NewLogrLogger(ctrl.Log.WithName("test")),
		DefaultRevisionLimit: 2,
	}
	labels := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			ControllerRevisionComponentLabel: "comp1",
		},
	}
	selector, err := metav1.LabelSelectorAsSelector(labels)
	assert.NoError(t, err)

	app := &v1alpha2.ApplicationConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app1",
			Namespace: "biz",
		},
		Spec: v1alpha2.ApplicationConfigurationSpec{
			Components: []v1alpha2.ApplicationConfigurationComponent{{
				ComponentName: "comp1",
			}},
		},
	}
	currentComponent := &v1alpha2.Component{
		ObjectMeta: metav1.ObjectMeta{Namespace: "biz", Name: "comp1"},
		Spec:       v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &v1.Deployment{Spec: v1.DeploymentSpec{Template: v12.PodTemplateSpec{Spec: v12.PodSpec{Containers: []v12.Container{{Image: "nginx:v1"}}}}}}}},
	}
	mixComponent := currentComponent.DeepCopy()
	mixComponent.Name = "comp2"

	cases := []struct {
		name string
		// resource to init
		init []runtime.Object
		// resource to create
		create *v1alpha2.Component
		// resource to update
		update   func(create *v1alpha2.Component) *v1alpha2.Component
		expected func(*v1alpha2.Component, *v1alpha2.Component)
		// expected trigger reconcile
		expectedChange bool
	}{
		{
			name: "CreateComponent",
			init: []runtime.Object{
				app,
				currentComponent,
			},
			create:         currentComponent,
			expectedChange: true,
			expected: func(current, new *v1alpha2.Component) {
				tt := &v1alpha2.Component{}
				fakeClient.Get(context.TODO(), client.ObjectKey{Namespace: "biz", Name: "comp1"}, tt)
				item, _ := q.Get()
				req := item.(reconcile.Request)
				// AppConfig event triggered, and compare revision created
				assert.Equal(t, req.Name, "app1")
				revisions := &appsv1.ControllerRevisionList{}
				err = fakeClient.List(context.TODO(), revisions, &client.ListOptions{LabelSelector: selector})
				assert.NoError(t, err)
				assert.Equal(t, 1, len(revisions.Items))
				assert.Equal(t, new.Name, revisions.Items[0].Labels[ControllerRevisionComponentLabel])
				assert.Equal(t, true, strings.HasPrefix(revisions.Items[0].Name, "comp1-"))
				// check component's spec saved in corresponding controllerRevision
				gotComp, err := UnpackRevisionData(&revisions.Items[0])
				assert.NoError(t, err)
				deployment := &v1.Deployment{}
				k8sjson.Unmarshal(gotComp.Spec.Workload.Raw, deployment)
				assert.Equal(t, new.Spec.Workload.Object, deployment)
				// check component's status saved in corresponding controllerRevision
				assert.Equal(t, gotComp.Status.LatestRevision.Name, revisions.Items[0].Name)
				assert.Equal(t, gotComp.Status.LatestRevision.Revision, revisions.Items[0].Revision)
				q.Done(item)
			},
		}, {
			name: "UpdateComponent",
			update: func(comp *v1alpha2.Component) *v1alpha2.Component {
				newComp := comp.DeepCopy()
				newComp.Spec.Workload = runtime.RawExtension{Object: &v1.Deployment{Spec: v1.DeploymentSpec{Template: v12.PodTemplateSpec{Spec: v12.PodSpec{Containers: []v12.Container{{Image: "nginx:v2"}}}}}}}

				return newComp
			},
			expectedChange: true,
			expected: func(current, new *v1alpha2.Component) {
				item, _ := q.Get()
				req := item.(reconcile.Request)
				// AppConfig event triggered, and compare revision created
				assert.Equal(t, req.Name, "app1")
				revisions := &appsv1.ControllerRevisionList{}
				err = fakeClient.List(context.TODO(), revisions, &client.ListOptions{LabelSelector: selector})
				assert.NoError(t, err)
				// Component changed, we have two revision now.
				assert.Equal(t, 2, len(revisions.Items))
				for _, v := range revisions.Items {
					assert.Equal(t, true, strings.HasPrefix(v.Name, "comp1-"))
					if v.Revision == 2 {
						gotComp, err := UnpackRevisionData(&v)
						assert.NoError(t, err)
						// check component's spec saved in corresponding controllerRevision
						deployment := &v1.Deployment{}
						k8sjson.Unmarshal(gotComp.Spec.Workload.Raw, deployment)
						assert.Equal(t, new.Spec.Workload.Object, deployment)
						// check component's status saved in corresponding controllerRevision
						assert.Equal(t, gotComp.Status.LatestRevision.Name, v.Name)
						assert.Equal(t, gotComp.Status.LatestRevision.Revision, v.Revision)
					}
				}
				q.Done(item)
			},
		}, {
			name: "ExpectedNotChange",
			update: func(comp *v1alpha2.Component) *v1alpha2.Component {
				newComp := comp.DeepCopy()
				newComp.Labels = map[string]string{"bar": "foo"}

				return newComp
			},
			expectedChange: false,
		}, {
			name: "RevisionLimit",
			update: func(comp *v1alpha2.Component) *v1alpha2.Component {
				newComp := comp.DeepCopy()
				newComp.Spec.Workload = runtime.RawExtension{Object: &v1.Deployment{Spec: v1.DeploymentSpec{Template: v12.PodTemplateSpec{Spec: v12.PodSpec{Containers: []v12.Container{{Image: "nginx:v3"}}}}}}}

				return newComp
			},
			expectedChange: true,
			expected: func(current, new *v1alpha2.Component) {
				revisions := &appsv1.ControllerRevisionList{}
				err = fakeClient.List(context.TODO(), revisions, &client.ListOptions{LabelSelector: selector})
				assert.NoError(t, err)
				assert.Equal(t, 2, len(revisions.Items))
			},
		}, {
			init: []runtime.Object{
				mixComponent,
			},
			name:           "RevisionMixed",
			create:         mixComponent,
			expectedChange: true,
			expected: func(current, new *v1alpha2.Component) {
				newLabels := &metav1.LabelSelector{
					MatchLabels: map[string]string{
						ControllerRevisionComponentLabel: "comp2",
					},
				}
				newSelector, err := metav1.LabelSelectorAsSelector(newLabels)
				assert.NoError(t, err)
				revisions := &appsv1.ControllerRevisionList{}
				err = fakeClient.List(context.TODO(), revisions, &client.ListOptions{LabelSelector: selector})
				assert.NoError(t, err)
				newRevisions := &appsv1.ControllerRevisionList{}
				err = fakeClient.List(context.TODO(), newRevisions, &client.ListOptions{LabelSelector: newSelector})
				assert.NoError(t, err)
				assert.Equal(t, 2, len(revisions.Items))
				assert.Equal(t, 1, len(newRevisions.Items))
			},
		},
	}

	// init new component
	newComp := currentComponent
	for _, testcase := range cases {
		for _, initResource := range testcase.init {
			err := fakeClient.Create(context.TODO(), initResource)
			assert.NoError(t, err)
		}

		if testcase.create != nil {
			evt := event.CreateEvent{
				Object: testcase.create,
				Meta:   testcase.create.GetObjectMeta(),
			}
			instance.Create(evt, q)
		}

		if testcase.update != nil {
			newComp = testcase.update(currentComponent)
			updateEvt := event.UpdateEvent{
				ObjectOld: currentComponent,
				MetaOld:   currentComponent.GetObjectMeta(),
				ObjectNew: newComp,
				MetaNew:   newComp.GetObjectMeta(),
			}
			instance.Update(updateEvt, q)
		}

		if testcase.expectedChange {
			assert.Equal(t, 1, q.Len(), "%s: no event created, but suppose have one", testcase.name)
		} else {
			assert.Equal(t, 0, q.Len(), "%s: should not trigger event with nothing changed no change", testcase.name)
		}

		if testcase.expected != nil {
			testcase.expected(currentComponent, newComp)
		}

		currentComponent = newComp
	}
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
