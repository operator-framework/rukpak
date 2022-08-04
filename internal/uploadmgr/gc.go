package uploadmgr

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

type bundleGC struct {
	storageDir          string
	storageSyncInterval time.Duration
	cache               cache.Cache
	log                 logr.Logger
}

// NewBundleGC returns a Runnable for controller-runtime that automatically garbage-collects
// bundle uploads as those bundles are deleted. In case deletion events are missed, the
// garbage collector also periodically deletes files in the storageDir that are not
// associated with an active uploaded bundle.
func NewBundleGC(cache cache.Cache, storageDir string, storageSyncInterval time.Duration) manager.Runnable {
	return &bundleGC{storageDir: storageDir, storageSyncInterval: storageSyncInterval, cache: cache, log: ctrl.Log.WithName("gc")}
}

// Start implemente the controller-runtime Runnable interface.
// It blocks until the context is closed.
func (gc *bundleGC) Start(ctx context.Context) error {
	bundleInformer, err := gc.cache.GetInformer(ctx, &rukpakv1alpha1.Bundle{})
	if err != nil {
		return err
	}
	bundleInformer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj interface{}) {
			bundle := obj.(*rukpakv1alpha1.Bundle)
			if bundle.Spec.Source.Type != rukpakv1alpha1.SourceTypeUpload {
				return
			}
			filename := bundlePath(gc.storageDir, bundle.Name)
			gc.log.Info("removing file", "path", filename)
			if err := os.RemoveAll(filename); err != nil {
				gc.log.Error(err, "failed to remove file", "path", filename)
			}
		},
	})

	// Wait for the cache to sync to ensure that our bundle List calls
	// in the below loop see a full view of the bundles that exist in
	// the cluster.
	if ok := gc.cache.WaitForCacheSync(ctx); !ok {
		if ctx.Err() == nil {
			return fmt.Errorf("cache did not sync")
		}
		return fmt.Errorf("cache did not sync: %v", ctx.Err())
	}

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
				if bundle.Spec.Source.Type != rukpakv1alpha1.SourceTypeUpload {
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
