# Versioning mechanism

## Prerequisite

1. Make sure [`Simple Rollout`](https://github.com/oam-dev/catalog/tree/master/traits/simplerollouttrait) was installed for this demo.
2. Make sure [`OAM runtime`](../../README.md#Install-OAM-Kubernetes-Runtime) was installed and started.


## Containing Trait with revisionEnabled and ApplicationConfiguration always using the latest component

After Simple Rollout Trait installed, please make sure you have its trait definition:

```shell script
$ kubectl get traitdefinitions.core.oam.dev
NAME                                         DEFINITION-NAME
simplerollouttraits.extend.oam.dev           simplerollouttraits.extend.oam.dev
```

Simple Rollout is a trait with TraitDefinition as below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: simplerollouttraits.extend.oam.dev
spec:
  revisionEnabled: true
  workloadRefPath: spec.workloadRef
  appliesToWorkloads:
    - core.oam.dev/v1alpha2.ContainerizedWorkload
    - deployments.apps
  definitionRef:
    name: simplerollouttraits.extend.oam.dev
``` 

You can see that the  `revisionEnabled` flag is set to true.

Step 1. Create OAM component

```shell script
$ kubectl apply -f examples/containerized-workload/sample_component.yaml
component.core.oam.dev/example-component created
``` 

The first step is the same with [Component Mutable Demo](./component-mutable.md#ApplicationConfiguration-always-using-the-latest-component)

Step 2. Create AppConfig

In this example, we use SimpleRolloutTrait which has `revisionEnabled` to be true.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig-rollout
spec:
  components:
    - componentName: example-component
      traits:
        - trait:
            apiVersion: extend.oam.dev/v1alpha2
            kind: SimpleRolloutTrait
            spec:
              replica: 6
              maxUnavailable: 2
              batch: 2
```

Assume we name it as `versioning-demo-app.yaml` and apply this AppConfig.

```shell script
$ kubectl apply -f examples/component-versioning/versioning-demo-app.yaml
applicationconfiguration.core.oam.dev/example-appconfig created
```

ContainerizedWorkload was created with **the name of revisionName**.

```shell script
$ kubectl get containerizedworkloads.core.oam.dev
NAME                                     AGE
example-component-brnggdript3e8125vheg   2m18s
```

And the trait works on stable at:

```shell script
$ kubectl get simplerollouttraits.extend.oam.dev
NAME                AGE
example-component-trait-bbc946f94   3m16s
$ kubectl get simplerollouttraits.extend.oam.dev example-component -o yaml
apiVersion: extend.oam.dev/v1alpha2
kind: SimpleRolloutTrait
metadata:
  name: example-component
  ...
spec:
  batch: 2
  maxUnavailable: 2
  replica: 6
  workloadRef:
    apiVersion: core.oam.dev/v1alpha2
    kind: ContainerizedWorkload
    name: example-component-brnggdript3e8125vheg
status:
  currentWorkloadRef:
    apiVersion: core.oam.dev/v1alpha2
    kind: ContainerizedWorkload
    name: example-component-brnggdript3e8125vheg
``` 

Step 3. Update Component

Change the component yaml with the ENV value.

```
$ kubectl edit components example-component
component.core.oam.dev/example-component edited
```

```yaml
     containers:
      - env:
        - name: TEST_ENV
-         value: test
+         value: test2
```

A new ContainerizedWorkload created with the revision name:

```shell script
$ kubectl get containerizedworkloads.core.oam.dev
NAME                                     AGE
example-component-brnggdript3e8125vheg   7m4s
example-component-brngj9ript3e8125vhf0   60s
```

The workloadRef of Simple Rollout trait was updated to the new workload instance name:

```shell script
$ kubectl get simplerollouttraits.extend.oam.dev example-component -o yaml
apiVersion: extend.oam.dev/v1alpha2
kind: SimpleRolloutTrait
metadata:
  name: example-component
  ...
spec:
  batch: 2
  maxUnavailable: 2
  replica: 6
  workloadRef:
    apiVersion: core.oam.dev/v1alpha2
    kind: ContainerizedWorkload
    name: example-component-brngj9ript3e8125vhf0
status:
  currentWorkloadRef:
    apiVersion: core.oam.dev/v1alpha2
    kind: ContainerizedWorkload
    name: example-component-brnggdript3e8125vheg
``` 

It will rollout the instance of ContainerizedWorkload and finally delete the old one.

```shell script
$ kubectl get simplerollouttraits.extend.oam.dev example-component -o yaml
apiVersion: extend.oam.dev/v1alpha2
kind: SimpleRolloutTrait
metadata:
  name: example-component
  ...
spec:
  batch: 2
  maxUnavailable: 2
  replica: 6
  workloadRef:
    apiVersion: core.oam.dev/v1alpha2
    kind: ContainerizedWorkload
    name: example-component-brngj9ript3e8125vhf0
status:
  currentWorkloadRef:
    apiVersion: core.oam.dev/v1alpha2
    kind: ContainerizedWorkload
    name: example-component-brngj9ript3e8125vhf0
``` 

```shell script
$ kubectl get containerizedworkloads.core.oam.dev
NAME                                     AGE
example-component-brngj9ript3e8125vhf0   3m
```

In this workflow, every change of component will trigger a new workload instance created and the old one won't
be deleted. The `revisionEnabled` trait must be responsible for the garbage collection.

## Clean up Policy

You can use flag `-revision-limit` in `oam-kubernetes-runtime` to specify how many old controllerrevisions you want to retain.
By default, it is 50. Cleanup will be triggered after the component is created or updated, it will skip controllerrevision that is
still in use, the rest will be garbage-collected.

## Containing Trait with revisionEnabled and ApplicationConfiguration specify revision manually

This will be almost the same with [the case without revisionEnabled trait](component-mutable.md#ApplicationConfiguration-specify-revision-manually).
The only difference is the workload instance name is revisionName.