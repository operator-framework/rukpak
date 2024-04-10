/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/gorilla/handlers"
	"github.com/spf13/pflag"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kube-aggregator/pkg/apis/apiregistration"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crfinalizer "sigs.k8s.io/controller-runtime/pkg/finalizer"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	helmclient "github.com/operator-framework/helm-operator-plugins/pkg/client"

	rukpakv1alpha2 "github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/operator-framework/rukpak/internal/controllers/bundledeployment"
	"github.com/operator-framework/rukpak/internal/finalizer"
	"github.com/operator-framework/rukpak/internal/provisioner/plain"
	"github.com/operator-framework/rukpak/internal/provisioner/registry"
	"github.com/operator-framework/rukpak/internal/source"
	"github.com/operator-framework/rukpak/internal/storage"
	"github.com/operator-framework/rukpak/internal/util"
	"github.com/operator-framework/rukpak/internal/version"
	"github.com/operator-framework/rukpak/pkg/features"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	utilruntime.Must(rukpakv1alpha2.AddToScheme(scheme))
	utilruntime.Must(apiregistration.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var (
		httpBindAddr                string
		httpExternalAddr            string
		bundleCAFile                string
		enableLeaderElection        bool
		probeAddr                   string
		systemNamespace             string
		unpackImage                 string
		rukpakVersion               bool
		provisionerStorageDirectory string
	)
	flag.StringVar(&httpBindAddr, "http-bind-address", ":8080", "The address the http server binds to.")
	flag.StringVar(&httpExternalAddr, "http-external-address", "http://localhost:8080", "The external address at which the http server is reachable.")
	flag.StringVar(&bundleCAFile, "bundle-ca-file", "", "The file containing the certificate authority for connecting to bundle content servers.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&systemNamespace, "system-namespace", "", "Configures the namespace that gets used to deploy system resources.")
	flag.StringVar(&unpackImage, "unpack-image", util.DefaultUnpackImage, "Configures the container image that gets used to unpack Bundle contents.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&rukpakVersion, "version", false, "Displays rukpak version information")
	flag.StringVar(&provisionerStorageDirectory, "provisioner-storage-dir", storage.DefaultBundleCacheDir, "The directory that is used to store bundle contents.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	features.RukpakFeatureGate.AddFlag(pflag.CommandLine)
	pflag.Parse()

	if rukpakVersion {
		fmt.Println(version.String())
		os.Exit(0)
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog.Info("starting up the core controllers and servers", "git commit", version.String(), "unpacker image", unpackImage)

	dependentRequirement, err := labels.NewRequirement(util.CoreOwnerKindKey, selection.In, []string{rukpakv1alpha2.BundleDeploymentKind})
	if err != nil {
		setupLog.Error(err, "unable to create dependent label selector for cache")
		os.Exit(1)
	}
	dependentSelector := labels.NewSelector().Add(*dependentRequirement)

	cfg := ctrl.GetConfigOrDie()
	if systemNamespace == "" {
		systemNamespace = util.PodNamespace()
	}

	storageURL, err := url.Parse(fmt.Sprintf("%s/bundles/", httpExternalAddr))
	if err != nil {
		setupLog.Error(err, "unable to parse bundle content server URL")
		os.Exit(1)
	}

	localStorage := &storage.LocalDirectory{
		RootDirectory: provisionerStorageDirectory,
		URL:           *storageURL,
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&rukpakv1alpha2.BundleDeployment{}: {},
			},
			DefaultNamespaces: map[string]cache.Config{
				systemNamespace:     {},
				cache.AllNamespaces: {LabelSelector: dependentSelector},
			},
		},
		Metrics: server.Options{
			BindAddress: httpBindAddr,
			ExtraHandlers: map[string]http.Handler{
				// NOTE: ExtraHandlers aren't actually metrics-specific. We can run
				// whatever handlers we want on the existing webserver that
				// controller-runtime runs when MetricsBindAddress is configured on the
				// manager.
				"/bundles/": httpLogger(localStorage),
			},
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "core.rukpak.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	var rootCAs *x509.CertPool
	if bundleCAFile != "" {
		var err error
		if rootCAs, err = util.LoadCertPool(bundleCAFile); err != nil {
			setupLog.Error(err, "unable to parse bundle certificate authority file")
			os.Exit(1)
		}
	}

	httpLoader := storage.NewHTTP(
		storage.WithRootCAs(rootCAs),
		storage.WithBearerToken(cfg.BearerToken),
	)
	bundleStorage := storage.WithFallbackLoader(localStorage, httpLoader)

	// This finalizer logic MUST be co-located with this main
	// controller logic because it deals with cleaning up bundle data
	// from the bundle cache when the bundles are deleted. The
	// consequence is that this process MUST remain running in order
	// to process DELETE events for bundles that include this finalizer.
	// If this process is NOT running, deletion of such bundles will
	// hang until $something removes the finalizer.
	//
	// If the bundle cache is backed by a storage implementation that allows
	// multiple writers from different processes (e.g. a ReadWriteMany volume or
	// an S3 bucket), we could have separate processes for finalizer handling
	// and the primary provisioner controllers. For now, the assumption is
	// that we are not using such an implementation.
	bundleFinalizers := crfinalizer.NewFinalizers()
	if err := bundleFinalizers.Register(finalizer.DeleteCachedBundleKey, &finalizer.DeleteCachedBundle{Storage: bundleStorage}); err != nil {
		setupLog.Error(err, "unable to register finalizer", "finalizerKey", finalizer.DeleteCachedBundleKey)
		os.Exit(1)
	}

	unpacker, err := source.NewDefaultUnpacker(mgr, systemNamespace, unpackImage, rootCAs)
	if err != nil {
		setupLog.Error(err, "unable to setup bundle unpacker")
		os.Exit(1)
	}

	bdNamespaceMapper := func(obj client.Object) (string, error) {
		bd, ok := obj.(*rukpakv1alpha2.BundleDeployment)
		if !ok {
			return "", fmt.Errorf("cannot derive namespace from object of type %T", obj)
		}
		return bd.Spec.InstallNamespace, nil
	}
	systemNamespaceMapper := func(obj client.Object) (string, error) {
		return systemNamespace, nil
	}
	cfgGetter, err := helmclient.NewActionConfigGetter(mgr.GetConfig(), mgr.GetRESTMapper(),
		helmclient.ClientNamespaceMapper(bdNamespaceMapper),
		helmclient.StorageNamespaceMapper(systemNamespaceMapper),
	)
	if err != nil {
		setupLog.Error(err, "unable to create action config getter")
		os.Exit(1)
	}

	acg, err := helmclient.NewActionClientGetter(cfgGetter)
	if err != nil {
		setupLog.Error(err, "unable to create action client getter")
		os.Exit(1)
	}
	commonBDProvisionerOptions := []bundledeployment.Option{
		bundledeployment.WithActionClientGetter(acg),
		bundledeployment.WithFinalizers(bundleFinalizers),
		bundledeployment.WithStorage(bundleStorage),
		bundledeployment.WithUnpacker(unpacker),
	}

	if err := bundledeployment.SetupWithManager(mgr, systemNamespace, append(
		commonBDProvisionerOptions,
		bundledeployment.WithProvisionerID(plain.ProvisionerID),
		bundledeployment.WithHandler(bundledeployment.HandlerFunc(plain.HandleBundleDeployment)),
	)...); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", rukpakv1alpha2.BundleDeploymentKind, "provisionerID", plain.ProvisionerID)
		os.Exit(1)
	}

	if err := bundledeployment.SetupWithManager(mgr, systemNamespace, append(
		commonBDProvisionerOptions,
		bundledeployment.WithProvisionerID(registry.ProvisionerID),
		bundledeployment.WithHandler(bundledeployment.HandlerFunc(registry.HandleBundleDeployment)),
	)...); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", rukpakv1alpha2.BundleDeploymentKind, "provisionerID", registry.ProvisionerID)
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func httpLogger(h http.Handler) http.Handler {
	return handlers.CustomLoggingHandler(nil, h, func(_ io.Writer, params handlers.LogFormatterParams) {
		ctrl.Log.WithName("http").Info("responded", "method", params.Request.Method, "status", params.StatusCode, "url", params.URL.String(), "size", params.Size)
	})
}
