module github.com/crossplane/oam-kubernetes-runtime

go 1.13

require (
	github.com/crossplane/crossplane-runtime v0.8.0
	github.com/crossplane/oam-controllers v0.0.0-00010101000000-000000000000
	github.com/google/go-cmp v0.4.0
	github.com/pkg/errors v0.8.1
	github.com/rs/xid v1.2.1
	golang.org/x/tools v0.0.0-20200325010219-a49f79bcc224
	k8s.io/api v0.18.2
	k8s.io/apiextensions-apiserver v0.18.2
	k8s.io/apimachinery v0.18.2
	k8s.io/client-go v0.18.2
	sigs.k8s.io/controller-runtime v0.6.0
	sigs.k8s.io/controller-tools v0.2.4
)

replace github.com/crossplane/oam-controllers => github.com/crossplane/addon-oam-kubernetes-local v0.0.0-20200522083149-1bc0918a6ce9
