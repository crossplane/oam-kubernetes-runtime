kubectl delete -f examples/dependency/demo.yaml
kubectl apply -f examples/dependency/definition.yaml
kubectl create -f examples/dependency/demo.yaml
go run cmd/oam-runtime/main.go
