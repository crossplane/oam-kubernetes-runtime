module github.com/crossplane/oam-kubernetes-runtime

go 1.13

require (
	github.com/crossplane/crossplane-runtime v0.7.1-0.20200421211018-be37c50cc2ab
	github.com/google/go-cmp v0.4.0
	github.com/onsi/ginkgo v1.10.1
	github.com/onsi/gomega v1.7.0
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_model v0.0.0-20190812154241-14fe0d1b01d4 // indirect
	golang.org/x/net v0.0.0-20200202094626-16171245cfb2 // indirect
	k8s.io/api v0.17.3
	k8s.io/apimachinery v0.17.3
	k8s.io/client-go v0.17.3
	sigs.k8s.io/controller-runtime v0.4.0
	sigs.k8s.io/controller-tools v0.2.4
)
