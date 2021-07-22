package main

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func main() {
	cmd := &cobra.Command{
		Use:          "unpack",
		Short:        "Unpack arbitrary bundle content from a container image",
		SilenceUsage: true,
		RunE:         run,
	}

	// TODO(tflannag): How to surface something like a unique ID?
	// TODO(tflannag): Use a CronJob instead of a Job so we're not unnecessarily firing
	// 				   off Jobs to check for new content?
	// TODO(tflannag): Somehow inheriting this `--azure-container-registry-config` flag from the g/go-containerregistry import.
	cmd.Flags().String("image", "", "configures the container image that contains bundle content")
	cmd.Flags().String("namespace", "rukpak", "configures the namespace for in-cluster authentication")
	cmd.Flags().String("service-account-name", "rukpak", "configures the serviceaccount metadata.Name for in-cluster authentication")
	cmd.Flags().String("manifest-dir", "manifests", "configures the unpacker to unpack contents stored in the container image filesystem")
	cmd.Flags().String("output-dir", "manifests", "configures the directory to store unpacked bundle content")

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "encountered an error while unpacking the bundle container image: %v\n", err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	image, err := cmd.Flags().GetString("image")
	if err != nil {
		return err
	}
	outputDir, err := cmd.Flags().GetString("output-dir")
	if err != nil {
		return err
	}
	logger := logrus.New()

	logger.Infof("stating the %s output directory", outputDir)
	if _, err := os.Stat(outputDir); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		logger.Infof("attempting to create the %s output directory as it does not exist", outputDir)
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return err
		}
		logger.Infof("the output directory %s has been created", outputDir)
	}

	// TODO(tflannag): Need to handle more advance authentication methods.
	// TODO(tflannag): Expose a way to run this locally w/o setting up any
	// image pull secrets or configuring service accounts.
	auth := authn.DefaultKeychain
	reader, err := pullBundleSource(logger, image, auth)
	if err != nil {
		return err
	}
	defer reader.Close()

	out, err := os.Create(filepath.Join(outputDir, "bundle.tar.gz"))
	if err != nil {
		return err
	}
	defer out.Close()

	logger.Info("copying the tar reader contents to the output tar file")
	if _, err := io.Copy(out, reader); err != nil {
		return err
	}
	logger.Info("finished unpacking bundle contents")

	// // TODO(tflannag): expose a tar file vs. a flattened filesystem.
	// if err := unpackBundleTarToDirectory(logger, reader, manifestDir, outputDir); err != nil {
	// 	return err
	// }

	return nil
}

func pullBundleSource(
	logger logrus.FieldLogger,
	bundleSource string,
	auth authn.Keychain,
) (io.ReadCloser, error) {
	ref, err := name.ParseReference(bundleSource)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}

	logger.Infof("attempting to pull the %s bundle image", bundleSource)
	image, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.NewMultiKeychain(auth)))
	if err != nil {
		return nil, err
	}
	// TODO(tflannag): Is there any other metadata we need to be tracking here?
	size, err := image.Size()
	if err != nil {
		return nil, err
	}
	logger.Infof("unpacked image has size %v", size)

	return mutate.Extract(image), nil
}

func unpackBundleTarToDirectory(
	logger logrus.FieldLogger,
	reader io.ReadCloser,
	manifestDir,
	outputDir string,
) error {
	logger.Infof("unpacking filesystem content from the %s directory to the %s output directory", manifestDir, outputDir)
	t := tar.NewReader(reader)

	for true {
		header, err := t.Next()
		if err == io.EOF {
			logger.Debug("processed the EOF marker")
			break
		}
		if err != nil {
			return fmt.Errorf("failed to unpack bundle image: %v", err)
		}

		// TODO(tflannag): Should we only be filtering out the target
		// manifest directory?
		target := filepath.Clean(header.Name)
		if !strings.Contains(target, manifestDir) {
			continue
		}
		logger.Infof("processing the manifest file %s", target)

		switch header.Typeflag {
		case tar.TypeDir:
			logger.Infof("attempting to mkdir the %s output directory if it doesn't already exist", outputDir)
			os.Mkdir(outputDir, 0755)

		case tar.TypeReg:
			outFile, err := os.Create(filepath.Join(outputDir, filepath.Base(target)))
			if err != nil {
				return err
			}
			defer outFile.Close()
			if _, err := io.Copy(outFile, t); err != nil {
				return err
			}
		}
	}
	return nil
}
