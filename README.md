# OAM Kubernetes Runtime

> :tada: We have decided to promote OAM Kubernetes Runtime to an end-to-end app platform engine with the name of [KubeVela](https://github.com/oam-dev/kubevela). Please check its [documentation site](https://kubevela.io) to learn about using OAM (Open Application Model) with Kubernetes in detail.
>
> We made this decision because the growth of this project's feature set and community adoption have fairly exceeded its original scope as "an OAM library" in past 6 months and this made us feel it worth to promote it to a independent project which may even change how the community build developer-centric platforms in the foresee future.
>
> Note that KubeVela is designed to support all features and APIs (i.e. OAM spec v0.2.x releases) of OAM Kubernetes Runtime. So as existing adopters, you could just replace your binary and everything is all set. We decided to avoid directly renaming this repository to KubeVela because there're some other adopters imported this project as a library, we don't want to break their use cases.
>
> Though this also means we are focusing all of our attention on KubeVela repository and will only be working on this repository for critical updates and bug fixes.

The plug-in for implementing Open Application Model (OAM) on Kubernetes. 

## Prerequisites

- Kubernetes v1.16+
- Helm 3

|   OAM Runtime Release    |         Supported Spec Release          |          Comments          |
| :---------------------------- | :--------------------------------: |  :--------------------------------: |
| [Latest release](https://github.com/crossplane/oam-kubernetes-runtime/releases) | [OAM Spec v0.2.1](https://github.com/oam-dev/spec/blob/v0.2.1/SPEC_LATEST_STABLE.md)  | |

## Installation

1. Create namespace for OAM runtime controller

```shell script
kubectl create namespace oam-system
```

2. Add helm repo

```console
helm repo add crossplane-master https://charts.crossplane.io/master/
```

3. Install OAM Kubernetes Runtime


Install with webhook enabled by following steps:

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

  - Step 3: Get CA Bundle info and install with its value

    ```shell script
    caValue=`kubectl config view --raw --minify --flatten -o jsonpath='{.clusters[].cluster.certificate-authority-data}'`
    helm install core-runtime -n oam-system ./charts/oam-kubernetes-runtime --set useWebhook=true --set certificate.caBundle=$caValue 
    ```

For quick developing purpose only:

<details>

You can install this lib without webhook by:

```
helm install oam --namespace oam-system crossplane-master/oam-kubernetes-runtime --devel
```

But be aware that in this case, you will lose critical validations and injections required by OAM control plane. Only do this when you know what you are doing.

</details>


### Verify the Installation

* We have some examples in the repo for you to verify the OAM control plane is working:

  ```shell script
  git clone git@github.com:crossplane/oam-kubernetes-runtime.git	
  cd ./oam-kubernetes-runtime	
  ```

* Apply a sample application configuration

  ```shell script
  kubectl apply -f examples/containerized-workload
  ```

* Verify that the application is running

  Check its components:

  ```console
  kubectl get components
  NAME                WORKLOAD-KIND           AGE
  example-component   ContainerizedWorkload   63s
  ```

  Check its application configuration:

  ```console
  kubectl get appconfig
  NAME                AGE
  example-appconfig   3m48s
  ```

  Check the status and events from the application   
  ```console
  kubectl describe appconfig example-appconfig
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

  You should also check underlying deployment and service looking like below
  ```console
  kubectl get deployments
  NAME                                    READY   UP-TO-DATE   AVAILABLE   AGE
  example-appconfig-workload-deployment   3/3   3           3              28s
  ```

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

## Want to help?
Check out [DEVELOPMENT.md](./DEVELOPMENT.md) to see how to develop with OAM Kubernetes runtime

## Licenses
The OAM Kubernetes runtime is released under the [Apache 2.0 license](LICENSE).
