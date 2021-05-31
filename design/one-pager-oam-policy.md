# Introduce workload policy in OAM

* Owner: Ryan Zhang (@ryanzhang-oss)
* Reviewers: Crossplane Maintainers
* Status: Draft

## Terminology

* **CRD (Custom Resource Definition)** : A standard Kubernetes Custom Resource Definition
* **CR (Custom Resource)** : An instance of a Kubernetes type that was defined using a CRD
* **GVK (Group Version Kind)** : The API Group, Version, and Kind for a type of Kubernetes
 resource (including CRDs)

## Background
We replaced `workloadType` with `workloadDefinition` in the OAM  
[v1alpha2](https://github.com/oam-dev/spec/releases/tag/v1.0.0-alpha.2) specification. With the
introduction of `workloadDefinition`, an OAM user can now import any type of Kubernetes
resources to an OAM platform. However, a `workloadDefinition` currently only
contains the schematic part of the workload. It does not contain any information regarding
the runtime characteristics of the workload. Similarly, a `component` is also focused on the
 specification details of a workload. The absence of runtime characteristics description in the
  `workload` part of OAM system creates some problems.
 
Before we dive deep into our proposal, here is a hypothetical OAM application that we will use as 
the baseline to illustrate the problems and our solution.
 
```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: containerizedworkloads.core.oam.dev
spec:
  definitionRef:
    name: containerizedworkloads.core.oam.dev
  childResourceKinds:
    - apiVersion: apps/v1
      kind: Deployment
---
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: manualscalertraits.core.oam.dev
spec:
  definitionRef:
    name: manualscalertraits.core.oam.dev
---
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: ingresstraits.core.oam.dev
spec:
  definitionRef:
    name: ingresstraits.core.oam.dev
---
apiVersion: core.oam.dev/v1alpha2
kind: Component
metadata:
  name: example-db
spec:
  workload:
     apiVersion: core.oam.dev/v1alpha2
     kind: ContainerizedWorkload
     metadata:
       name: mydb-example
     spec:
       containers:
         - name: mysql
           image: mysql:latest
---
apiVersion: core.oam.dev/v1alpha2
kind: Component
metadata:
  name: example-web
spec:
  workload:
     apiVersion: core.oam.dev/v1alpha2
     kind: ContainerizedWorkload
     metadata:
       name: myweb-example
     spec:
       containers:
         - name: wordpress
           image: wordpress:latest
```
The problem is two folds
1. Developers may need to express opinions on how to run the workload. We currently do not have a
 way to express that.
   - For example,  let's say that the `example-db` component developer wants to deploy the
    database component in a master only mode. Therefore, an application operator cannot apply a
    `ManualScalerTrait` to the `example-db` component. In the same time, let's say that the
    `example-web` component developer wants to expose the component to the internet. This
     means that an application operator needs to know that it should apply an `IngressTrait' to
     this component.  
    
2. OAM runtime needs an extensible way for a `trait` to apply to groups of `workload`.
    - `TraitDefinition` in the 
    [current spec](https://github.com/oam-dev/spec/blob/master/6.traits.md#spec) 
    has a `appliesToWorkloads` field. This field works for traits that only applies to some
    specific workloads. A `EtcdBackup` trait is a good example. This field is not ideal for a
    more general trait such as `ManualScalerTrait`, `IngressTrait`, or `RolloutTrait`. We cannot
    put every workload that needs a high availability configuration in the `appliesToWorkloads
    ` field in a `ManualScalerTrait`. 

## Goals
In order to maximize the extensibility of our OAM implementation, our solution need to meet the
 following two design objections.
1. **Allow developers to express opinions on how to operate the workload**.
2. **Allow a trait to apply to a category of workloads**.

## Proposal
The overall idea has two parts. 
### A `policies` field in the Component 
The first part of the proposal is to add a new optional `policies` field in the OAM `Component
` object's spec. The component CRD will have an extra item in the spec like below: 
 ```yaml
   policies:
     items:
       properties:
         type:
           type: string
         value:
           anyOf:
           - type: integer
           - type: string
           x-kubernetes-int-or-string: true
       required:
       - type
       - value
       type: object
     type: array
 ```
OAM will define a set of policy types and each policy, in general, also has a finite number of
 possible values.

For example, we can define a `maxInstancePolicy` to indicate the max numbers of instance to
 which a `workload` can scale up. The `example-db` component could use the `maxInstancePolicy
 ` with value 1 to express that the component cannot scale beyond one instance.
```yaml

apiVersion: core.oam.dev/v1alpha2
kind: Component
metadata:
  name: example-db
spec:
  workload:
     apiVersion: core.oam.dev/v1alpha2
     kind: ContainerizedWorkload
     metadata:
       name: mydb-example
     spec:
       containers:
         - name: mysql
           image: mysql:latest
  policies
    - type: maxInstancePolicy
      value: 1
```
Similarly, we can define an `accessibiltyPolicy` to indicate how does a workload want to be
 accessed. The possible values for this policy can be `internet`, `cluster` and `reject`. 
The `example-web` component could use the `accessibiltyPolicy` with value `internet` to express that
the component should be accessible from outside of the cluster.
```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Component
metadata:
  name: example-web
spec:
  workload:
     apiVersion: core.oam.dev/v1alpha2
     kind: ContainerizedWorkload
     metadata:
       name: myweb-example
     spec:
       containers:
         - name: wordpress
           image: wordpress:latest
  policies
      - type: accessibiltyPolicy
        value: internet
```
### An `appliesToPolices` field in the `TraitDefinition`    
The second part is that we will add an `appliesToPolices` field in the `TraitDefinition` to
indicate what type of `workload` the trait can apply to. For the first iteration, we will use
a simple exact match for the type of policies that has a finite set of string values. The optional
 new field has the following schema.
```yaml
   appliesToPolices:
     items:
       properties:
         type:
           type: string
         value:
           anyOf:
           - type: integer
           - type: string
           x-kubernetes-int-or-string: true
       required:
       - type
       - value
       type: object
     type: array
 ```
For example, the ingressTrait can use the `appliesToPolices` field to indicate that it can apply
 to any workload that has the exact same `accessibiltyPolicy`.
```yaml
---
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: ingresstraits.core.oam.dev
spec:
  definitionRef:
    name: ingresstraits.core.oam.dev
  appliesToPolices:
    - type: accessibiltyPolicy
      value: internet
```
The policy can also help in the case of `ManualScalerTraits` although we don't plan to support
complicated logical or mathematical based matching rules in the first iteration. This allows one
 to write an admission webhook to verify that the `Component` to which a `ManualScalerTraits
 ` applies can be scale to the `replicas` number of instance specified if the `Component
 ` contains a `maxInstancePolicy` or a `minInstancePolicy`.


## Impact to the existing system
To summarize this proposal, here are the changes to the existing OAM runtime. This proposal
 introduces two optional `Experimental` fields.
- `policies` field in the `Component` : This field helps the developer to express their opinion
 on how to operate the `workload`. It also helps the OAM runtime to enforce operational rules during
  runtime.
- `appliesToPolices` field in the `TraitDefinition`: This field declares the type of `workload
` that a `trait` applies to. This allows a `trait` to be able to apply to any `workload` that
 contains a policy that matches one of the field in its `appliesToPolices` field.
- OAM admission control: We need to write an OAM admission controller to enforce the semantics of
 the `policies` and `appliesToPolices` fields.

## Alternative approach
1. One alternative approach is that we can define a standalone OAM resources called `policy`.
 This makes our `policy` system a lot more expressive, and we can even support
[OPA](https://www.openpolicyagent.org/) type of rules. The downside of this approach is that it
will make the OAM `policy` system, perhaps overly, complicated. We decide not to go down this
route for our first iteration. We can revisit this approach should we encounter scenarios
that ask for a more powerful policy system.