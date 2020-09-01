module github.com/crossplane/oam-kubernetes-runtime

go 1.13

require (
	github.com/crossplane/crossplane-runtime v0.8.0
	github.com/davecgh/go-spew v1.1.1
	github.com/gertd/go-pluralize v0.1.7
	github.com/ghodss/yaml v1.0.0
	github.com/go-logr/logr v0.1.0
	github.com/google/go-cmp v0.4.0
	github.com/json-iterator/go v1.1.8
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	github.com/pkg/errors v0.9.1
	github.com/rs/xid v1.2.1
	github.com/stretchr/testify v1.4.0
	go.uber.org/zap v1.10.0
	golang.org/x/tools v0.0.0-20200630223951-c138986dd9b9 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	k8s.io/api v0.18.5
	k8s.io/apiextensions-apiserver v0.18.2
	k8s.io/apimachinery v0.18.5
	k8s.io/client-go v0.18.5
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20200410145947-61e04a5be9a6
	k8s.io/kubectl v0.18.5
	sigs.k8s.io/controller-runtime v0.6.0
	sigs.k8s.io/controller-tools v0.2.4
)
