// +build tools

package tools

import (
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint" // Better linting
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"     // Generate deepcopy, conversion, and CRDs
)
