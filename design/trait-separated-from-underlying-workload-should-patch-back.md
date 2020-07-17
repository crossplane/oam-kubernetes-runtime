# Trait separated from underlying workload should patch back before workload emitted

- Owner: Shibo Li (@ifree613), Jianbo Sun (@wonderflow)
- Date: 07/18/2020
- Status: Draft

## Background

A platform tend to provide simper view to developers. For example, in OAM spec, higher level abstractions like
ContainerizedWorkload are defined to hide unnecessary details of Deployment from developers. At the mean time,
with the concept of separate of concerns, the spec abstracted operational related fields from Deployment into 
several traits in implementation, both of them have CRDs and controllers.

In below example, `securityContext` and `nodeSelector` will be separated as traits.

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
    ...
```

However, any changes happened to pod template of Deployment will trigger re-create of pods. 
In our current workflow, traits will take effect asynchronously after workload created. That means
a deployment will be created first without these traits' information, and then being updated several times.
Updating pod template of deployment leads to re-create of pods, which finally makes the Application unstable. 

Let's use a concrete example to make this problem more clear. For example, we currently have an OAM app running
like below. It just contains a component with ContainerizedWorkload, and a canary trait.
 
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

The real running workload is deployment.

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

After some time, the App operator want to add two traits on this App for some security reason, and they are both fields
of underlying Deployment.

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

After AppConfig deployed, NodeSelector and SecurityContext trait will work in a random order.
Assuming the controller of NodeSelector trait will work first, then it will patch and update the underlying Deployment.

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
+     nodeSelector:
+       disktype: ssd
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

This will cause first round re-create of K8s pods, and please notice it won't trigger the canary trait to work.
The App will become unstable at this point.

At the same time, the controller of SecurityContext trait will work and update the deployment again.

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
+     securityContext:
+       runAsNonRoot: true
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

This will cause a second round of pods re-create. Again, the canary trait won't work.

There're no doubt too many risks here in this workflow. We can conclude into two problems:

1. Unstable changes to underlying workload can't happen several times in one deploy, all changes of trait
should only affect underlying workload once.
2. Changes of trait which will affect the underlying workload must trigger new revision and make the canary trait work.

In this proposal, we're going to fix these two problems.

## Goals

1. make sure all changes of workload and trait only will affect underlying workload once.
2. make sure underlying workload change will trigger revision create and trigger rollout/canary trait to work.

## Proposal

The overall idea of this proposal is to merge all traits and patch on the workload only once, and make this action
trigger component revision generated automatically.

To make it more clear, let's define what kind of trait we are talking with first. Let's call them `patch trait`
for convenience. A trait will be `patch trait` only if it's change will **affect fields of underlying workload**
and **MUST trigger new revision to do rollout**. 

Let's see some examples.

   - SecurityContext, NodeSelector and SideCar Trait are all `patch trait` if we are using K8s Deployment to implement.
   Because these traits will all affect fields of underlying workload, and will make pods re-create, these changes
   MUST trigger new revision and do rollout.
   - ManualScaler is NOT a `patch trait`, even though `replica` cloud be field of underlying workload, but change
   replica don't need to do rollout and won't affect the stability of an APP.
   - Ingress, Service Binding Trait are all NOT `patch trait`, it won't affect any field of underlying workload and also don't need to
   do rollout.


Now let's go straight forward to this proposal: 

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

2. All patch traits MUST be created or updated before workload emit, and oam-k8s-runtime will be pending until all patch traits are
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
