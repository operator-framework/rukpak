package v0_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPlainV0(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Plain+v0 Suite")
}
