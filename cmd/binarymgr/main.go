/*
Copyright 2022.

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
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(rukpakv1alpha1.AddToScheme(scheme))
}

func main() {
	var (
		httpAddress         string
		storageDir          string
		storageSyncInterval time.Duration
		probeAddr           string
	)
	flag.StringVar(&httpAddress, "http-bind-address", "127.0.0.1:8080", "Listen address for http upload and metrics endpoints")
	flag.StringVar(&storageDir, "storage-dir", "/var/cache/bundles", "Directory in which to store uploaded bundles")
	flag.DurationVar(&storageSyncInterval, "storage-sync-interval", time.Minute, "Interval on which to garbage collect unused bundles")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     httpAddress,
		HealthProbeBindAddress: probeAddr,
	})
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	// NOTE: AddMetricsExtraHandler isn't actually metrics-specific. We can run
	// whatever handlers we want on the existing webserver that
	// controller-runtime runs when MetricsBindAddress is configured on the
	// manager.
	if err := mgr.AddMetricsExtraHandler("/bundles/", newUploadHandler(mgr.GetClient(), storageDir)); err != nil {
		setupLog.Error(err, "unable to add upload handler to manager")
		os.Exit(1)
	}
	if err := mgr.Add(newBundleGC(mgr.GetCache(), storageDir, storageSyncInterval)); err != nil {
		setupLog.Error(err, "unable to add bundle garbage collector to manager")
		os.Exit(1)
	}
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

func newUploadHandler(cl client.Client, storageDir string) http.Handler {
	r := mux.NewRouter()
	r.Methods(http.MethodGet).PathPrefix("/bundles/").Handler(http.StripPrefix("/bundles/", http.FileServer(http.FS(os.DirFS(storageDir)))))
	r.Methods(http.MethodPut).Path("/bundles/{bundleName}.tgz").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bundleName := mux.Vars(r)["bundleName"]

		bundle := &rukpakv1alpha1.Bundle{}
		if err := cl.Get(r.Context(), types.NamespacedName{Name: bundleName}, bundle); err != nil {
			http.Error(w, err.Error(), int(getCode(err)))
			return
		}
		if bundle.Spec.Source.Type != rukpakv1alpha1.SourceTypeBinary {
			http.Error(w, fmt.Sprintf("bundle source type is %q; expected %q", bundle.Spec.Source.Type, rukpakv1alpha1.SourceTypeBinary), http.StatusConflict)
			return
		}
		if bundle.Status.Phase == rukpakv1alpha1.PhaseUnpacked {
			http.Error(w, "bundle has already been unpacked, cannot change content of existing bundle", http.StatusConflict)
			return
		}

		bundleFile, err := os.Create(bundlePath(storageDir, bundleName))
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to store bundle binary: %v", err), http.StatusInternalServerError)
			return
		}
		defer bundleFile.Close()

		if _, err := io.Copy(bundleFile, r.Body); err != nil {
			http.Error(w, fmt.Sprintf("failed to store bundle binary: %v", err), http.StatusInternalServerError)
			return
		}

		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := cl.Get(r.Context(), types.NamespacedName{Name: bundleName}, bundle); err != nil {
				return err
			}
			if bundle.Status.Phase == rukpakv1alpha1.PhaseUnpacked {
				return nil
			}

			bundle.Status.Phase = rukpakv1alpha1.PhasePending
			meta.SetStatusCondition(&bundle.Status.Conditions, metav1.Condition{
				Type:    rukpakv1alpha1.TypeUnpacked,
				Status:  metav1.ConditionFalse,
				Reason:  rukpakv1alpha1.ReasonUnpackPending,
				Message: "received binary upload, waiting for provisioner to unpack it.",
			})
			return cl.Status().Update(r.Context(), bundle)
		}); err != nil {
			http.Error(w, err.Error(), int(getCode(err)))
			return
		}
	})
	return handlers.CustomLoggingHandler(nil, r, func(_ io.Writer, params handlers.LogFormatterParams) {
		ctrl.Log.WithName("http").Info("responded", "method", params.Request.Method, "status", params.StatusCode, "url", params.URL.String(), "size", params.Size)
	})
}

func getCode(err error) int32 {
	if status := apierrors.APIStatus(nil); errors.As(err, &status) {
		return status.Status().Code
	}
	return http.StatusInternalServerError
}

func bundlePath(baseDir, bundleName string) string {
	return filepath.Join(baseDir, fmt.Sprintf("%s.tgz", bundleName))
}

type bundleGC struct {
	storageDir          string
	storageSyncInterval time.Duration
	cache               cache.Cache
	log                 logr.Logger
}

func newBundleGC(cache cache.Cache, storageDir string, storageSyncInterval time.Duration) *bundleGC {
	return &bundleGC{storageDir: storageDir, storageSyncInterval: storageSyncInterval, cache: cache, log: ctrl.Log.WithName("gc")}
}

func (gc *bundleGC) Start(ctx context.Context) error {
	bundleInformer, err := gc.cache.GetInformer(ctx, &rukpakv1alpha1.Bundle{})
	if err != nil {
		return err
	}
	bundleInformer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj interface{}) {
			bundle := obj.(*rukpakv1alpha1.Bundle)
			if bundle.Spec.Source.Type != rukpakv1alpha1.SourceTypeBinary {
				return
			}
			filename := bundlePath(gc.storageDir, bundle.Name)
			gc.log.Info("removing file", "path", filename)
			if err := os.RemoveAll(filename); err != nil {
				gc.log.Error(err, "failed to remove file", "path", filename)
			}
		},
	})
	ticker := time.NewTicker(gc.storageSyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			storageDirEntries, err := os.ReadDir(gc.storageDir)
			if err != nil {
				gc.log.Error(err, "failed to read local bundle storage directory")
				continue
			}
			existingFiles := sets.NewString()
			for _, e := range storageDirEntries {
				existingFiles.Insert(e.Name())
			}
			bundles := &rukpakv1alpha1.BundleList{}
			if err := gc.cache.List(ctx, bundles); err != nil {
				gc.log.Error(err, "failed to list bundles from cache", err)
				continue
			}
			for _, bundle := range bundles.Items {
				if bundle.Spec.Source.Type != rukpakv1alpha1.SourceTypeBinary {
					continue
				}
				existingFiles.Delete(filepath.Base(bundlePath(gc.storageDir, bundle.Name)))
			}
			for _, staleFile := range existingFiles.List() {
				filename := filepath.Join(gc.storageDir, staleFile)
				gc.log.Info("removing file", "path", filename)
				if err := os.RemoveAll(filename); err != nil {
					gc.log.Error(err, "failed to remove file", "path", filename)
				}
			}
		}
	}
}
