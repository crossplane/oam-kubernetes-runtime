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

package containerizedworkload

import (
	"context"
	"errors"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
)

var (
	deploymentKind       = reflect.TypeOf(appsv1.Deployment{}).Name()
	deploymentAPIVersion = appsv1.SchemeGroupVersion.String()
	serviceKind          = reflect.TypeOf(corev1.Service{}).Name()
	serviceAPIVersion    = corev1.SchemeGroupVersion.String()
)

// Reconcile error strings.
const (
	labelKey = "containerizedworkload.oam.crossplane.io"

	errNotContainerizedWorkload = "object is not a containerized workload"
)

// TranslateContainerWorkload translates a ContainerizedWorkload into a Deployment.
// nolint:gocyclo
func TranslateContainerWorkload(ctx context.Context, w oam.Workload) ([]oam.Object, error) {
	cw, ok := w.(*v1alpha2.ContainerizedWorkload)
	if !ok {
		return nil, errors.New(errNotContainerizedWorkload)
	}

	d := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       deploymentKind,
			APIVersion: deploymentAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cw.GetName(),
			Namespace: w.GetNamespace(),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					labelKey: string(cw.GetUID()),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						labelKey: string(cw.GetUID()),
					},
				},
			},
		},
	}
	if cw.Spec.OperatingSystem != nil {
		if d.Spec.Template.Spec.NodeSelector == nil {
			d.Spec.Template.Spec.NodeSelector = map[string]string{}
		}
		d.Spec.Template.Spec.NodeSelector["beta.kubernetes.io/os"] = string(*cw.Spec.OperatingSystem)
	}

	if cw.Spec.CPUArchitecture != nil {
		if d.Spec.Template.Spec.NodeSelector == nil {
			d.Spec.Template.Spec.NodeSelector = map[string]string{}
		}
		d.Spec.Template.Spec.NodeSelector["kubernetes.io/arch"] = string(*cw.Spec.CPUArchitecture)
	}

	for _, container := range cw.Spec.Containers {
		if container.ImagePullSecret != nil {
			d.Spec.Template.Spec.ImagePullSecrets = append(d.Spec.Template.Spec.ImagePullSecrets, corev1.LocalObjectReference{
				Name: *container.ImagePullSecret,
			})
		}
		kubernetesContainer := corev1.Container{
			Name:    container.Name,
			Image:   container.Image,
			Command: container.Command,
			Args:    container.Arguments,
		}

		if container.Resources != nil {
			kubernetesContainer.Resources = corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    container.Resources.CPU.Required,
					corev1.ResourceMemory: container.Resources.Memory.Required,
				},
			}
			for _, v := range container.Resources.Volumes {
				mount := corev1.VolumeMount{
					Name:      v.Name,
					MountPath: v.MountPath,
				}
				if v.AccessMode != nil && *v.AccessMode == v1alpha2.VolumeAccessModeRO {
					mount.ReadOnly = true
				}
				kubernetesContainer.VolumeMounts = append(kubernetesContainer.VolumeMounts, mount)

			}
		}

		for _, p := range container.Ports {
			port := corev1.ContainerPort{
				Name:          p.Name,
				ContainerPort: p.Port,
			}
			if p.Protocol != nil {
				port.Protocol = corev1.Protocol(*p.Protocol)
			}
			kubernetesContainer.Ports = append(kubernetesContainer.Ports, port)
		}

		for _, e := range container.Environment {
			if e.Value != nil {
				kubernetesContainer.Env = append(kubernetesContainer.Env, corev1.EnvVar{
					Name:  e.Name,
					Value: *e.Value,
				})
				continue
			}
			if e.FromSecret != nil {
				kubernetesContainer.Env = append(kubernetesContainer.Env, corev1.EnvVar{
					Name: e.Name,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							Key: e.FromSecret.Key,
							LocalObjectReference: corev1.LocalObjectReference{
								Name: e.FromSecret.Name,
							},
						},
					},
				})
			}
		}

		if container.LivenessProbe != nil {
			kubernetesContainer.LivenessProbe = &corev1.Probe{}
			if container.LivenessProbe.InitialDelaySeconds != nil {
				kubernetesContainer.LivenessProbe.InitialDelaySeconds = *container.LivenessProbe.InitialDelaySeconds
			}
			if container.LivenessProbe.TimeoutSeconds != nil {
				kubernetesContainer.LivenessProbe.TimeoutSeconds = *container.LivenessProbe.TimeoutSeconds
			}
			if container.LivenessProbe.PeriodSeconds != nil {
				kubernetesContainer.LivenessProbe.PeriodSeconds = *container.LivenessProbe.PeriodSeconds
			}
			if container.LivenessProbe.SuccessThreshold != nil {
				kubernetesContainer.LivenessProbe.SuccessThreshold = *container.LivenessProbe.SuccessThreshold
			}
			if container.LivenessProbe.FailureThreshold != nil {
				kubernetesContainer.LivenessProbe.FailureThreshold = *container.LivenessProbe.FailureThreshold
			}

			// NOTE(hasheddan): Kubernetes specifies that only one type of
			// handler should be provided. OAM does not impose that same
			// restriction. We optimistically check all and set whatever is
			// provided.
			if container.LivenessProbe.HTTPGet != nil {
				kubernetesContainer.LivenessProbe.Handler.HTTPGet = &corev1.HTTPGetAction{
					Path: container.LivenessProbe.HTTPGet.Path,
					Port: intstr.IntOrString{IntVal: container.LivenessProbe.HTTPGet.Port},
				}

				for _, h := range container.LivenessProbe.HTTPGet.HTTPHeaders {
					kubernetesContainer.LivenessProbe.Handler.HTTPGet.HTTPHeaders = append(kubernetesContainer.LivenessProbe.Handler.HTTPGet.HTTPHeaders, corev1.HTTPHeader{
						Name:  h.Name,
						Value: h.Value,
					})
				}
			}
			if container.LivenessProbe.Exec != nil {
				kubernetesContainer.LivenessProbe.Exec = &corev1.ExecAction{
					Command: container.LivenessProbe.Exec.Command,
				}
			}
			if container.LivenessProbe.TCPSocket != nil {
				kubernetesContainer.LivenessProbe.TCPSocket = &corev1.TCPSocketAction{
					Port: intstr.IntOrString{IntVal: container.LivenessProbe.TCPSocket.Port},
				}
			}
		}

		if container.ReadinessProbe != nil {
			kubernetesContainer.ReadinessProbe = &corev1.Probe{}
			if container.ReadinessProbe.InitialDelaySeconds != nil {
				kubernetesContainer.ReadinessProbe.InitialDelaySeconds = *container.ReadinessProbe.InitialDelaySeconds
			}
			if container.ReadinessProbe.TimeoutSeconds != nil {
				kubernetesContainer.ReadinessProbe.TimeoutSeconds = *container.ReadinessProbe.TimeoutSeconds
			}
			if container.ReadinessProbe.PeriodSeconds != nil {
				kubernetesContainer.ReadinessProbe.PeriodSeconds = *container.ReadinessProbe.PeriodSeconds
			}
			if container.ReadinessProbe.SuccessThreshold != nil {
				kubernetesContainer.ReadinessProbe.SuccessThreshold = *container.ReadinessProbe.SuccessThreshold
			}
			if container.ReadinessProbe.FailureThreshold != nil {
				kubernetesContainer.ReadinessProbe.FailureThreshold = *container.ReadinessProbe.FailureThreshold
			}

			// NOTE(hasheddan): Kubernetes specifies that only one type of
			// handler should be provided. OAM does not impose that same
			// restriction. We optimistically check all and set whatever is
			// provided.
			if container.ReadinessProbe.HTTPGet != nil {
				kubernetesContainer.ReadinessProbe.Handler.HTTPGet = &corev1.HTTPGetAction{
					Path: container.ReadinessProbe.HTTPGet.Path,
					Port: intstr.IntOrString{IntVal: container.ReadinessProbe.HTTPGet.Port},
				}

				for _, h := range container.ReadinessProbe.HTTPGet.HTTPHeaders {
					kubernetesContainer.ReadinessProbe.Handler.HTTPGet.HTTPHeaders = append(kubernetesContainer.ReadinessProbe.Handler.HTTPGet.HTTPHeaders, corev1.HTTPHeader{
						Name:  h.Name,
						Value: h.Value,
					})
				}
			}
			if container.ReadinessProbe.Exec != nil {
				kubernetesContainer.ReadinessProbe.Exec = &corev1.ExecAction{
					Command: container.ReadinessProbe.Exec.Command,
				}
			}
			if container.ReadinessProbe.TCPSocket != nil {
				kubernetesContainer.ReadinessProbe.TCPSocket = &corev1.TCPSocketAction{
					Port: intstr.IntOrString{IntVal: container.ReadinessProbe.TCPSocket.Port},
				}
			}
		}

		d.Spec.Template.Spec.Containers = append(d.Spec.Template.Spec.Containers, kubernetesContainer)
	}

	return []oam.Object{d}, nil
}

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
					labelKey: string(w.GetUID()),
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
