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

package health

import (
	"context"
	"fmt"
	"sync"
	"time"

	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/pkg/errors"
)

const (
	errNoWorkload            = "could not retrieve workload %q"
	errNoWorkloadResources   = "could not retrieve resources for workload %q"
	errResourceNotFound      = "could not retrieve resource %q %q %q"
	errDeploymentUnavailable = "no ready instance found in %q %q %q"

	defaultTimeout = 10 * time.Second
)

// UpdateHealthStatus updates the status of the healthscope based on workload resources.
func UpdateHealthStatus(ctx context.Context, log logging.Logger, client client.Client, healthScope *v1alpha2.HealthScope) error {
	timeout := defaultTimeout
	if healthScope.Spec.ProbeTimeout != nil {
		timeout = time.Duration(*healthScope.Spec.ProbeTimeout) * time.Second
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resourceRefs := []runtimev1alpha1.TypedReference{}
	for _, workloadRef := range healthScope.Spec.WorkloadReferences {
		// Get workload object.
		workloadObject := unstructured.Unstructured{}
		workloadObject.SetAPIVersion(workloadRef.APIVersion)
		workloadObject.SetKind(workloadRef.Kind)
		workloadObjectRef := types.NamespacedName{Namespace: healthScope.GetNamespace(), Name: workloadRef.Name}
		if err := client.Get(ctxWithTimeout, workloadObjectRef, &workloadObject); err != nil {
			return errors.Wrapf(err, errNoWorkload, workloadRef.Name)
		}

		// TODO(artursouza): not every workload has child resources, need to handle those scenarios too.
		// TODO(artursouza): change this to use an utility method instead.
		if value, err := fieldpath.Pave(workloadObject.UnstructuredContent()).GetValue("status.resources"); err == nil {
			refs := value.([]interface{})
			for _, item := range refs {
				ref := item.(map[string]interface{})
				resourceRef := runtimev1alpha1.TypedReference{
					APIVersion: fmt.Sprintf("%v", ref["apiVersion"]),
					Kind:       fmt.Sprintf("%v", ref["kind"]),
					Name:       fmt.Sprintf("%v", ref["name"]),
				}

				resourceRefs = append(resourceRefs, resourceRef)
			}
		} else {
			return errors.Wrapf(err, errNoWorkloadResources, workloadRef.Name)
		}
	}

	statusc := resourcesHealthStatus(ctxWithTimeout, log, client, healthScope.Namespace, resourceRefs)
	status := true
	for r := range statusc {
		status = status && r
	}

	health := "unhealthy"
	if status {
		health = "healthy"
	}

	healthScope.Status.Health = health
	return nil
}

func resourcesHealthStatus(ctx context.Context, log logging.Logger, client client.Client, namespace string, refs []runtimev1alpha1.TypedReference) <-chan bool {
	status := make(chan bool, len(refs))
	var wg sync.WaitGroup
	wg.Add(len(refs))
	for _, ref := range refs {
		go func(resourceRef runtimev1alpha1.TypedReference) {
			defer wg.Done()
			err := resourceHealthStatus(ctx, client, namespace, resourceRef)
			status <- (err == nil)
			if err != nil {
				log.Debug("Unhealthy resource", "resource", resourceRef.Name, "error", err)
			}
		}(ref)
	}
	go func() {
		wg.Wait()
		close(status)
	}()

	return status
}

func resourceHealthStatus(ctx context.Context, client client.Client, namespace string, ref runtimev1alpha1.TypedReference) error {
	if ref.GroupVersionKind() == apps.SchemeGroupVersion.WithKind("Deployment") {
		return deploymentHealthStatus(ctx, client, namespace, ref)
	}

	// TODO(artursouza): add other health checks.
	// Generic health check by validating if the resource exists.
	object := unstructured.Unstructured{}
	object.SetAPIVersion(ref.APIVersion)
	object.SetKind(ref.Kind)
	objectRef := types.NamespacedName{Namespace: namespace, Name: ref.Name}
	err := client.Get(ctx, objectRef, &object)
	return err
}

func deploymentHealthStatus(ctx context.Context, client client.Client, namespace string, ref runtimev1alpha1.TypedReference) error {
	deployment := apps.Deployment{}
	deployment.APIVersion = ref.APIVersion
	deployment.Kind = ref.Kind
	deploymentRef := types.NamespacedName{Namespace: namespace, Name: ref.Name}
	if err := client.Get(ctx, deploymentRef, &deployment); err != nil {
		return errors.Wrapf(err, errResourceNotFound, ref.APIVersion, ref.Kind, ref.Name)
	}

	if deployment.Status.ReadyReplicas == 0 {
		return fmt.Errorf(errDeploymentUnavailable, ref.APIVersion, ref.Kind, ref.Name)
	}
	return nil
}
