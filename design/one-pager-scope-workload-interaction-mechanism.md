# Scopes and workloads interaction mechanism in OAM

* Owner: Ryan Zhang (@ryanzhang-oss)
* Status: Implemented

This proposal stays in line with [Traits and workloads interaction mechanism in OAM](https://github.com/crossplane/oam-kubernetes-runtime/blob/master/design/one-pager-trait-workload-interaction-mechanism.md) and enables scopes a similar mechanism with how traits interact with workloads.

## Background
> Application scopes are used to group components together into logical applications by providing different forms of application boundaries with common group behaviors.

It's a crucial step for OAM implementation to make Scope aware of which workloads are associated to it. Currently, OAM implementation assumes that every Scope has a field `.spec.workloadRefs` to record associated workload references and get/set references in a hard-code way. It's in need of a more generic mechanism to do that.
 
We will use the following hypothetical OAM application as the baseline to illustrate the problem and our solution. 

`HealthScope` CRD refers to [here](https://github.com/crossplane/oam-kubernetes-runtime/blob/master/charts/oam-kubernetes-runtime/crds/core.oam.dev_healthscopes.yaml).
 
```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: example-workload.oam.dev
spec:
  definitionRef:
    name: example-workload.oam.dev
---
apiVersion: core.oam.dev/v1alpha2
kind: ScopeDefinition
metadata:
  name: healthscope.core.oam.dev
spec:
  allowComponentOverlap: true
  definitionRef:
    name: healthscope.core.oam.dev
---
apiVersion: core.oam.dev/v1alpha2
kind: HealthScope
metadata:
  name: example-health-scope
---
apiVersion: core.oam.dev/v1alpha2
kind: Component
metadata:
  name: example-component
spec:
  workload:
    apiVersion: oam.dev/v1alpha2
    kind: example-workload
    metadata: 
      name: my-example-workload
---
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  components:
    - componentName: example-component      
      scopes:
        - scopeRef:
            apiVersion: core.oam.dev/v1alpha2
            kind: HealthScope
            name: example-health-scope
```

The problem is two folds
1. A scope controller needs a way to find the workloads that it's going to control. 
   - In the example, HealthScope should know that it is supposed to check the health condition of `my-example-workload`. 
However, we want to keep the applicationConfiguration controller agnostic to the schema of any `scope` it interacts with or `workload` it generates to make it extensible. 
Thus, the applicationConfiguration controller needs to get the HealthScope CR (because scope is a kind of global resource) and give it a reference of the `my-example-workload` workload **without knowing HealthScope's specific schema**.
    
2. A scope controller needs to know the exact resources it will control. Note that these
resources are most likely not the workload itself.
    - Use the same example, just knowing the `my-example-workload` workload may be not enough for the `HealthScope` to work. 
The scope controller does not work with the `my-example-workload` workload directly. 
It needs to find the actual Kubernetes resources that the `my-example-workload` workload generates and then it can check the health condition of real targets.

## Goals
In order to maximize the extensibility of our OAM implementation, our solution need to meet the
 following two design objections.
1. **Extensible scope system**: We want to allow a `scope` to apply to any eligible `workload`
instead of just a list of specific ones. This means that we want to empower a scope developer to write the controller code once, and it will work for any new `workload` that this `scope` can interact with in the future.
    - Using the example again, the `HealthScope` should work with any workload that generates a Kubernetes resource that has a `.status.readyReplicas` field. 
2. **Adopting existing CRDs**: The mechanism cannot put any limit on the `scope` or `workload` CRDs. It means that we cannot assume any pre-defined CRD fields in any `scope` or `workload` beyond Kubernetes conventions (i.e. spec or status).
    - For example, the following `DemoScope` is a hypothetical operator which is possible to be used as a `scope` in an OAM application to control `DemoTarget` workloads. 
Here, the `TargetEndpoints` field referring to which `workload` it applies, and we need to accommodate this type of `scope`. 
But currently we can't, because OAM implementation assumes that every eligible `scope` has `.spec.workloadRefs` field, however, this hypothetical scope does not have it.  
      ```yaml
      apiVersion: "demo.oam.dev/v1beta2"
      kind: "DemoScope"
      metadata:
        name: demo-scope
      spec:
        TargetEndpoints: [<a.b.c>]
      ```

## Proposal
The overall idea is for the applicationConfiguration controller to fill critical information in the workload and scope CRs. 
In addition, we will provide a helper library so that scope controller developers can locate the resources they need with a simple function call. 
Here is the list of changes that we propose.
1. Add an optional field called `workloadRefPath` to the `scopeDefinition` schema. This is for the scope owner to declare that the scope relies on the OAM scope/workload interaction mechanism. 
The value of the field is the path to the field that takes `workloadRef` objects. 
In our example, the scope definition would look like below since our `HealthScope` takes the `workloadRef` field at `spec.workloadRefs`.
     ```yaml
       apiVersion: core.oam.dev/v1alpha2
       kind: ScopeDefinition
       metadata:
         name: healthscopes.core.oam.dev
       spec:
         workloadRefPath: spec.workloadRefs
         definitionRef:
           name: healthscopes.core.oam.dev
     ```
2. ApplicationConfig controller no longer assumes that all Scope CRDs contain a `.spec.workloadRefs` field conforming to the OAM definition. It only fills the workload GVK to a Scope CR with `spec.workloadRefs` field defined as below if the corresponding `scopeDefiniton` has a `spec.workloadRefs` field.   
     ```yaml
       workloadRef:
         properties:
           apiVersion:
             type: string
           kind:
             type: string
           name:
             type: string
         required:
         - apiVersion
         - kind
         - name
         type: object
     ```
     
3. Add a `childResourceKinds` field in the  WorkloadDefinition. 

4. OAM runtime will provide a helper library. The library follows the following logic to help a scope developer locate the resources for the scope to control.

> Detail about changes of WorkloadDefinition and Helper Library refers to [Traits and workloads interaction mechanism in OAM](https://github.com/crossplane/oam-kubernetes-runtime/blob/master/design/one-pager-trait-workload-interaction-mechanism.md)

## Impact to the existing system
Here are the impacts of this mechanism to the existing OAM components
- ApplicationConfiguration: This mechanism requires minimum changes in the
 applicationConfiguration controller except that it now needs to check if a `scopeDefiniton` has a `spec.workloadRefsPath` before patching the workloadRef field.
- Scope & ScopeDefinitiion: This mechanism is optional so all existing scope controller still works. 
But for those scopes with `spec.workloadRefs`, their owner should add `spec.workloadRefsPath: spec.workloadRefs` in corresponding `scopeDefiniton`.  
