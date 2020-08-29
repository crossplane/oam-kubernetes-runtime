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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

const (
	// workloadName = "myWorkload"
	namespace = "ns"
)

var (
	ctx        = context.Background()
	errMockErr = errors.New("get error")
)

func TestCheckContainerziedWorkloadHealth(t *testing.T) {
	mockClient := test.NewMockClient()
	cwRef := runtimev1alpha1.TypedReference{}
	cwRef.SetGroupVersionKind(corev1alpha2.SchemeGroupVersion.WithKind(kindContainerizedWorkload))
	deployRef := runtimev1alpha1.TypedReference{}
	deployRef.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindDeployment))
	svcRef := runtimev1alpha1.TypedReference{}
	svcRef.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindService))
	cw := corev1alpha2.ContainerizedWorkload{
		Status: corev1alpha2.ContainerizedWorkloadStatus{
			Resources: []runtimev1alpha1.TypedReference{deployRef, svcRef},
		},
	}

	tests := []struct {
		caseName  string
		mockGetFn test.MockGetFn
		wlRef     runtimev1alpha1.TypedReference
		expect    *HealthCondition
	}{
		{
			caseName: "not matched checker",
			wlRef:    runtimev1alpha1.TypedReference{},
			expect:   nil,
		},
		{
			caseName: "healthy workload",
			wlRef:    cwRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				if o, ok := obj.(*corev1alpha2.ContainerizedWorkload); ok {
					*o = cw
					return nil
				}
				if o, ok := obj.(*apps.Deployment); ok {
					*o = apps.Deployment{
						Status: apps.DeploymentStatus{
							ReadyReplicas: 1, // healthy
						},
					}
				}
				return nil
			},
			expect: &HealthCondition{
				HealthStatus: StatusHealthy,
			},
		},
		{
			caseName: "unhealthy for deployment not ready",
			wlRef:    cwRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				if o, ok := obj.(*corev1alpha2.ContainerizedWorkload); ok {
					*o = cw
					return nil
				}
				if o, ok := obj.(*apps.Deployment); ok {
					*o = apps.Deployment{
						Status: apps.DeploymentStatus{
							ReadyReplicas: 0, // unhealthy
						},
					}
				}
				return nil
			},
			expect: &HealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
		{
			caseName: "unhealthy for ContainerizedWorkload not found",
			wlRef:    cwRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				return errMockErr
			},
			expect: &HealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
		{
			caseName: "unhealthy for deployment not found",
			wlRef:    cwRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				if o, ok := obj.(*corev1alpha2.ContainerizedWorkload); ok {
					*o = cw
					return nil
				}
				if _, ok := obj.(*apps.Deployment); ok {
					return errMockErr
				}
				return nil
			},
			expect: &HealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
		{
			caseName: "unhealthy for service not found",
			wlRef:    cwRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				switch o := obj.(type) {
				case *corev1alpha2.ContainerizedWorkload:
					*o = cw
				case *apps.Deployment:
					*o = apps.Deployment{
						Status: apps.DeploymentStatus{
							ReadyReplicas: 1, // healthy
						},
					}
				case *unstructured.Unstructured:
					return errMockErr
				}
				return nil
			},
			expect: &HealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
	}

	for _, tc := range tests {
		func(t *testing.T) {
			mockClient.MockGet = tc.mockGetFn
			result := CheckContainerziedWorkloadHealth(ctx, mockClient, tc.wlRef, namespace)
			if tc.expect == nil {
				assert.Nil(t, result, tc.caseName)
			} else {
				assert.Equal(t, tc.expect.HealthStatus, result.HealthStatus, tc.caseName)
			}

		}(t)
	}
}

func TestCheckDeploymentHealth(t *testing.T) {
	mockClient := test.NewMockClient()
	deployRef := runtimev1alpha1.TypedReference{}
	deployRef.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindDeployment))

	tests := []struct {
		caseName  string
		mockGetFn test.MockGetFn
		wlRef     runtimev1alpha1.TypedReference
		expect    *HealthCondition
	}{
		{
			caseName: "not matched checker",
			wlRef:    runtimev1alpha1.TypedReference{},
			expect:   nil,
		},
		{
			caseName: "healthy workload",
			wlRef:    deployRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				if o, ok := obj.(*apps.Deployment); ok {
					*o = apps.Deployment{
						Status: apps.DeploymentStatus{
							ReadyReplicas: 1, // healthy
						},
					}
				}
				return nil
			},
			expect: &HealthCondition{
				HealthStatus: StatusHealthy,
			},
		},
		{
			caseName: "unhealthy for deployment not ready",
			wlRef:    deployRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				if o, ok := obj.(*apps.Deployment); ok {
					*o = apps.Deployment{
						Status: apps.DeploymentStatus{
							ReadyReplicas: 0, // unhealthy
						},
					}
				}
				return nil
			},
			expect: &HealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
		{
			caseName: "unhealthy for deployment not found",
			wlRef:    deployRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				return errMockErr
			},
			expect: &HealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
	}

	for _, tc := range tests {
		func(t *testing.T) {
			mockClient.MockGet = tc.mockGetFn
			result := CheckDeploymentHealth(ctx, mockClient, tc.wlRef, namespace)
			if tc.expect == nil {
				assert.Nil(t, result, tc.caseName)
			} else {
				assert.Equal(t, tc.expect.HealthStatus, result.HealthStatus, tc.caseName)
			}
		}(t)
	}
}

func TestCheckStatefulsetHealth(t *testing.T) {
	mockClient := test.NewMockClient()
	stsRef := runtimev1alpha1.TypedReference{}
	stsRef.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindStatefulSet))

	tests := []struct {
		caseName  string
		mockGetFn test.MockGetFn
		wlRef     runtimev1alpha1.TypedReference
		expect    *HealthCondition
	}{
		{
			caseName: "not matched checker",
			wlRef:    runtimev1alpha1.TypedReference{},
			expect:   nil,
		},
		{
			caseName: "healthy workload",
			wlRef:    stsRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				if o, ok := obj.(*apps.StatefulSet); ok {
					*o = apps.StatefulSet{
						Status: apps.StatefulSetStatus{
							ReadyReplicas: 1, // healthy
						},
					}
				}
				return nil
			},
			expect: &HealthCondition{
				HealthStatus: StatusHealthy,
			},
		},
		{
			caseName: "unhealthy for statefulset not ready",
			wlRef:    stsRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				if o, ok := obj.(*apps.StatefulSet); ok {
					*o = apps.StatefulSet{
						Status: apps.StatefulSetStatus{
							ReadyReplicas: 0, // unhealthy
						},
					}
				}
				return nil
			},
			expect: &HealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
		{
			caseName: "unhealthy for statefulset not found",
			wlRef:    stsRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				return errMockErr
			},
			expect: &HealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
	}

	for _, tc := range tests {
		func(t *testing.T) {
			mockClient.MockGet = tc.mockGetFn
			result := CheckStatefulsetHealth(ctx, mockClient, tc.wlRef, namespace)
			if tc.expect == nil {
				assert.Nil(t, result, tc.caseName)
			} else {
				assert.Equal(t, tc.expect.HealthStatus, result.HealthStatus, tc.caseName)
			}
		}(t)
	}
}

func TestCheckDaemonsetHealth(t *testing.T) {
	mockClient := test.NewMockClient()
	dstRef := runtimev1alpha1.TypedReference{}
	dstRef.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindDaemonSet))

	tests := []struct {
		caseName  string
		mockGetFn test.MockGetFn
		wlRef     runtimev1alpha1.TypedReference
		expect    *HealthCondition
	}{
		{
			caseName: "not matched checker",
			wlRef:    runtimev1alpha1.TypedReference{},
			expect:   nil,
		},
		{
			caseName: "healthy workload",
			wlRef:    dstRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				if o, ok := obj.(*apps.DaemonSet); ok {
					*o = apps.DaemonSet{
						Status: apps.DaemonSetStatus{
							NumberUnavailable: 0, // healthy
						},
					}
				}
				return nil
			},
			expect: &HealthCondition{
				HealthStatus: StatusHealthy,
			},
		},
		{
			caseName: "unhealthy for daemonset not ready",
			wlRef:    dstRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				if o, ok := obj.(*apps.DaemonSet); ok {
					*o = apps.DaemonSet{
						Status: apps.DaemonSetStatus{
							NumberUnavailable: 1, // unhealthy
						},
					}
				}
				return nil
			},
			expect: &HealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
		{
			caseName: "unhealthy for daemonset not found",
			wlRef:    dstRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				return errMockErr
			},
			expect: &HealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
	}

	for _, tc := range tests {
		func(t *testing.T) {
			mockClient.MockGet = tc.mockGetFn
			result := CheckDaemonsetHealth(ctx, mockClient, tc.wlRef, namespace)
			if tc.expect == nil {
				assert.Nil(t, result, tc.caseName)
			} else {
				assert.Equal(t, tc.expect.HealthStatus, result.HealthStatus, tc.caseName)
			}
		}(t)
	}
}
