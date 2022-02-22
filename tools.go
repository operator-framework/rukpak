//go:build tools
// +build tools

package tools

import (
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint" // Better linting
	_ "github.com/onsi/ginkgo/ginkgo"                       // For running E2E tests
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"     // Generate deepcopy, conversion, and CRDs
)
