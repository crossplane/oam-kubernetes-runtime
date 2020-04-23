# oam-kubernetes-runtime Project

The OAM Kubernetes runtime project is a collection of Golang help libraries and official api type
 definitions for building an OAM Kubernetes runtime that is compatible with the latest stable
  OAM spec. 

## Prerequisites

1. go version 1.12+
2. Kubernetes version v1.15+ with KubeCtl configured 


## Getting started

The functionality of this library can be demonstrated with the following steps:

* Fork and Clone this project

* Build the library 

```shell
make submodules 

make generate

make
```

* Install CRDs to your Kubernetes cluster

```shell
kubectl apply -f cluster/charts/oam-kubernetes-runtime/crds/

```

* Run OAM sample controller
```
go run pkg/examples/containerized-workload/main.go
```

* Apply the sample application config

```
kubectl apply -f pkg/examples/containerized-workload/sample_application_config.yaml
 
```

* Verify that corresponding CRs are emitted. 

You should see a containerizedworkloads looking like below
```
kubectl get containerizedworkloads.core.oam.dev  
NAME                         AGE
example-appconfig-workload   12s
```

And a manualscalertraits looking like below
```
kubectl get manualscalertraits.core.oam.dev
NAME                      AGE
example-appconfig-trait   54s
```

## Goals
We created this library with the following goals in mind  
* We want to make it super easy to build an OAM runtime on Kubernetes
* We hope that all OAM Kubernetes platform builders use and contribute to this library. 
* We hope that we can facilitate workload/trait interoperability based on the universal adoption
 of this library among the OAM platform builders.

 
## Community, discussion, contribution
You can reach the maintainers of this project at:
* Slack channel: [#oam](https://crossplane.slack.com/#oam)
