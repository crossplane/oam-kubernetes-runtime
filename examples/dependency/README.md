# Prerequisite


Prepare CRD and Definitions:

```shell
kubectl apply -f examples/dependency/definition.yaml
```

Make sure [`OAM runtime`](../../README.md#install-oam-runtime) was installed and started.


# Case 1: Use status as output and pass through to another workload 

Let's see the application yaml:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  components:
    - componentName: source
      dataOutputs:
      - name: example-key
        fieldPath: "status.key"
    - componentName: sink
      dataInputs:
      - valueFrom:
          dataOutputName: example-key
        toFieldPaths:
        - "spec.key"
```

In this example, we want `status.key` from `source` component and pass it as `spec.key` to `sink` component.

Run this command to make this demo work:

```shell script
kubectl apply -f examples/dependency/demo.yaml
```

At initial time, the workload of `source` component don't have `status.key`:

```shell script
$ kubectl get foo.example.com source -o yaml
apiVersion: example.com/v1
kind: Foo
metadata:
  name: source
  namespace: default
```

And the workload of `sink` component not exist yet.

After a while, assuming the controller of this CRD will reconcile and give the status.

Let manually add it by `kubectl edit foo.example.com source`

```yaml
    apiVersion: example.com/v1
    kind: Foo
    metadata:
      name: source
      namespace: default
+   status:
+     key: test 
```

Then the dependency will meet the requirement. You should see that the `sink` workload appears and
the field `spec.key` of `sink` workload has been filled:

```shell script
$ kubectl get foo sink -o yaml
apiVersion: example.com/v1
kind: Foo
metadata:
  name: sink 
  namespace: default
spec:
  key: test
```

clean resource for next case:

```shell script
kubectl delete -f examples/dependency/demo.yaml
kubectl delete foo --all
```

# Case 2: Use matcher to evaluate whether a resource is ready

In this example, we will add matcher base on dependency.

```shell script
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  components:
    - componentName: source
      dataOutputs:
        - name: example-key
          fieldPath: "spec.key"
          conditions:
            - op: eq
              value: running
              fieldPath: "status.state"
    - componentName: sink
      dataInputs:
        - valueFrom:
            dataOutputName: example-key
          toFieldPaths:
            - "spec.key"
```

Run this command to make this demo work:

```shell script
kubectl apply -f examples/dependency/demo-with-conditions.yaml
```


You can see that we want field `spec.key` from source component which already exist.

```shell script
$ kubectl get foo.example.com source -o yaml
apiVersion: example.com/v1
kind: Foo
metadata:
  name: source
  namespace: default
spec:
  key: test-value
status:
  state: pending
```

But the state of status is `pending`. We hope this state to be running and only after that the dependency system
can pass `spec.key` to `sink` component. So we specify a matcher for it.

```yaml
          matchers:
            - op: eq
              value: running
              fieldPath: "status.state"
```
 
And now `sink` workload is not created because its dataInputs is not ready.

```shell script
$ kubectl get foo.example.com sink -o yaml
Error from server (NotFound): foo.example.com "sink" not found
```

After a while, assuming the controller of this CRD will reconcile and make the `status.state` to be running.
Let's update it manually. 

```shell script
$ kubectl edit foo.example.com source -o yaml
apiVersion: example.com/v1
kind: Foo
metadata:
  name: source
  namespace: default
spec:
  key: test-value
status:
-  state: pending
+  state: running
```

Then the dependency will meet the requirement. You should see that the field "spec.key" of `sink` workload has been filled.

```shell
$ kubectl get foo.example.com sink -o yaml
apiVersion: example.com/v1
kind: Foo
metadata:
  name: sink
  namespace: default
spec:
  key: test-value
```

clean resource for next case:

```shell script
kubectl delete -f examples/dependency/demo-with-conditions.yaml
kubectl delete foo --all
```