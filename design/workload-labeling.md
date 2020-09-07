# Workload Labeling

* Owner: Jianbo Sun (@wonderflow), Zheng Xi Zhou (@zzxwill)
* Reviewers: Crossplane Maintainers
* Status: Draft

## Background

Per [issue #193](https://github.com/crossplane/oam-kubernetes-runtime/issues/193), there are several sceneries which currently don't have elegant solutions.

1). In ApplicationConfiguration `bookinfo`, OAM users have to specify the pod label in `spec.labels` field of the service trait `my-service`, while OAM Kubernetes
Runtime exactly knows how to set `selector` by the workload. 

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: bookinfo
spec:
  components:
    - componentName: details
      traits:
        - trait:
            apiVersion: v1
            kind: Service
            metadata:
              name: details
            spec:
              labels:
                app: details
```

2). In ApplicationConfiguration `appconfig-example`, OAM users have to copy GVK and name of the workload to `spec.scaleTargetRef`
filed， from where is clearly specified in Component. Again, the runtime exactly knows how to set `scaleTargetRef`.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: appconfig-example
spec:
  components:
    - componentName: php-apache
      traits:
        - trait:
            apiVersion: autoscaling/v2beta2
            kind: HorizontalPodAutoscaler
            metadata:
              name: php-apache
            spec:
              scaleTargetRef:
                apiVersion: apps/v1
                kind: Deployment
                name: php-apache
---
apiVersion: core.oam.dev/v1alpha2
kind: Component
metadata:
  name: php-apache
spec:
  workload:
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: php-apache
```

3). Here is a similar situation in which users have to manually set `serviceName` and `servicePort` in trait Ingress.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example
spec:
  components:
    - componentName: web
      traits:
        - trait:
            apiVersion: v1
            kind: Service
            metadata:
              name: web
            spec:
              selector:
                app: web
              ports:
              - port: 80
        - trait:
            apiVersion: extensions/v1beta1
            kind: Ingress
            spec:
              rules:
              - http:
                  paths:
                  - path: /
                    backend:
                      serviceName: web
                      servicePort: 80
```

4). If a trait can easily retrieve the underlying pods, it will be helpful for:
- ingress/service/traffic trait can easily route their traffic to pods with the information of OAM AppConfig/Component.
- log/metrics trait can easily find which pods need to gather logs or metrics by OAM AppConfig.
At present, users have to manually set `spec.template.metadata.label` in workload which can help trait find the targeted
pods.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Component
metadata:
  name: example-deployment
spec:
  workload:
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: nginx-deployment
      labels:
        app: nginx
    spec:
      selector:
        matchLabels:
          app: nginx
      template:
        metadata:
          labels:
            component.oam.dev/name: example-deployment
            app: nginx
        spec:
          containers:
          - name: nginx
            image: nginx:1.17
            ports:
            - containerPort: 80
```

In all, to achieve various goals, OAM users have to repeatedly specify information to manually establish connections
between pods, workloads and traits.

## Goals

The goal is to recommend a simple way - Workload Labeling, to easily establish connections between Workloads, Components, AppConfigurations,
and pods.

## Proposal

Step 1: Setting the label `definition.oam.dev/unique-pod` as the unique identifier of pods

We found that if our final target workload resource is pod, we can always assume the pod has unique labels:

- For pods created by K8s Deployment, the `pod-template-hash` label will be automatically added.

```yaml
apiVersion: v1
kind: Pod
metadata:
  labels:
    pod-template-hash: 748f857667
```

- For pods created by K8s DaemonSet, StatefulSet, [OpenKruise/Cloneset](https://openkruise.io/en-us/docs/cloneset.html), `controller-revision-hash` label will be generated.
```yaml
apiVersion: v1
kind: Pod
metadata:
  labels:
    controller-revision-hash: b8d6c88f6
```

Declare `definition.oam.dev/unique-pod` in WorkloadDefinition as OAM specified labels, which will always exist in pods.
Then trait automatically fills correlation fields by detecting the unique pod label, for example, generating service, split traffic, etc.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: xxx
  labels:
    definition.oam.dev/unique-pod: pod-template-hash # optional but recommended, values in [pod-template-hash, controller-revision-hash]
spec:
  ...
```

Step 2: Utilizing generated workload labels to establish clear relationship between workloads and its component

OAM Kubernetes Runtime now has the ability of [automatically generating these labels for every workload](https://github.com/crossplane/oam-kubernetes-runtime/pull/189) as below.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    deployment.kubernetes.io/revision: "1"
  labels:
    app: nginx
    app.oam.dev/component: label-component
    app.oam.dev/name: label-appconfig
    app.oam.dev/revision: label-component-v1
    definition.oam.dev/unique-pod: pod-template-hash
```

Label `app.oam.dev/component` marks the Component, `app.oam.dev/name` marks the ApplicationConfiguration, and `app.oam.dev/revision`
marks the revision of the component.

Currently, a trait can find the attached workload by [`spec.workloadRef`](./one-pager-trait-scope-workload-interaction-mechanism.md) field of the Trait. With those labels above, a
trait can quickly find the corresponding component and its revision.

```yaml
spec:
  workloadRef:
    apiVersion: apps/v1
    kind: Deployment
    name: label-nginx
```

Moreover, with recommended label `definition.oam.dev/unique-pod` in Component manifest set, we can see it is
generated as one label of the Deployment workload, which can help identify the pods.

## Best practice

- For OAM users

We recommend you to set label `definition.oam.dev/unique-pod` in WorkloadDefinition and Component. 

We do NOT recommend you to use Ingress/Service, HPA trait at all. Though based on the mechanism of this proposal,
we can automatically set the required, we encourage users to use official Route and Scale trait.

- For Trait builder

We recommend you to utilize those Workload labels to help identify the Components, ApplicationConfiguration and Pods, to
help implement operation logic.

## Impact to existing system

As the best practice, the proposal won't affect the existing system.
