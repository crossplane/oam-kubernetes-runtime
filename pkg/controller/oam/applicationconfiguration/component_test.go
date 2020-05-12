package applicationconfiguration

import (
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/logging"

	"k8s.io/client-go/kubernetes/fake"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllertest"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestComponentHandler(t *testing.T) {
	q := controllertest.Queue{Interface: workqueue.New()}

	var instance = ComponentHandler{
		client:     test.NewMockClient(),
		appsClient: fake.NewSimpleClientset().AppsV1(),
		l:          logging.NewLogrLogger(ctrl.Log.WithName("test")),
	}
	comp := &v1alpha2.Component{
		ObjectMeta: metav1.ObjectMeta{Namespace: "biz", Name: "baz"},
	}

	// Test Create Event
	evt := event.CreateEvent{
		Object: comp,
		Meta:   comp.GetObjectMeta(),
	}
	instance.Create(evt, q)

}
