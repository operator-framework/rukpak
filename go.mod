module github.com/operator-framework/rukpak

go 1.16

require (
	github.com/go-logr/logr v0.4.0
	github.com/google/go-containerregistry v0.5.2-0.20210609162550-f0ce2270b3b4
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.1.3
	k8s.io/api v0.21.2
	k8s.io/apimachinery v0.21.2
	k8s.io/client-go v0.21.2
	sigs.k8s.io/controller-runtime v0.9.2
	sigs.k8s.io/controller-tools v0.6.1
)
