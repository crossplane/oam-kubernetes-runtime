/*
Copyright 2020 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package workload

import (
	"context"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
)

var (
	serviceKind       = reflect.TypeOf(corev1.Service{}).Name()
	serviceAPIVersion = corev1.SchemeGroupVersion.String()
)

// LabelKey is the label applied to translated workload objects.
const LabelKey = "workload.oam.crossplane.io"

// ServiceInjector adds a Service object for the first Port on the first
// Container for the first Deployment observed in a workload translation.
func ServiceInjector(ctx context.Context, w oam.Workload, objs []oam.Object) ([]oam.Object, error) {
	if objs == nil {
		return nil, nil
	}

	for _, o := range objs {
		d, ok := o.(*appsv1.Deployment)
		if !ok {
			continue
		}

		// We don't add a Service if there are no containers for the Deployment.
		// This should never happen in practice.
		if len(d.Spec.Template.Spec.Containers) < 1 {
			continue
		}

		s := &corev1.Service{
			TypeMeta: metav1.TypeMeta{
				Kind:       serviceKind,
				APIVersion: serviceAPIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      d.GetName(),
				Namespace: d.GetNamespace(),
				Labels: map[string]string{
					LabelKey: string(w.GetUID()),
				},
			},
			Spec: corev1.ServiceSpec{
				Selector: d.Spec.Selector.MatchLabels,
				Ports:    []corev1.ServicePort{},
				Type:     corev1.ServiceTypeLoadBalancer,
			},
		}

		// We only add a single Service for the Deployment, even if multiple
		// ports or no ports are defined on the first container. This is to
		// exclude the need for implementing garbage collection in the
		// short-term in the case that ports are modified after creation.
		if len(d.Spec.Template.Spec.Containers[0].Ports) > 0 {
			s.Spec.Ports = []corev1.ServicePort{
				{
					Name:       d.GetName(),
					Port:       d.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort,
					TargetPort: intstr.FromInt(int(d.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort)),
				},
			}
		}
		objs = append(objs, s)
		break
	}
	return objs, nil
}
