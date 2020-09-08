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

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/pkg/errors"
)

const (
	errFmtUnsupportWorkload   = "APIVersion %v Kind %v workload is not supportted by HealthScope"
	errHealthCheck            = "error occurs in health check"
	errUnhealthyChildResource = "unhealthy child resource exists"
	errFmtResourceNotReady    = "resource not ready, resource status: %+v"

	defaultTimeout = 10 * time.Second
)

var (
	kindContainerizedWorkload = corev1alpha2.ContainerizedWorkloadKind
	kindDeployment            = reflect.TypeOf(apps.Deployment{}).Name()
	kindService               = reflect.TypeOf(core.Service{}).Name()
	kindStatefulSet           = reflect.TypeOf(apps.StatefulSet{}).Name()
	kindDaemonSet             = reflect.TypeOf(apps.DaemonSet{}).Name()
)

// HealthCondition holds health status of any resource
type HealthCondition struct {
	// Target represents resource being diagnosed
	Target runtimev1alpha1.TypedReference `json:"target"`

	IsHealthy bool `json:"isHealthy"`

	// Diagnosis contains diagnosis info as well as error info
	Diagnosis string `json:"diagnosis,omitempty"`

	// SubConditions represents health status of its child resources, if exist
	SubConditions []*HealthCondition `json:"subConditions,omitempty"`
}

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
		IsHealthy: false,
		Target:    ref,
	}
	cwObj := corev1alpha2.ContainerizedWorkload{}
	cwObj.SetGroupVersionKind(corev1alpha2.SchemeGroupVersion.WithKind(kindContainerizedWorkload))
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, &cwObj); err != nil {
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}
	r.Target.UID = cwObj.GetUID()

	r.SubConditions = []*HealthCondition{}
	childRefs := cwObj.Status.Resources

	for _, childRef := range childRefs {
		switch childRef.Kind {
		case kindDeployment:
			// reuse Deployment health checker
			childCondition := CheckDeploymentHealth(ctx, c, childRef, namespace)
			r.SubConditions = append(r.SubConditions, childCondition)
		default:
			childCondition := &HealthCondition{
				Target:    childRef,
				IsHealthy: true,
			}
			o := unstructured.Unstructured{}
			o.SetAPIVersion(childRef.APIVersion)
			o.SetKind(childRef.Kind)
			if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: childRef.Name}, &o); err != nil {
				// for unspecified resource
				// if cannot get it, then check fails
				childCondition.IsHealthy = false
				childCondition.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
			}
			r.SubConditions = append(r.SubConditions, childCondition)
		}
	}

	r.IsHealthy = true
	for _, sc := range r.SubConditions {
		if !sc.IsHealthy {
			r.IsHealthy = false
			r.Diagnosis = errUnhealthyChildResource
			break
		}
	}
	return r
}

// CheckDeploymentHealth checks health status of Deployment
func CheckDeploymentHealth(ctx context.Context, client client.Client, ref runtimev1alpha1.TypedReference, namespace string) *HealthCondition {
	if ref.GroupVersionKind() != apps.SchemeGroupVersion.WithKind(kindDeployment) {
		return nil
	}
	r := &HealthCondition{
		IsHealthy: false,
		Target:    ref,
	}
	deployment := apps.Deployment{}
	deployment.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindDeployment))
	deploymentRef := types.NamespacedName{Namespace: namespace, Name: ref.Name}
	if err := client.Get(ctx, deploymentRef, &deployment); err != nil {
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}
	r.Target.UID = deployment.GetUID()

	if deployment.Status.ReadyReplicas == 0 {
		r.Diagnosis = fmt.Sprintf(errFmtResourceNotReady, deployment.Status)
		return r
	}
	r.IsHealthy = true
	return r
}

// CheckStatefulsetHealth checks health status of StatefulSet
func CheckStatefulsetHealth(ctx context.Context, client client.Client, ref runtimev1alpha1.TypedReference, namespace string) *HealthCondition {
	if ref.GroupVersionKind() != apps.SchemeGroupVersion.WithKind(kindStatefulSet) {
		return nil
	}
	r := &HealthCondition{
		IsHealthy: false,
		Target:    ref,
	}
	statefulset := apps.StatefulSet{}
	statefulset.APIVersion = ref.APIVersion
	statefulset.Kind = ref.Kind
	nk := types.NamespacedName{Namespace: namespace, Name: ref.Name}
	if err := client.Get(ctx, nk, &statefulset); err != nil {
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}
	r.Target.UID = statefulset.GetUID()

	if statefulset.Status.ReadyReplicas == 0 {
		r.Diagnosis = fmt.Sprintf(errFmtResourceNotReady, statefulset.Status)
		return r
	}
	r.IsHealthy = true
	return r
}

// CheckDaemonsetHealth checks health status of DaemonSet
func CheckDaemonsetHealth(ctx context.Context, client client.Client, ref runtimev1alpha1.TypedReference, namespace string) *HealthCondition {
	if ref.GroupVersionKind() != apps.SchemeGroupVersion.WithKind(kindDaemonSet) {
		return nil
	}
	r := &HealthCondition{
		IsHealthy: false,
		Target:    ref,
	}
	daemonset := apps.DaemonSet{}
	daemonset.APIVersion = ref.APIVersion
	daemonset.Kind = ref.Kind
	nk := types.NamespacedName{Namespace: namespace, Name: ref.Name}
	if err := client.Get(ctx, nk, &daemonset); err != nil {
		r.Diagnosis = errors.Wrap(err, errHealthCheck).Error()
		return r
	}
	r.Target.UID = daemonset.GetUID()

	if daemonset.Status.NumberUnavailable != 0 {
		r.Diagnosis = fmt.Sprintf(errFmtResourceNotReady, daemonset.Status)
		return r
	}
	r.IsHealthy = true
	return r
}
