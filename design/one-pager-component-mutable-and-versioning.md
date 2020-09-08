# Component mutable and versioning mechanism

* Owner: Lei Zhang (@resouer), Jianbo Sun (@wonderflow)
* Reviewers: Crossplane Maintainers
* Status: Implemented
* Updated at 2020-09-04 by @wonderflow

## Terminology

* Application: A holistic term for talking about all pieces that are rendered from an `ApplicationConfiguration`.
* ComponentSchematic: A term in OAM spec v1alpha1, which is deprecated and replaced by `Component` in v1alpha2.

## Background

OAM v1alpha1 requires developers to define another ComponentSchematic with
a different name in order to update it (i.e. ComponentSchematic is immutable). We got feedbacks from
internal OAM adopters in Alibaba as well as [AWS ECS for OAM](https://github.com/awslabs/amazon-ecs-for-open-application-model#upgrade-and-scale-oam-workloads-with-oam-ecs)
that it's not a good experience.  Component is now [mutable](https://github.com/oam-dev/spec/pull/356)  in v1alpha2 spec. Here is how we plan to implement it.

Since a component is mutable, we need to address the issue of component versioning.

One of the requirements that we should allow users to specify a fixed version of the component in an ApplicationConfiguration, so that the component will not be upgraded as soon as its definition changes.

On the other hand, the versioning mechanism can also be used by traits. For example, a trait can specify
a different revision in order to control the upgrade experience. 


## Goals

1. Component is mutable. Developers who modify the Component object will directly trigger immediate
upgrade of any `ApplicationConfiguration` that does not specify `Component` revision. 
2. Component revision can be specified in ApplicationConfiguration. Operator can specify a fixed revision
of Component in ApplicationConfiguration. Component mutation won't affect Application directly in this case, 
and can be upgraded in a more convenient way by working with traits.
3. The versioning mechanism should be compatible with current existing K8s workloads, eg, `K8s Deployment`, `OpenKruise`.
  OAM should support two common ways used in community for versioning mechanism. 
  - Workload itself can handle version. This means we can just update `Deployment` and let the workload do rollout or rollback without traits.
  - Corporate with traits. OAM runtime should create different `Deployment` instances for different revision and let the traits do rollout or rollback.

## Non-Goals

This proposal will not give a specific rollout or rollback design, that can be done by traits.  

## Proposal

### Using ControllerRevision to record versions of Component

Because a `Component` is mutable, we borrow an independent Revision object called [`ControllerRevision`](https://godoc.org/k8s.io/api/apps/v1#ControllerRevision) from the existing Kubernetes resources.
to track a `Component` change automatically.

For example, we have a following component yaml:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Component
metadata:
  name: frontend
spec:
  workload:
    apiVersion: core.oam.dev/v1alpha2
    kind: ContainerizedWorkload
    spec:
      containers:
      - name: my-cool-workload
        image: example/very-cool-workload:0.1.2@sha256:verytrustworthyhash
        cmd:
        - "bash lscpu"
```

When users apply this yaml file into the system.

```shell script
$ kubectl apply -f component.yaml
```

A `ControllerRevision` object will be created.

```shell script
$ kubectl get controllerrevisions
NAMESPACE     NAME                  CONTROLLER               REVISION   AGE
default       frontend-v1    core.oam.dev/component   1          2d15h

$ kubectl get controllerrevisions frontend-c8bb659c5 -o yaml
apiVersion: apps/v1
kind: ControllerRevision
metadata:
  name: frontend-v1 # you could name this anything you wanted, but just appending the semver would be good practice
  labels:
    controller.oam.dev/component: frontend
revision: 1
data:
  workload:
    apiVersion: core.oam.dev/v1alpha2
    kind: ContainerizedWorkload
    spec:
      containers:
      - name: my-cool-workload
        image: example/very-cool-workload:0.1.2@sha256:verytrustworthyhash
        cmd:
        - "bash lscpu"
```

Then `ControllerRevision` will have three parts of information that could be used:

1. labels: a label `controller.oam.dev/component` with the `componentName` as its value is added automatically.
2. revision number: the revision number increases monotonically with each new revision created.
3. data: the data field contains the snapshot information of the component.

And it will be recorded in the status of `component.yaml`.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Component
metadata:
  name: frontend
spec:
  workload:
    apiVersion: core.oam.dev/v1alpha2
    kind: ContainerizedWorkload
    spec:
      containers:
      - name: my-cool-workload
        image: example/very-cool-workload:0.1.2@sha256:verytrustworthyhash
        cmd:
        - "bash lscpu"
status:
  latestRevision: frontend-v1
```


If you make a change to `component.yaml`:

```shell script
           cmd:
-            - "bash lscpu"
+            - "bash top"
```

A new `ControllerRevision` will be automatically generated:

```shell script
$ kubectl get controllerrevisions
NAMESPACE     NAME                  CONTROLLER               REVISION   AGE
default       frontend-v1    core.oam.dev/component   1          2d15h
default       frontend-v2    core.oam.dev/component   2          2d14h

$ kubectl get ControllerRevisions frontend-a75588698 -o yaml
apiVersion: apps/v1
kind: ControllerRevision
metadata:
  name: frontend-v2
  labels:
    controller.oam.dev/component: frontend
revision: 2
spec:
  workload:
    apiVersion: core.oam.dev/v1alpha2
    kind: ContainerizedWorkload
    spec:
      containers:
      - name: my-cool-workload
        image: example/very-cool-workload:0.1.2@sha256:verytrustworthyhash
        cmd:
        - "bash top"
```

Now with `ControllerRevision` in place, let's talk about how to use it in Trait and ApplicationConfiguration.

### ControllerRevision could be referenced by Trait 

Some traits can control version of component which means they can specify a component revision and won't be affected
by the change of component.

For these traits, they have to declare `revisionEnabled` field to be `true` in TraitDefinition like below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: fancytraits.core.oam.dev
spec:
  revisionEnabled: true
  definitionRef:
    name: fancytraits.core.oam.dev
```

Workload instance's name emitted by OAM runtime will be decided by the `revisionEnabled` field.

- If **NO** traits binding with a component has `revisionEnabled` as true, OAM runtime will let the workload itself to handle
   versioning things. We will only have one workload instance with the same name of componentName, and update workload directly
   when component changed.
- If **ANY** trait binding with a component has `revisionEnabled` as true, OAM runtime will always create a new workload with
   a new name which can be aligned with the name of newly created component revision. OAM runtime will leave the trait to
   control existing workloads to do rolling update and garbage collection. 

So the essential difference between true or false of `revisionEnabled` is whether we create or update workload.

---

For example, when we don't use any trait who has revisionEnabled as true, things will work like below:

Firstly, we create an `ApplicationConfiguration` with definition : 

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  components:
    - componentName: frontend
```

The real workload instance will be created with the same name as the componentName.

```yaml
  apiVersion: core.oam.dev/v1alpha2
  kind: ContainerizedWorkload
  metadata:
    name: frontend
  spec:
    containers:
    - name: my-cool-workload
      image: example/very-cool-workload:0.1.2@sha256:verytrustworthyhash
      cmd:
      - "bash lscpu"
```

But if we use any trait with revisionEnabled.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  components:
    - componentName: frontend
      traits:
        - trait:
            apiVersion: core.oam.dev/v1alpha2
            kind: FancyTrait
            spec:
              traffic:
                - revisionName: frontend-v1
                  percent: 80%
                - revision: latest
                  percent: 20%                 
```

The real workload instance will be created with the same name as the RevisionName.

```yaml
  apiVersion: core.oam.dev/v1alpha2
  kind: ContainerizedWorkload
  metadata:
    name: frontend-v1
  spec:
    containers:
    - name: my-cool-workload
      image: example/very-cool-workload:0.1.2@sha256:verytrustworthyhash
      cmd:
      - "bash lscpu"
```

**NOTICE:** In this case, many workload instances of `frontend` are created by the OAM runtime, and can keep running. It's the traits' responsibility to handle the garbage collection of the workload instances it references. In our case here, OAM runtime will leave all revision running, the `FancyTrait` should ensure only the latest along with the other workload whose name is `frontend-v1` is running, and delete the rest. 

### Always Using the latest revision when using componentName field

When we use a Component with componentName in ApplicationConfiguration, OAM runtime will always use the latest
component revision to update/create workload and all corresponding traits. 

For example, below is an AppConfig containing one component with two traits.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: rollouts.core.oam.dev
spec:
  revisionEnabled: true
  definitionRef:
    name: rollouts.core.oam.dev
---
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  components:
    - componentName: frontend
      traits:
        - trait:
            apiVersion: core.oam.dev/v1alpha2
            kind: ManuelScaler
            spec:
              replica: 3
        - trait:
            apiVersion: core.oam.dev/v1alpha2
            kind: Rollout
            spec:
              source:
                revision: 1
```

Once the `frontend` component changed, the `ApplicationConfiguration` controller will trigger
an upgrade for the application.

1. Because the component has a revisionEnabled trait binding with it, so we will aways create new workload instead of update. 
2. Create a workload according to the new component revision. 
3. Update all traits pointing their target workloadRef to the new workload.
4. Rollout trait will do rolling update from `frontend-v1`(revision: 1) to the new one, and finally delete the source workload.


### Using revisionName field can specify a fixed component version

This mechanism usually used in production.

When a `ControllerRevision` is specified in ApplicationConfiguration like below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  components:
    - revisionName: frontend-v1
      traits:
        - trait:
            apiVersion: core.oam.dev/v1alpha2
            kind: ManuelScaler
            spec:
              replica: 3
```

In this case:
- The name of workload instance will be the same with revisionName.
- all traits will point to the specified revision of workload. 
- Component change won't affect the running application, but `ControllerRevision` will still be created.

Old revisions can also work if any revision aware traits pointing to them.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: traffics.core.oam.dev
spec:
  revisionEnabled: true
  definitionRef:
    name: traffics.core.oam.dev
---
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  components:
    - revisionName: frontend-v2
      traits:
        - trait:
            apiVersion: core.oam.dev/v1alpha2
            kind: Traffic
            spec:
              route:
                - revision: frontend-v1 # old revision
                  percent: 60%
                - revision: frontend-v2 # the new one
                  percent: 40%
```

Still, the `Traffic` trait should handle garbage collection things for the old workloads when no longer needed.

## Example: A blue-green workflow

Let's say we have a `Rollout` Trait with definition like below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: rollouts.extend.oam.dev
spec:
  revisionEnabled: true
  definitionRef:
    name: traffics.core.oam.dev
```

It was used in application with a component.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  components:
    - componentName: frontend
      traits:
        - trait:
            apiVersion: extend.oam.dev/v1
            kind: Rollout
            spec:
              replica: 10
              batch: 3
              maxUnavailable: 1
```

Before changing the component, the App will be running in a stable state like below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ContainerizedWorkload
  metadata:
    name: frontend-v1
spec:
  containers:
    - name: my-cool-workload
      image: example/very-cool-workload:0.1.2@sha256:verytrustworthyhash
      cmd:
        - "bash lscpu"
---
apiVersion: extend.oam.dev/v1
kind: Rollout
spec:
  batch: 3
  replica: 10
  maxUnavailable: 1
  workloadRef:
    apiVersion: core.oam.dev/v1alpha2
    kind: ContainerizedWorkload
    name: frontend-v1
```

Also, we will have ControllerRevisions running like below:

```
$ kubectl get controllerrevisions.apps
NAMESPACE     NAME                  CONTROLLER               REVISION   AGE
default       frontend-v1    core.oam.dev/component   1          2d15h
```


When we upgrade `frontend` component, new workload will be created with the new revision created.

```
$ kubectl get controllerrevisions.apps
NAMESPACE     NAME                  CONTROLLER               REVISION   AGE
default       frontend-v1    core.oam.dev/component   1          2d15h
default       frontend-v2    core.oam.dev/component   2          15s
```

The old workload will still be there running as rollout trait has flag `revisionEnabled=true`.


```yaml
# the old one 
apiVersion: core.oam.dev/v1alpha2
kind: ContainerizedWorkload
  metadata:
    name: frontend-v1
spec:
  containers:
    - name: my-cool-workload
      image: example/very-cool-workload:0.1.2@sha256:verytrustworthyhash
      cmd:
        - "bash lscpu"
---
# the new one 
apiVersion: core.oam.dev/v1alpha2
kind: ContainerizedWorkload
metadata:
  name: frontend-v2
spec:
  containers:
    - name: my-cool-workload
      image: example/very-cool-workload:new
      cmd:
        - "bash top"
```

The CR of rollout trait will be automatically updated by OAM runtime:

```yaml
# rollout spec was pointing to the new workload, but status is still refer to the old.
apiVersion: extend.oam.dev/v1
kind: Rollout
spec:
  batch: 3
  replica: 10
  maxUnavailable: 1
  workloadRef:
    apiVersion: core.oam.dev/v1alpha2
    kind: ContainerizedWorkload
    name: frontend-a75588698
```

Then the rollout trait will take control of the blue-green deploy progress.

1. It will find out all the controllerRevisions that belong to the component with the label `controller.oam.dev/component=frontend`.
2. With the controllerRevision, it will find out all existing workload instances.
2. It will start to do a rolling update, decrease the replica of old workloads, and increase the new workload. 

Finally, the old workload will be removed by the rollout trait, getting into another stable state.


## Impact to the existing system

1. Add `revisionName` field into AppConfig and this field is mutually exclusive with `componentName`,
   also `componentName` will be optional if `revisionName` is used.
2. Add `revisionEnabled` flag into TraitDefinition, and it will affect workload name created by OAM runtime.
3. A new `ControllerRevision` is created for a `Component` when its parameters changed. A change in `parameterValue`
   does not affect a `Component` revision. ParameterValue will be configured in ApplicationConfiguration which was supposed
   to be some kind of operational action, so it will be treated as a special kind of trait. The encouraged usage of
   parameterValues is for [parameter passing](https://github.com/crossplane/oam-kubernetes-runtime/pull/24).
   It's an anti-pattern to use parameters to rollout the app (e.g. update image with parameterValue).


## Alternative approach

We do discuss a lot for this proposal, most discussion happened in ["OAM component versioning mechanism
"](https://github.com/oam-dev/spec/issues/336) and ["The Component should mutable"](https://github.com/oam-dev/spec/issues/350).
Refer to these issues to see more details.
