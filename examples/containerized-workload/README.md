# Custom Workload

This is an example web application with a custom workload

## Run ApplicationConfiguration

Install the components.

```bash
$ kubectl apply -f .
applicationconfiguration.core.oam.dev/example-appconfig created
component.core.oam.dev/example-component created
healthscope.core.oam.dev/example-health-scope created
secret/mysecret created
```

## Result

An `example-component` is created (running Wordpress, which you will see on the corresponding `Service` endpoint):

```
$ kubectl get all
NAME                                     READY   STATUS    RESTARTS   AGE
pod/example-component-564b7b45fd-9lgvr   1/1     Running   0          46s
pod/example-component-564b7b45fd-q4nxs   1/1     Running   0          46s
pod/example-component-564b7b45fd-r9nbg   1/1     Running   0          47s

NAME                        TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)        AGE
service/example-component   NodePort    10.109.59.180   <none>        80:31537/TCP   47s
service/kubernetes          ClusterIP   10.96.0.1       <none>        443/TCP        10d

NAME                                READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/example-component   3/3     3            3           47s

NAME                                           DESIRED   CURRENT   READY   AGE
replicaset.apps/example-component-564b7b45fd   3         3         3       47s
```

Wordpress will respond on port 80. There is a UI, but since there is no database it will not be fully functional.

## Clean up

Clean resources.

```bash
$ kubectl delete -f .
```
