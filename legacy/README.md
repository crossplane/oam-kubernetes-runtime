# Legacy Support

Now lots of apps are still running on K8s clusters version v1.14, v1.15, while oam-k8s-runtime requires the minimum
K8s version to be v1.16.

Currently, the main block is OAM runtime use CRD v1, while these old K8s versions don't support CRD v1.
So we generate v1beta1 CRD here for convenience. But we have no guarantee that oam-runtime will support the
legacy k8s versions. 

Follow the instructions in [README](../README.md) to create a namespace like `oam-system` and add the OAM Kubernetes
Runtime helm repo.

```
$ kubectl create namespace oam-system
$ helm repo add crossplane-master https://charts.crossplane.io/master/
```

Run the following command to install an OAM Kubernetes Runtime legacy chart.

```
$ helm install oam --namespace oam-system crossplane-master/oam-kubernetes-runtime-legacy --devel
```

If you'd like to install an older version of the legacy chart, use `helm search` to choose a proper chart version.
```
$ helm search repo oam-kubernetes-runtime-legacy --devel -l
  NAME                                           	CHART VERSION   	APP VERSION     	DESCRIPTION
  crossplane-master/oam-kubernetes-runtime-legacy	0.3......       	0.3......           A Helm chart for OAM Kubernetes Resources Contr
  crossplane-master/oam-kubernetes-runtime-legacy	0.3.1-5.g11e1894	0.3.1-5.g11e1894	A Helm chart for OAM Kubernetes Resources Contr
  crossplane-master/oam-kubernetes-runtime-legacy	0.3......       	0.3......	        A Helm chart for OAM Kubernetes Resources Contr

$ helm install oam --namespace oam-system crossplane-master/oam-kubernetes-runtime-legacy --version 0.3.1-5.g11e1894 --devel
```

Install the legacy chart as below if you want a nightly version.

```
$ helm install oam --namespace oam-system crossplane-master/oam-kubernetes-runtime-legacy --set image.tag=master --devel
```
