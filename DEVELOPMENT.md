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
$ make submodules
Submodule 'build' (https://github.com/upbound/build) registered for path 'build'
Cloning into '/Users/xxx/Programming/golang/src/github.com/crossplane/oam-kubernetes-runtime/build'...
Submodule path 'build': checked out 'e8fb77d69aefc49dd2e9ead59da21bd719cacb78'
```


```
$ make
12:42:21 [ .. ] verify dependencies have expected content
all modules verified
12:42:21 [ OK ] go modules dependencies verified
12:42:21 [ .. ] installing golangci-lint-v1.23.8 darwin-amd64
12:42:28 [ OK ] installing golangci-lint-v1.23.8 darwin-amd64
12:42:28 [ .. ] golangci-lint
12:42:43 [ OK ] golangci-lint
12:42:44 [ .. ] go build linux_amd64
12:42:44 [ OK ] go build linux_amd64
```

## Make your change
* Generate and install CRDs to your Kubernetes cluster

```shell
$ make generate
12:43:01 [ .. ] go generate darwin_amd64
go: downloading sigs.k8s.io/controller-tools v0.2.4
go: extracting sigs.k8s.io/controller-tools v0.2.4
go: downloading github.com/spf13/cobra v0.0.5
go: downloading gopkg.in/yaml.v3 v3.0.0-20190905181640-827449938966
go: downloading github.com/fatih/color v1.7.0
go: downloading golang.org/x/tools v0.0.0-20200325010219-a49f79bcc224
go: downloading github.com/gobuffalo/flect v0.1.5
go: extracting github.com/gobuffalo/flect v0.1.5
go: extracting github.com/fatih/color v1.7.0
go: extracting gopkg.in/yaml.v3 v3.0.0-20190905181640-827449938966
go: downloading github.com/mattn/go-colorable v0.1.2
go: downloading github.com/mattn/go-isatty v0.0.8
go: extracting github.com/spf13/cobra v0.0.5
go: extracting github.com/mattn/go-colorable v0.1.2
go: extracting github.com/mattn/go-isatty v0.0.8
go: extracting golang.org/x/tools v0.0.0-20200325010219-a49f79bcc224
go: finding sigs.k8s.io/controller-tools v0.2.4
go: finding github.com/spf13/cobra v0.0.5
go: finding golang.org/x/tools v0.0.0-20200325010219-a49f79bcc224
go: finding github.com/fatih/color v1.7.0
go: finding github.com/gobuffalo/flect v0.1.5
go: finding gopkg.in/yaml.v3 v3.0.0-20190905181640-827449938966
go: finding github.com/mattn/go-colorable v0.1.2
go: finding github.com/mattn/go-isatty v0.0.8
12:43:08 [ OK ] go generate darwin_amd64
```


```
$ kubectl apply -f crds/
customresourcedefinition.apiextensions.k8s.io/applicationconfigurations.core.oam.dev configured
customresourcedefinition.apiextensions.k8s.io/components.core.oam.dev configured
customresourcedefinition.apiextensions.k8s.io/containerizedworkloads.core.oam.dev configured
customresourcedefinition.apiextensions.k8s.io/manualscalertraits.core.oam.dev configured
customresourcedefinition.apiextensions.k8s.io/scopedefinitions.core.oam.dev configured
customresourcedefinition.apiextensions.k8s.io/traitdefinitions.core.oam.dev configured
customresourcedefinition.apiextensions.k8s.io/workloaddefinitions.core.oam.dev configured
```

* Make your code changes

Make changes and run the following to test it.

If you only change code under [core apis](./apis/core), remember to
regenerate crd manifests as the step above.

## Run a simple and basic workflow locally
You can start running OAM Kubernetes runtime to verify your changes.
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

And a `HealthScope` looking like below
```
kubectl get healthscopes.core.oam.dev
NAME                   AGE
example-health-scope   23s
```

## Prepare for Pull Request

Before make a PR, please:

1) write respective unit-test
2) run `make reviewable` to generate and lint the code.
3) run `make test` to run unit-tests

## Clean up

Delete all crds by `kubectl delete -f crds/` and other resources you created.
