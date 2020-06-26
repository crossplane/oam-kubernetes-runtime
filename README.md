# OAM Kubernetes Runtime

[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/mod/github.com/crossplane/oam-kubernetes-runtime)

The OAM Kubernetes runtime project is a collection of Golang helper libraries and OAM api type
definitions. 

It is designed to help OAM platform builders rather than being used directly by developers
or end-users. We would recommend end-users to check
[Crossplane  official  documentation](https://crossplane.github.io/docs) instead.

In addition, we created this library with the following goals in mind  
* All OAM Kubernetes platform builders use and contribute to this library. 
* The wide adoption of this library can facilitate workload/trait interoperability among OAM
 Kubernetes platform builders.

## Getting started
Check out [DEVELOPMENT.md](./DEVELOPMENT.md) to see how to develop with OAM Kubernetes runtime

And a `HealthScope` looking like below
```
kubectl get healthscopes.core.oam.dev
NAME                   AGE
example-health-scope   23s
```

## Community, discussion, contribution
You can reach the maintainers of this project at:
* Slack channel: [crossplane#oam](https://crossplane.slack.com/#oam)

## Licenses
The OAM Kubernetes runtime is released under the [Apache 2.0 license](LICENSE).
