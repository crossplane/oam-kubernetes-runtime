# Trait Work as Patch of Workload

- Owner: Shibo Li (@ifree613), Jianbo Sun (@wonderflow)
- Date: 07/18/2020
- Status: Draft

## Background

OAM encourages separate of concern for developers and operators. So we use abstract workload like ContainerizedWorkload
to hide details for K8s built-in object like Deployment. We separate Deployment into one `ContainerizedWorkload` and
several traits in implementation, both of them have CRDs and controllers.

Everything goes well until we change traits which will affect fields in pod template of Deployment, that will trigger
re-create of pods. Trait will take effect asynchronously after workload created. In another world, a deployment will be
created first, after that trait will update the deployment to take effect. Updating pod template of deployment leads to
re-create of pods, which finally makes the Application unstable. 
 
For example, if we currently have an OAM app running like below:

It just contains a component with ContainerizedWorkload, and a canary trait.
 
```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Component
metadata:
  name: example-component
spec:
  workload:
    apiVersion: core.oam.dev/v1alpha2
    kind: ContainerizedWorkload
    spec:
      containers:
        - name: wordpress
          image: wordpress:4.6.1-apache
          ports:
            - containerPort: 80
              name: wordpress
          env:
            - name: TEST_ENV
              value: test
---
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  - componentName: example-component
      traits:
        - trait:
            apiVersion: extended.oam.dev/v1alpha2
            kind: Canary
            spec:
              canaryNumber: 1
```

The real running workload is like below:

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: wordpress
          image: wordpress:4.6.1-apache
          ports:
            - containerPort: 80
              name: wordpress
          env:
            - name: TEST_ENV
              value: test
```

After some time, the App operator want to add two traits on this App for some security reason.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  components:
    - componentName: example-component
      traits:
        - trait:
            apiVersion: extended.oam.dev/v1alpha2
            kind: Canary
            spec:
              canaryNumber: 1
        - trait:
            apiVersion: extended.oam.dev/v1alpha2
            kind: NodeSelector
            spec:
              disktype: ssd
        - trait:
            apiVersion: extended.oam.dev/v1alpha2
            kind: SecurityContext
            spec:
              runAsNonRoot: true
```

After the change of AppConfig deployed, NodeSelector and SecurityContext trait will work in a random order. Assuming the controller of NodeSelector trait
will work first, then it will patch and update the Deployment.

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      nodeSelector:
        disktype: ssd
      containers:
        - name: wordpress
          image: wordpress:4.6.1-apache
          ports:
            - containerPort: 80
              name: wordpress
          env:
            - name: TEST_ENV
              value: test
``` 

This will cause first round re-create of pods, it won't trigger the canary trait to work, and the App will become unstable.

Before the deployment become stable, the controller of SecurityContext trait will work and update the deployment again.

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
      nodeSelector:
        disktype: ssd
      containers:
        - name: wordpress
          image: wordpress:4.6.1-apache
          ports:
            - containerPort: 80
              name: wordpress
          env:
            - name: TEST_ENV
              value: test
``` 

This will cause a second round of re-create. Again, the canary trait won't work.

There're no doubt too many risks here in this workflow. We can conclude into two problems:

1. Pods can't be re-created several times, all changes of trait in one deploy can only affect pod once.
2. Changes of trait which will affect the underlying workload must trigger new revision and make the canary trait work.

In this proposal, we're going to fix these two probelms.

## Goals

1. make sure all changes of workload and trait only will affect underlying workload once.
2. make sure underlying workload change will trigger revision create and trigger rollout/canary trait to work.

## Proposal

The overall idea of this proposal is to merge all traits and patch on the workload only once, and this action will
trigger component revision generated automatically.

To achieve this, we propose: 

1. Add `patch` field into TraitDefinition, this will make the trait become a kind of patch trait.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: nodeselectors.extended.oam.dev
spec:
  patch: true 
  appliesToWorkloads:
    - core.oam.dev/v1alpha2.ContainerizedWorkload
  definitionRef:
    name: nodeselectors.extended.oam.dev
```

2. All patch traits MUST be created before workload emit, and oam-k8s-runtime will be pending until all patch traits are
ready.

```yaml
apiVersion: extended.oam.dev/v1alpha2
kind: NodeSelector
metadata:
  name: nodeselector-demo
spec:
  disktype: ssd
```

3. Controller of patch trait can reconcile by their own logic, but MUST contain `patchConfig` and `phase` fields in
its status. The value of `patchConfig` field is a name of ConfigMap, while the value of `phase` will tell oam-k8s-runtime
whether the patch trait is ready. 

```yaml
apiVersion: extended.oam.dev/v1alpha2
kind: NodeSelector
metadata:
  name: nodeselector-demo
spec:
  disktype: ssd
status:
  patchConfig: nodeselector-cm
  phase: Ready
```

4. The ConfigMap will contain this json-patch information in its `patch` field of data. 

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: nodeselector-cm
data:
  patch: |
    {
      "spec": {
        "template": {
          "spec": {
            "nodeSelector": {
              "disktype": "ssd"
            }
          }
        }
      }
    }
```

5. oam-k8s-runtime will periodically check the status of patch trait. If the `phase` of status become `Ready`, the
oam-k8s-runtime will gather the patch data from the ConfigMap. After all patch traits gathered, oam-k8s-runtime will
patch all the information into workload.
 
6. All changes of patch trait will trigger creation of component revision once, and update the status of component,
make the latestRevision to be the newly created one. The rollout/canary trait will work after that.

## Limitations

1. All patch trait will naturally become dependency of workload.
2. Operators of all patch trait MUST be written following our guide. (TODO(wonderflow): in the near future, we should
give some mechanism to generate these traits operators automatically)

## Impact to existing system

1. Patch trait added, along with some guides to write these kinds of controllers.
2. Changes of trait will generate component revisions and trigger rollout automatically.
This action MUST reflect in UI dashboard to let the developer understand.
There is also an alternative solution on it. We can add a policy to component which can indicate whether changes of trait
should take effect at once. If the policy is no, we can make all changes take effect until next time real component changes happen.
But this solution not recommended, because mix changes in one deploy will take extra risks.