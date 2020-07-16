# OAM Kubernetes Runtime

[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/mod/github.com/crossplane/oam-kubernetes-runtime)

The OAM Kubernetes runtime project is a collection of OAM core controllers and Golang helper
libraries. It is designed to help OAM platform builders to quickly build an end-user facing OAM
platform instead of starting from scratch. Application developers or end-users are not
recommended to build or run their applications using this repo directly. 

We created this repo with the following goals in mind  
* All OAM Kubernetes platform builders use and contribute to this repo. 
* The wide adoption of this repo can facilitate workload/trait interoperability among OAM
 Kubernetes platform builders.


## Prerequisites

- Kubernetes v1.16+
- Helm 3

## Install OAM runtime

#### Clone this repo

```console
git clone git@github.com:crossplane/oam-kubernetes-runtime.git
cd ./oam-kubernetes-runtime
```

#### Install OAM controllers

```console
kubectl create namespace oam-system
helm install core-runtime -n oam-system ./charts/oam-kubernetes-runtime
```

## Verify

* Apply a sample application configuration

```console
kubectl apply -f examples/containerized-workload
```

* Verify that the application is running
You can check the status and events from the applicationConfiguration object   
```console
kubectl describe applicationconfigurations.core.oam.dev example-appconfig
Status:
  Conditions:
    Last Transition Time:  2020-06-12T21:18:40Z
    Reason:                Successfully reconciled resource
    Status:                True
    Type:                  Synced
  Workloads:
    Component Name:  example-component
    Traits:
      Trait Ref:
        API Version:  core.oam.dev/v1alpha2
        Kind:         ManualScalerTrait
        Name:         example-appconfig-trait
    Workload Ref:
      API Version:  core.oam.dev/v1alpha2
      Kind:         ContainerizedWorkload
      Name:         example-appconfig-workload
Events:
  Type    Reason                 Age              From                                       Message
  ----    ------                 ----             ----                                       -------
  Normal  RenderedComponents     6s (x2 over 7s)  oam/applicationconfiguration.core.oam.dev  Successfully rendered components
  Normal  AppliedComponents      6s (x2 over 6s)  oam/applicationconfiguration.core.oam.dev  Successfully applied components
  Normal  Deployment created     6s (x3 over 6s)  ContainerizedWorkload                      Workload `example-appconfig-workload` successfully server side patched a deployment `example-appconfig-workload`
  Normal  Service created        6s (x3 over 6s)  ContainerizedWorkload                      Workload `example-appconfig-workload` successfully server side patched a service `example-appconfig-workload`
  Normal  Manual scalar applied  6s (x2 over 6s)  ManualScalarTrait                          Trait `example-appconfig-trait` successfully scaled a resouce to 3 instances

```

You should also see a deployment looking like below
```console
kubectl get deployments
NAME                                    READY   UP-TO-DATE   AVAILABLE   AGE
example-appconfig-workload-deployment   3/3   3           3              28s
```

And a service looking like below
```console
kubectl get services
AME                                             TYPE       CLUSTER-IP     EXTERNAL-IP   PORT(S)    AGE
example-appconfig-workload-deployment-service   NodePort   10.96.78.215   <none>        8080/TCP   28s
```

## Cleanup
```console
helm uninstall core-runtime -n oam-system
kubectl delete -f examples/containerized-workload
kubectl delete namespace oam-system --wait
```

## Community, discussion, contribution
You can reach the maintainers of this project at:
* Slack channel: [crossplane#oam](https://crossplane.slack.com/#oam)

## Want to help?
Check out [DEVELOPMENT.md](./DEVELOPMENT.md) to see how to develop with OAM Kubernetes runtime


## Licenses
The OAM Kubernetes runtime is released under the [Apache 2.0 license](LICENSE).
