Run the following commands to prepare demo AppConfig and Components, and start oam-runtime:

```shell
kubectl delete -f examples/dependency/demo.yaml
kubectl delete foo --all
kubectl apply -f examples/dependency/
go run examples/containerized-workload/main.go
```

You should see that the field "spec.key" has been filled:

```shell
kubectl get foo sink -o yaml
```

