package e2e

// ImageRepo is meant to be set via -ldflags during execution of this code
// where it will then be used to configure the testing suite.
var ImageRepo = "docker-registry.rukpak-e2e.svc.cluster.local:5000/bundles"
