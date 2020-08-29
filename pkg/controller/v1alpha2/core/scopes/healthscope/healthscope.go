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

package healthscope

import (
	"context"
	"fmt"
	"reflect"
	"time"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/pkg/errors"
)

const (
	infoFmtUnknownWorkload = "APIVersion %v Kind %v workload is unknown for HealthScope "
	infoFmtReady           = "Ready: %d/%d "
	errHealthCheck         = "error occurs in health check "

	defaultTimeout = 10 * time.Second
)

type HealthStatus = v1alpha2.HealthStatus

const (
	StatusHealthy   = v1alpha2.StatusHealthy
	StatusUnhealthy = v1alpha2.StatusUnhealthy
	StatusUnknown   = v1alpha2.StatusUnknown
)

var (
	kindContainerizedWorkload = corev1alpha2.ContainerizedWorkloadKind
	kindDeployment            = reflect.TypeOf(apps.Deployment{}).Name()
	kindService               = reflect.TypeOf(core.Service{}).Name()
	kindStatefulSet           = reflect.TypeOf(apps.StatefulSet{}).Name()
	kindDaemonSet             = reflect.TypeOf(apps.DaemonSet{}).Name()
)

// HealthCondition holds health status of any resource
type HealthCondition = v1alpha2.HealthCondition

// A WorloadHealthChecker checks health status of specified resource
// and saves status into an HealthCondition object.
type WorloadHealthChecker interface {
	Check(context.Context, client.Client, runtimev1alpha1.TypedReference, string) *HealthCondition
}

// WorkloadHealthCheckFn checks health status of specified resource
// and saves status into an HealthCondition object.
type WorkloadHealthCheckFn func(context.Context, client.Client, runtimev1alpha1.TypedReference, string) *HealthCondition

// Check the health status of specified resource
func (fn WorkloadHealthCheckFn) Check(ctx context.Context, c client.Client, tr runtimev1alpha1.TypedReference, ns string) *HealthCondition {
	return fn(ctx, c, tr, ns)
}

// CheckContainerziedWorkloadHealth check health status of ContainerizedWorkload
func CheckContainerziedWorkloadHealth(ctx context.Context, c client.Client, ref runtimev1alpha1.TypedReference, namespace string) *HealthCondition {
	if ref.GroupVersionKind() != corev1alpha2.SchemeGroupVersion.WithKind(kindContainerizedWorkload) {
		return nil
	}
	r := &HealthCondition{
		HealthStatus:   StatusHealthy,
		TargetWorkload: ref,
	}
	cwObj := corev1alpha2.ContainerizedWorkload{}
	cwObj.SetGroupVersionKind(corev1alpha2.SchemeGroupVersion.WithKind(kindContainerizedWorkload))
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, &cwObj); err != nil {
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}
	//TODO get component name from labels
	r.TargetWorkload.UID = cwObj.GetUID()

	subConditions := []*HealthCondition{}
	childRefs := cwObj.Status.Resources

	for _, childRef := range childRefs {
		switch childRef.Kind {
		case kindDeployment:
			// reuse Deployment health checker
			childCondition := CheckDeploymentHealth(ctx, c, childRef, namespace)
			subConditions = append(subConditions, childCondition)
		case kindService:
			childCondition := &HealthCondition{
				TargetWorkload: childRef,
				HealthStatus:   StatusHealthy,
			}
			o := unstructured.Unstructured{}
			o.SetAPIVersion(childRef.APIVersion)
			o.SetKind(childRef.Kind)
			if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: childRef.Name}, &o); err != nil {
				childCondition.HealthStatus = StatusUnhealthy
				childCondition.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
			}
			subConditions = append(subConditions, childCondition)
		}
	}

	for _, sc := range subConditions {
		if sc.HealthStatus != StatusHealthy {
			r.HealthStatus = StatusUnhealthy
		}
		r.Diagnosis += sc.Diagnosis
	}
	return r
}

// CheckDeploymentHealth checks health status of Deployment
func CheckDeploymentHealth(ctx context.Context, client client.Client, ref runtimev1alpha1.TypedReference, namespace string) *HealthCondition {
	if ref.GroupVersionKind() != apps.SchemeGroupVersion.WithKind(kindDeployment) {
		return nil
	}
	r := &HealthCondition{
		HealthStatus:   StatusUnhealthy,
		TargetWorkload: ref,
	}
	deployment := apps.Deployment{}
	deployment.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindDeployment))
	deploymentRef := types.NamespacedName{Namespace: namespace, Name: ref.Name}
	if err := client.Get(ctx, deploymentRef, &deployment); err != nil {
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}
	r.TargetWorkload.UID = deployment.GetUID()
	r.Diagnosis = fmt.Sprintf(infoFmtReady, deployment.Status.ReadyReplicas, *deployment.Spec.Replicas)

	// Health criteria
	if deployment.Status.ReadyReplicas != *deployment.Spec.Replicas {
		return r
	}
	r.HealthStatus = StatusHealthy
	return r
}

// CheckStatefulsetHealth checks health status of StatefulSet
func CheckStatefulsetHealth(ctx context.Context, client client.Client, ref runtimev1alpha1.TypedReference, namespace string) *HealthCondition {
	if ref.GroupVersionKind() != apps.SchemeGroupVersion.WithKind(kindStatefulSet) {
		return nil
	}
	r := &HealthCondition{
		HealthStatus:   StatusUnhealthy,
		TargetWorkload: ref,
	}
	statefulset := apps.StatefulSet{}
	statefulset.APIVersion = ref.APIVersion
	statefulset.Kind = ref.Kind
	nk := types.NamespacedName{Namespace: namespace, Name: ref.Name}
	if err := client.Get(ctx, nk, &statefulset); err != nil {
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}
	r.TargetWorkload.UID = statefulset.GetUID()
	r.Diagnosis = fmt.Sprintf(infoFmtReady, statefulset.Status.ReadyReplicas, *statefulset.Spec.Replicas)

	// Health criteria
	if statefulset.Status.ReadyReplicas != *statefulset.Spec.Replicas {
		return r
	}
	r.HealthStatus = StatusUnhealthy
	return r
}

// CheckDaemonsetHealth checks health status of DaemonSet
func CheckDaemonsetHealth(ctx context.Context, client client.Client, ref runtimev1alpha1.TypedReference, namespace string) *HealthCondition {
	if ref.GroupVersionKind() != apps.SchemeGroupVersion.WithKind(kindDaemonSet) {
		return nil
	}
	r := &HealthCondition{
		HealthStatus:   StatusUnhealthy,
		TargetWorkload: ref,
	}
	daemonset := apps.DaemonSet{}
	daemonset.APIVersion = ref.APIVersion
	daemonset.Kind = ref.Kind
	nk := types.NamespacedName{Namespace: namespace, Name: ref.Name}
	if err := client.Get(ctx, nk, &daemonset); err != nil {
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}
	r.TargetWorkload.UID = daemonset.GetUID()
	r.Diagnosis = fmt.Sprintf(infoFmtReady, daemonset.Status.NumberReady, daemonset.Status.DesiredNumberScheduled)

	// Health criteria
	if daemonset.Status.NumberReady != daemonset.Status.DesiredNumberScheduled {
		return r
	}
	r.HealthStatus = StatusHealthy
	return r
}

// func (r *Reconciler) listHealthCheckTrait(ctx context.Context, ns string) ([]unstructured.Unstructured, error) {
//     //TODO Get HealthCheckTrait GVK dynamically
//     hcTraitList := &unstructured.UnstructuredList{}
//     hcTraitList.SetAPIVersion("extend.oam.dev/v1alpha2")
//     hcTraitList.SetKind("HealthCheckTrait")
//
//     if err := r.client.List(ctx, hcTraitList); err != nil {
//         return nil, err
//     }
//     return hcTraitList.Items, nil
//
// }

func (r *Reconciler) getHealthConditionFromTrait(ctx context.Context, wlRef runtimev1alpha1.TypedReference, ns string) (*HealthCondition, error) {
	// get workload instance

	// get component name

	// get trait name by component name

	hcTraitList := &unstructured.UnstructuredList{}
	hcTraitList.SetAPIVersion("extend.oam.dev/v1alpha2")
	hcTraitList.SetKind("HealthCheckTrait")

	if err := r.client.List(ctx, hcTraitList); err != nil {
		return nil, err
	}

	return nil, nil
}

func (r *Reconciler) getUnknownWorkloadHealthCondition(ctx context.Context, wlRef runtimev1alpha1.TypedReference, ns string) *HealthCondition {
	healthCondition := &HealthCondition{
		TargetWorkload: wlRef,
		HealthStatus:   StatusUnknown,
		Diagnosis:      fmt.Sprintf(infoFmtUnknownWorkload, wlRef.APIVersion, wlRef.Kind),
	}

	wl := &unstructured.Unstructured{}
	wl.SetGroupVersionKind(wlRef.GroupVersionKind())
	if err := r.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: wlRef.Name}, wl); err != nil {
		healthCondition.Diagnosis += errors.Wrap(err, errHealthCheck).Error()
		return healthCondition
	}
	wlBytes, err := wl.MarshalJSON()
	if err != nil {
		healthCondition.Diagnosis += errors.Wrap(err, errHealthCheck).Error()
		return healthCondition
	}
	healthCondition.WorkloadInfo = string(wlBytes)
	return healthCondition
}
