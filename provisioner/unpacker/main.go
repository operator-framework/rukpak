package main

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/operator-framework/rukpak/api/v1alpha1"
)

type unpackOptions struct {
	BundleName       string
	StorageNamespace string
	ManifestDir      string
}

func newCmd() *cobra.Command {
	o := unpackOptions{}

	cmd := &cobra.Command{
		Use: "run",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			logger := logrus.New()
			if err := o.run(ctx, logger); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&o.BundleName, "bundle", "", "configures which existing Bundle custom resource to pull and unpack referenced content")
	cmd.Flags().StringVar(&o.StorageNamespace, "namespace", "rukpak", "configures the namespace that houses the bundle storage resources")
	cmd.Flags().StringVar(&o.ManifestDir, "manifests", "manifests", "configures the directory that contains the plain bundle manifests to unpack")

	return cmd
}

func main() {
	cmd := newCmd()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (o unpackOptions) run(ctx context.Context, logger logrus.FieldLogger) error {
	config, err := controllerruntime.GetConfig()
	if err != nil {
		return err
	}
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		return err
	}
	client, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	bundle := &v1alpha1.Bundle{}
	if err := client.Get(ctx, types.NamespacedName{Name: o.BundleName}, bundle); err != nil {
		return err
	}

	cm := &corev1.ConfigMap{}
	if err := client.Get(ctx, types.NamespacedName{
		Name:      bundle.GetName(),
		Namespace: o.StorageNamespace,
	}, cm); err != nil {
		return err
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}

	ref, err := name.ParseReference(string(bundle.Spec.Source))
	if err != nil {
		return err
	}

	logger.Infof("attempting to pull the %s image", ref)
	image, err := remote.Image(ref)
	if err != nil {
		return err
	}
	logger.Infof("successfully pull the %s image", ref)

	t := tar.NewReader(mutate.Extract(image))
	for true {
		header, err := t.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.FileInfo().IsDir() {
			continue
		}
		if !strings.Contains(header.Name, o.ManifestDir) {
			continue
		}
		logger.Infof("processing the %s file", header.Name)

		content, err := ioutil.ReadAll(t)
		if err != nil {
			return fmt.Errorf("failed to read from the current %s file: %v", header.Name, err)
		}
		cm.Data[filepath.Base(header.Name)] = string(content)
	}
	if len(cm.Data) == 0 {
		return fmt.Errorf("failed to write any contents to the %s/%s configmap", cm.GetName(), cm.GetNamespace())
	}

	logrus.Infof("attempting to update the %s/%s configmap with bundle contents", cm.GetNamespace(), cm.GetName())
	if err := client.Update(ctx, cm); err != nil {
		return fmt.Errorf("failed to update the %s/%s configmap: %v", cm.GetNamespace(), cm.GetName(), err)
	}
	logrus.Infof("bundle unpack process completed")

	return nil
}
