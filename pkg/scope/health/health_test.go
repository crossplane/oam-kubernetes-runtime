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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

const (
	workloadName = "myWorkload"
)

func TestUpdateHealthStatus(t *testing.T) {
	type args struct {
		client      client.Client
		healthScope *v1alpha2.HealthScope
	}

	type want struct {
		err    error
		health string
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NoWorkloadRef": {
			reason: "Health scope is empty.",
			args: args{
				client: &test.MockClient{
					MockGet: func(_ context.Context, _ client.ObjectKey, obj runtime.Object) error {
						return nil
					},
				},
				healthScope: &v1alpha2.HealthScope{},
			},
			want: want{
				err:    nil,
				health: "healthy",
			},
		},
		"ErrorGettingWorkload": {
			reason: "Health scope reports error if cannot find workload.",
			args: args{
				client: &test.MockClient{
					MockGet: func(_ context.Context, _ client.ObjectKey, obj runtime.Object) error {
						return fmt.Errorf("cannot get workload")
					},
				},
				healthScope: &v1alpha2.HealthScope{
					Spec: v1alpha2.HealthScopeSpec{
						WorkloadReferences: []v1alpha1.TypedReference{
							{
								APIVersion: "core.oam.dev/v1alpha2",
								Kind:       "ContainerizedWorkload",
								Name:       workloadName,
							},
						},
					},
				},
			},
			want: want{
				err:    errors.Wrapf(fmt.Errorf("cannot get workload"), errNoWorkload, workloadName),
				health: "",
			},
		},
		"WorkloadWithBadData": {
			reason: "Health scope has one healthy resource.",
			args: args{
				client: &test.MockClient{
					MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
						if key.Name == workloadName {
							workload := obj.(*unstructured.Unstructured)

							// Bad data injected here.
							if err := fieldpath.Pave(workload.UnstructuredContent()).SetValue("status", "BAD_DATA"); err == nil {
								return err
							}

							return nil
						}

						return fmt.Errorf("Unexpected key")
					},
				},
				healthScope: &v1alpha2.HealthScope{
					Spec: v1alpha2.HealthScopeSpec{
						WorkloadReferences: []v1alpha1.TypedReference{
							{
								APIVersion: "core.oam.dev/v1alpha2",
								Kind:       "ContainerizedWorkload",
								Name:       workloadName,
							},
						},
					},
				},
			},
			want: want{
				err:    errors.Wrapf(errors.Errorf("status: not an object"), errNoWorkloadResources, workloadName),
				health: "",
			},
		},
		"OneHealthyWorkloadRef": {
			reason: "Health scope has one healthy resource.",
			args: args{
				client: &test.MockClient{
					MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
						if key.Name == workloadName {
							workload := obj.(*unstructured.Unstructured)

							refs := []interface{}{
								map[string]interface{}{
									"apiVersion": "myVersion",
									"kind":       "myKind",
									"name":       "myName",
								},
							}

							if err := fieldpath.Pave(workload.UnstructuredContent()).SetValue("status.resources", refs); err == nil {
								return err
							}

							return nil
						}

						if key.Name == "myName" {
							return nil
						}

						return fmt.Errorf("Unexpected key")
					},
				},
				healthScope: &v1alpha2.HealthScope{
					Spec: v1alpha2.HealthScopeSpec{
						WorkloadReferences: []v1alpha1.TypedReference{
							{
								APIVersion: "core.oam.dev/v1alpha2",
								Kind:       "ContainerizedWorkload",
								Name:       workloadName,
							},
						},
					},
				},
			},
			want: want{
				err:    nil,
				health: "healthy",
			},
		},
		"OneUnhealthyWorkloadRefOnly": {
			reason: "Health scope has one unhealthy resource.",
			args: args{
				client: &test.MockClient{
					MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
						if key.Name == workloadName {
							workload := obj.(*unstructured.Unstructured)

							refs := []interface{}{
								map[string]interface{}{
									"apiVersion": "myVersion",
									"kind":       "myKind",
									"name":       "NotFound",
								},
							}

							if err := fieldpath.Pave(workload.UnstructuredContent()).SetValue("status.resources", refs); err == nil {
								return err
							}

							return nil
						}

						return fmt.Errorf("Not found")
					},
				},
				healthScope: &v1alpha2.HealthScope{
					Spec: v1alpha2.HealthScopeSpec{
						WorkloadReferences: []v1alpha1.TypedReference{
							{
								APIVersion: "core.oam.dev/v1alpha2",
								Kind:       "ContainerizedWorkload",
								Name:       workloadName,
							},
						},
					},
				},
			},
			want: want{
				err:    nil,
				health: "unhealthy",
			},
		},
		"OneHealthyDeploymentAndOneUnknownResource": {
			reason: "Health scope handles Deployment specially and aggregates health checks to unhealthy.",
			args: args{
				client: &test.MockClient{
					MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
						if key.Name == workloadName {
							workload := obj.(*unstructured.Unstructured)

							refs := []interface{}{
								map[string]interface{}{
									"apiVersion": "apps/v1",
									"kind":       "Deployment",
									"name":       "myDeployment",
								},
								map[string]interface{}{
									"apiVersion": "myVersion",
									"kind":       "myKind",
									"name":       "NotFound",
								},
							}

							if err := fieldpath.Pave(workload.UnstructuredContent()).SetValue("status.resources", refs); err == nil {
								return err
							}

							return nil
						}

						if key.Name == "myDeployment" {
							deployment := obj.(*apps.Deployment)

							deployment.Status.ReadyReplicas = 1

							return nil
						}

						return fmt.Errorf("Not found")
					},
				},
				healthScope: &v1alpha2.HealthScope{
					Spec: v1alpha2.HealthScopeSpec{
						WorkloadReferences: []v1alpha1.TypedReference{
							{
								APIVersion: "core.oam.dev/v1alpha2",
								Kind:       "ContainerizedWorkload",
								Name:       workloadName,
							},
						},
					},
				},
			},
			want: want{
				err:    nil,
				health: "unhealthy",
			},
		},
		"OneHealthyDeploymentAndOneHealthyResource": {
			reason: "Health scope handles Deployment specially and aggregates health checks to healthy.",
			args: args{
				client: &test.MockClient{
					MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
						if key.Name == workloadName {
							workload := obj.(*unstructured.Unstructured)

							refs := []interface{}{
								map[string]interface{}{
									"apiVersion": "apps/v1",
									"kind":       "Deployment",
									"name":       "myDeployment",
								},
								map[string]interface{}{
									"apiVersion": "myVersion",
									"kind":       "myKind",
									"name":       "myName",
								},
							}

							if err := fieldpath.Pave(workload.UnstructuredContent()).SetValue("status.resources", refs); err == nil {
								return err
							}

							return nil
						}

						if key.Name == "myName" {
							return nil
						}

						if key.Name == "myDeployment" {
							deployment := obj.(*apps.Deployment)

							deployment.Status.ReadyReplicas = 1

							return nil
						}

						return fmt.Errorf("Unexpected")
					},
				},
				healthScope: &v1alpha2.HealthScope{
					Spec: v1alpha2.HealthScopeSpec{
						WorkloadReferences: []v1alpha1.TypedReference{
							{
								APIVersion: "core.oam.dev/v1alpha2",
								Kind:       "ContainerizedWorkload",
								Name:       workloadName,
							},
						},
					},
				},
			},
			want: want{
				err:    nil,
				health: "healthy",
			},
		},
		"DeploymentNotReady": {
			reason: "Health scope handles Deployment specially for ready instances.",
			args: args{
				client: &test.MockClient{
					MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
						if key.Name == workloadName {
							workload := obj.(*unstructured.Unstructured)

							refs := []interface{}{
								map[string]interface{}{
									"apiVersion": "apps/v1",
									"kind":       "Deployment",
									"name":       "myDeployment",
								},
							}

							if err := fieldpath.Pave(workload.UnstructuredContent()).SetValue("status.resources", refs); err == nil {
								return err
							}

							return nil
						}

						if key.Name == "myDeployment" {
							deployment := obj.(*apps.Deployment)

							// artursouza: fails health check for this.
							deployment.Status.ReadyReplicas = 0

							return nil
						}

						return fmt.Errorf("Unexpected")
					},
				},
				healthScope: &v1alpha2.HealthScope{
					Spec: v1alpha2.HealthScopeSpec{
						WorkloadReferences: []v1alpha1.TypedReference{
							{
								APIVersion: "core.oam.dev/v1alpha2",
								Kind:       "ContainerizedWorkload",
								Name:       workloadName,
							},
						},
					},
				},
			},
			want: want{
				err:    nil,
				health: "unhealthy",
			},
		},
		"DeploymentNotFound": {
			reason: "Health scope handles Deployment when not found.",
			args: args{
				client: &test.MockClient{
					MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
						if key.Name == workloadName {
							workload := obj.(*unstructured.Unstructured)

							refs := []interface{}{
								map[string]interface{}{
									"apiVersion": "apps/v1",
									"kind":       "Deployment",
									"name":       "myDeploymentNotFound",
								},
							}

							if err := fieldpath.Pave(workload.UnstructuredContent()).SetValue("status.resources", refs); err == nil {
								return err
							}

							return nil
						}

						return fmt.Errorf("Not found")
					},
				},
				healthScope: &v1alpha2.HealthScope{
					Spec: v1alpha2.HealthScopeSpec{
						WorkloadReferences: []v1alpha1.TypedReference{
							{
								APIVersion: "core.oam.dev/v1alpha2",
								Kind:       "ContainerizedWorkload",
								Name:       workloadName,
							},
						},
					},
				},
			},
			want: want{
				err:    nil,
				health: "unhealthy",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			log := logging.NewNopLogger()
			scope := tc.args.healthScope
			err := UpdateHealthStatus(context.Background(), log, tc.args.client, scope)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\nReason: %s\nUpdateHealthStatus(...): -want error, +got error:\n%s", tc.reason, diff)
			}

			if diff := cmp.Diff(tc.want.health, scope.Status.Health, test.EquateErrors()); diff != "" {
				t.Errorf("\nReason: %s\nUpdateHealthStatus(...): -want health, +got health:\n%s", tc.reason, diff)
			}
		})
	}
}
