module github.com/crossplane/oam-kubernetes-runtime

go 1.15

require (
	github.com/crossplane/crossplane-runtime v0.10.0
	github.com/davecgh/go-spew v1.1.1
	github.com/evanphx/json-patch v4.9.0+incompatible
	github.com/ghodss/yaml v1.0.0
	github.com/go-logr/logr v0.4.0
	github.com/google/go-cmp v0.5.2
	github.com/kr/pretty v0.2.0 // indirect
	github.com/oam-dev/kubevela v1.0.3
	github.com/onsi/ginkgo v1.13.0
	github.com/onsi/gomega v1.10.3
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.7.0
	github.com/tidwall/gjson v1.6.8
	go.uber.org/zap v1.16.0
	golang.org/x/tools v0.0.0-20200630223951-c138986dd9b9 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	k8s.io/api v0.18.8
	k8s.io/apiextensions-apiserver v0.18.6
	k8s.io/apimachinery v0.18.8
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20200410145947-bcb3869e6f29
	k8s.io/kubectl v0.18.6
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	sigs.k8s.io/controller-runtime v0.6.2
	sigs.k8s.io/controller-tools v0.2.4
)

replace (
	k8s.io/client-go => k8s.io/client-go v0.18.6
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20200410145947-61e04a5be9a6
	k8s.io/kubectl => k8s.io/kubectl v0.18.6
	k8s.io/utils => k8s.io/utils v0.0.0-20200603063816-c1c6865ac451
)
