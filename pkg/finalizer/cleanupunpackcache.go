package finalizer

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/finalizer"

	"github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/operator-framework/rukpak/pkg/source"
)

var _ finalizer.Finalizer = &CleanupUnpackCache{}

const CleanupUnpackCacheKey = "core.rukpak.io/cleanup-unpack-cache"

type CleanupUnpackCache struct {
	Unpacker source.Unpacker
}

func (f CleanupUnpackCache) Finalize(ctx context.Context, obj client.Object) (finalizer.Result, error) {
	return finalizer.Result{}, f.Unpacker.Cleanup(ctx, obj.(*v1alpha2.BundleDeployment))
}
