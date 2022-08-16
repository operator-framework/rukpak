package bundle_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBundle(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bundle Suite")
}
