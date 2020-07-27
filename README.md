# OAM Kubernetes Runtime

[![License](https://img.shields.io/github/license/crossplane/oam-kubernetes-runtime?style=flat-square)](https://img.shields.io/github/license/crossplane/oam-kubernetes-runtime?style=flat-square)
[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/mod/github.com/crossplane/oam-kubernetes-runtime)
[![docker pulls](https://img.shields.io/docker/pulls/crossplane/oam-kubernetes-runtime?style=flat-square)](https://img.shields.io/docker/pulls/crossplane/oam-kubernetes-runtime?style=flat-square)

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

## Install OAM Runtime

1. Create namespace for OAM runtime controller

```shell script
kubectl create namespace oam-system
```

2. Add helm repo

```console
helm repo add crossplane-master https://charts.crossplane.io/master/
```

3. Install OAM Runtime Controller

You can directly install it without webhook by:

```
helm install oam --namespace oam-system crossplane-master/oam-kubernetes-runtime --devel
```

Or you can install with webhook enabled by following steps:

  - Step 1: Admission Webhook need you to prepare certificates and ca for production use.
    **For none-production use**, you could generate them by running the shell script provided in repo.
    ```shell script
    curl -sfL https://raw.githubusercontent.com/crossplane/oam-kubernetes-runtime/master/hack/ssl/ssl.sh | bash -s oam-kubernetes-runtime-webhook oam-system
    ```

    The shell will generate files like below:

    ```console
    $ tree
    .
    ├── csr.conf
    ├── oam-kubernetes-runtime-webhook.csr
    ├── oam-kubernetes-runtime-webhook.key
    └── oam-kubernetes-runtime-webhook.pem
    
    0 directories, 4 files
    ```

  - Step 2: Create secret for ssl certificates:
    * Notice the server key and certificate must be named tls.key and tls.crt, respectively.
    * Secret name can be user defined, we'd better align with chart values.

    ```shell script
    kubectl -n oam-system create secret generic webhook-server-cert --from-file=tls.key=./oam-kubernetes-runtime-webhook.key --from-file=tls.crt=./oam-kubernetes-runtime-webhook.pem
    ```

  - Step 3: Get CA Bundle info and install with it's value

    ```shell script
    caValue=`kubectl config view --raw --minify --flatten -o jsonpath='{.clusters[].cluster.certificate-authority-data}'`
    helm install core-runtime -n oam-system ./charts/oam-kubernetes-runtime --set useWebhook=true --set certificate.caBundle=$caValue 
    ```

## Get started

* We have some examples in our repo, clone and get started with it.

```shell script
git clone git@github.com:crossplane/oam-kubernetes-runtime.git	
cd ./oam-kubernetes-runtime	
```

* Apply a sample application configuration

```shell script
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
