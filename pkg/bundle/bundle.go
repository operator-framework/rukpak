package bundle

import (
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/operator-framework/rukpak/pkg/manifest"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(appsv1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(rbacv1.AddToScheme(scheme))
}

func newManifestFS(baseFS fs.FS) (manifest.FS, error) {
	const manifestDir = "manifests"
	subFS, err := fs.Sub(baseFS, manifestDir)
	if err != nil {
		return nil, err
	}
	fsys, err := manifest.NewFS(subFS)
	if err != nil {
		return nil, err
	}

	// verify the directory structure is flat pre the registry+v1 spec
	for path := range fsys {
		if len(filepath.SplitList(path)) > 2 {
			return nil, fmt.Errorf("manifest directory cannot have subdirectories, found: %q", filepath.Dir(path))
		}
	}

	return fsys, nil
}
