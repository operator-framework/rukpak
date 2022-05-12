package bundleinstance

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestUpdater(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Updater Suite")
}
