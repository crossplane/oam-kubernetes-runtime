# Indicate Workload has PodSpec field

- Owner: Jianbo Sun (@wonderflow)
- Date: 10/09/2020
- Status: Implemented

## Background

Since we have added labels like [`workload.oam.dev/podspecable`](https://github.com/oam-dev/spec/blob/master/4.workload_definitions.md#labels)
into OAM Spec to indicate the workload will contain 'podSpec' in its spec.

In most famous workload like `Deployment`, `StatefulSet`, `Job`, the `podSpec` field will always be `spec.template.spec`. 
But it's hard to know which field is podSpec for K8s CRD. For example, we write a workload named [`PodSpecWorkload`](https://github.com/oam-dev/kubevela/blob/master/charts/vela-core/crds/standard.oam.dev_podspecworkloads.yaml).
It has `podSpec` in `spec.podSpec`, this is different with `spec.template.spec`. In this case, we need a field to indicate
which field is `podSpec`. 
 
## Proposal

So I propose we add a field 'podSpecPath' and a label `workload.oam.dev/podspecable: true` into WorkloadDefinition.
 
The 'podSpecPath' field and the label are both optional fields, they could have following behaviors:

### No label and No `podSpecPath` field

```
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: webservice
spec:
  definitionRef:
    name: podspecworkloads.standard.oam.dev
   ...
```

In this case, we can't do any podSpec assumption for this workload.

### label specified with No `podSpecPath` field

```
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: webservice
  labels:
     workload.oam.dev/podspecable: true
spec:
  definitionRef:
    name: podspecworkloads.standard.oam.dev
   ...
```

In this case, we will always regard `spec.template.spec` as the `podSpecPath`. 
This will work for most famous workloads like K8s `Deployment`/`StatefulSet`/`Job` and many other CRD Objects like
`OpenKruise CloneSet`/`Knative Service`.

### Has `podSpecPath` field 

```
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: webservice
  labels:
     workload.oam.dev/podspecable: true
spec:
  podSpecPath: "spec.podSpec"
  definitionRef:
    name: podspecworkloads.standard.oam.dev
   ...
```

If the `podSpecPath` is specified, the `workload.oam.dev/podspecable: true` label will always be automatically added. 

In this case, trait can use `podSpecPath` to get the field of podSpec.

