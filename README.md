# OAM Kubernetes Project

The OAM Kubernetes runtime project is a collection of Golang helper libraries and OAM api type
 definitions. 

It is designed to help OAM platform builders rather than being used directly by developers
or end-users. We would recommend end-users to check
[Crossplane  official  documentation](https://crossplane.github.io/docs) instead.

In addition, we created this library with the following goals in mind  
* All OAM Kubernetes platform builders use and contribute to this library. 
* The wide adoption of this library can facilitate workload/trait interoperability among OAM
 Kubernetes platform builders.
 
## Prerequisites

1. Golang version 1.12+
2. Kubernetes version v1.15+ with kubectl configured

## Getting started

The functionality of this library can be demonstrated with the following steps:

* Fork and Clone this project

* Build the library 

```shell
make submodules 

make
```

* Generate and install CRDs to your Kubernetes cluster

```shell
make generate

kubectl apply -f crds/
```

* Run OAM sample controller
```
go run examples/containerized-workload/main.go
```

* Apply the sample application configurations

```
kubectl apply -f examples/containerized-workload/ 
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


## Community, discussion, contribution
You can reach the maintainers of this project at:
* Slack channel: [crossplane#oam](https://crossplane.slack.com/#oam)

## Licenses
The OAM Kubernetes runtime is released under the [Apache 2.0 license](LICENSE).