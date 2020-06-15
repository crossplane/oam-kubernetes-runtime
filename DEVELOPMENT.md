# Development

This doc explains how to set up a development environment, so you can get started
contributing to `oam-kubernetes-runtime` or build a PoC (Proof of Concept). 

## Prerequisites

1. Golang version 1.12+
2. Kubernetes version v1.15+ with kubectl configured

## Build

The functionality of this library can be demonstrated with the following steps:

* Clone this project
```shell script
git clone git@github.com:crossplane/oam-kubernetes-runtime.git
```

* Build the library 

```shell
make submodules 

make
```

## Make your change
* Generate and install CRDs to your Kubernetes cluster

```shell
make generate

kubectl apply -f crds/
```

* Make your code changes

Make changes and run the following to test it.

If you only change code under [core apis](./apis/core), remember to
regenerate crds mainifests as the step above.

## Run a simple and basic workflow locally
You can start running OAM Kubernetes runtime to verify your chanes.
* Run OAM sample controller
```
go run examples/containerized-workload/main.go
```

* Apply the sample application configurations or other manifests 
which you created for your code change verification

```
kubectl apply -f examples/containerized-workload/ 
```

* Verify that corresponding CRs are emitted. 

You should see a `ContainerizedWorkload` looking like below
```
kubectl get containerizedworkloads.core.oam.dev  
NAME                         AGE
example-appconfig-workload   12s
```

And a `Manualscalertrait` looking like below
```
kubectl get manualscalertraits.core.oam.dev
NAME                      AGE
example-appconfig-trait   54s
```

## Prepare for Pull Request

Before make a PR, please:

1) write respective unit-test
2) run `gofmt -s -w CHANGED_FILE` to format changed Go files
3) run `make test` to run unit-tests

## Clean up

Delete all crds by `kubectl delete -f crds/` and other rouses you created.
