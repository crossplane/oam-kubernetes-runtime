# Custom Workload

This is an example web application with a custom workload

## Run ApplicationConfiguration

Install the components.

```bash
$ kubectl apply -f .
workloaddefinition.core.oam.dev/web-service created
scopedefinition.core.oam.dev/healthscopes.core.oam.dev created
healthscope.core.oam.dev/example-health-scope created
applicationconfiguration.core.oam.dev/example-appconfig created
component.core.oam.dev/web-service-component created
```


> NOTE: The `oam-k8s-runtime` webhook is needed to enhance the workload in the `Component` definition. If you are running without the webhook you will need to add the following to the `spec.workload` in `sample_component.yaml`:  
>
>      apiVersion: core.oam.dev/v1alpha2
>      kind: ContainerizedWorkload


## Result

A `web-service-component` is created (running Wordpress, which you will see on the corresponding `Service` endpoint):

```
$ kubectl get all
NAME                                         READY   STATUS    RESTARTS   AGE
pod/web-service-component-78fbdd6787-5nwh5   1/1     Running   0          2m19s
pod/web-service-component-78fbdd6787-9gmfp   1/1     Running   0          2m20s
pod/web-service-component-78fbdd6787-r652l   1/1     Running   0          2m21s

NAME                            TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)        AGE
service/kubernetes              ClusterIP   10.96.0.1      <none>        443/TCP        10d
service/web-service-component   NodePort    10.99.171.93   <none>        80:30209/TCP   2m22s

NAME                                    READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/web-service-component   3/3     3            3           2m22s

NAME                                               DESIRED   CURRENT   READY   AGE
replicaset.apps/web-service-component-6b4b8b57b7   0         0         0       2m22s
replicaset.apps/web-service-component-78fbdd6787   3         3         3       2m21s
```

Wordpress will respond on port 80. There is a UI, but since there is no database it will not be fully functional.

## Clean up

Clean resources.

```bash
$ kubectl delete -f .
```
