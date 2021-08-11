package main

import (
	"os"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/operator-framework/rukpak/provisioner/k8s/cmd"
)

func main() {
	log := zap.New()
	if err := cmd.Execute(log); err != nil {
		log.Error(err, "command failed")
		os.Exit(1)
	}
}
