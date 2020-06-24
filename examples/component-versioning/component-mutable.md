# Component Mutable and use ControllerRevision to record 

## Prerequisite

1. Make sure [`addon-oam-kubernetes-local`](https://github.com/crossplane/addon-oam-kubernetes-local) was installed.
2. Before component versioning mechanism released in Crossplane, you can run `go run examples/containerized-workload/main.go`
   instead of using crossplane. After this feature was released in crossplane, install crossplane will work.

## ApplicationConfiguration always using the latest component

Step 1. Create OAM component

```shell script
$ kubectl apply -f examples/containerized-workload/sample_component.yaml
component.core.oam.dev/example-component created
``` 

ControllerRevision of Component created automatically

```shell script
$ kubectl get controllerrevisions.apps
NAME                                     CONTROLLER                                 REVISION   AGE
example-component-brk71b3ipt3d60vfo4sg   component.core.oam.dev/example-component   1          13s
``` 

Snapshot of current component stored in this ControllerRevision

```shell script
$ kubectl get controllerrevisions.apps example-component-brk71rbipt3d60vfo4t0 -o yaml
apiVersion: apps/v1
data:
  metadata:
    name: example-component
    namespace: default
    ...
  spec:
    workload:
      apiVersion: core.oam.dev/v1alpha2
      kind: ContainerizedWorkload
      spec:
        containers:
        - env:
          - name: TEST_ENV
            value: test
          image: wordpress:4.6.1-apache
          name: wordpress
          ports:
          - containerPort: 80
            name: wordpress
  status:
    latestRevision: ""
kind: ControllerRevision
metadata:
  name: example-component-brk71rbipt3d60vfo4t0
  namespace: default
  ownerReferences:
  - apiVersion: core.oam.dev/v1alpha2
    controller: true
    kind: Component
    name: example-component
  ...
revision: 1
```

Step 2. Create AppConfig

```shell script
$ kubectl apply -f examples/containerized-workload/sample_application_config.yaml
applicationconfiguration.core.oam.dev/example-appconfig created
```

ContainerizedWorkload was created with **the name of componentName**.

```shell script
$ kubectl get containerizedworkloads.core.oam.dev
NAME                AGE
example-component   7s
```

You can get the detail by:

```shell script
$ kubectl get containerizedworkloads.core.oam.dev example-component -o yaml
apiVersion: core.oam.dev/v1alpha2
kind: ContainerizedWorkload
metadata:
  name: example-component
  namespace: default
  ...
spec:
  containers:
  - env:
    - name: TEST_ENV
      value: test
    image: wordpress:4.6.1-apache
    name: wordpress
    ports:
    - containerPort: 80
      name: wordpress
status:
  ...
```


Step 3. Update the component will trigger upgrade automatically

Change the component yaml with the ENV value.

```
$ kubectl edit components example-component
component.core.oam.dev/example-component edited
```

```yaml
     containers:
      - env:
        - name: TEST_ENV
-         value: test
+         value: test2
```

A new controllerRevision instance created.

```
$ kubectl get controllerrevisions.apps
NAME                                     CONTROLLER                                 REVISION   AGE
example-component-brk71rbipt3d60vfo4t0   component.core.oam.dev/example-component   1          15m
example-component-brk78gript3d60vfo4tg   component.core.oam.dev/example-component   2          55s
```

With its content pointing to the new snapshot of component.

```shell script
$ kubectl get controllerrevisions.apps example-component-brk78gript3d60vfo4tg -o yaml
apiVersion: apps/v1
kind: ControllerRevision
metadata:
  name: example-component-brk78gript3d60vfo4tg
  ...
revision: 2
data:
  ...
        containers:
        - env:
          - name: TEST_ENV
            value: test2
  ...
```

The workload also updated, which means the change of component has triggered the upgrade of the App.

```shell script
$ kubectl get containerizedworkloads.core.oam.dev example-component -o yaml
apiVersion: core.oam.dev/v1alpha2
kind: ContainerizedWorkload
metadata:
  name: example-component
  ...
spec:
  containers:
  - env:
    - name: TEST_ENV
      value: test2
    image: wordpress:4.6.1-apache
    name: wordpress
    ports:
    - containerPort: 80
      name: wordpress
status:
  ...
```

Step 4. Clean the environment for next demo

```shell script
$ kubectl delete appconfig example-appconfig
applicationconfiguration.core.oam.dev "example-appconfig" deleted
$ kubectl delete component example-component
component.core.oam.dev "example-component" deleted
``` 

## ApplicationConfiguration specify revision manually


Step 1. The first step is the same. Create OAM component, and check the ControllerRevision.

```shell script
$ kubectl apply -f examples/containerized-workload/sample_component.yaml
component.core.oam.dev/example-component created
``` 

```shell script
$ kubectl get controllerrevisions.apps
NAME                                     CONTROLLER                                 REVISION   AGE
example-component-brk71b3ipt3d60vfo4sg   component.core.oam.dev/example-component   1          13s
``` 

Step 2. Specify ControllerRevision in OAM AppConfig.

In our example, the name of revision is `example-component-brk71b3ipt3d60vfo4sg`,
so we must specify it in AppConfig with revisionName.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  components:
    - revisionName: example-component-brk71b3ipt3d60vfo4sg
      traits:
        - trait:
            apiVersion: core.oam.dev/v1alpha2
            kind: ManualScalerTrait
            spec:
              replicaCount: 3
```

Assume we name it as `component-mutable-app.yaml` and apply this AppConfig.

```shell script
$ kubectl apply -f component-mutable-app.yaml
applicationconfiguration.core.oam.dev/example-appconfig created
```

ContainerizedWorkload created with the same name with componentName.

```shell script
$ kubectl get containerizedworkloads.core.oam.dev
NAME                AGE
example-component   21m
```

```shell script
k get containerizedworkloads.core.oam.dev example-component -o yaml
apiVersion: core.oam.dev/v1alpha2
kind: ContainerizedWorkload
metadata:
  name: example-component
  namespace: default
  ...
spec:
  containers:
  - env:
    - name: TEST_ENV
      value: test
    image: wordpress:4.6.1-apache
    name: wordpress
   ...
```

Step 3. Change Component will not affect the workload.

```
$ kubectl edit components example-component
component.core.oam.dev/example-component edited
```

```yaml
     containers:
      - env:
        - name: TEST_ENV
-         value: test
+         value: test2
```

The controllerRevision was created.

```shell script
NAME                                     CONTROLLER                                 REVISION   AGE
example-component-brk71b3ipt3d60vfo4sg   component.core.oam.dev/example-component   1          29m
example-component-brke6rbipt3d60vfo4ug   component.core.oam.dev/example-component   2          73s
```

But the workload didn't change.

```shell script
$ kubectl get containerizedworkloads.core.oam.dev example-component -o yaml
apiVersion: core.oam.dev/v1alpha2
kind: ContainerizedWorkload
metadata:
  name: example-component
  namespace: default
  ...
spec:
  containers:
  - env:
    - name: TEST_ENV
      value: test
    image: wordpress:4.6.1-apache
    name: wordpress
    ports:
    - containerPort: 80
      name: wordpress
```

So specify revisionName will let AppConfig use a fixed revision of component.


Step 4. Clean the environment for next demo

```shell script
$ kubectl delete appconfig example-appconfig
applicationconfiguration.core.oam.dev "example-appconfig" deleted
$ kubectl delete component example-component
component.core.oam.dev "example-component" deleted
```

## Note

In this case, we use ContainerizedWorkload as an example. The general rule applies to any type of workload.