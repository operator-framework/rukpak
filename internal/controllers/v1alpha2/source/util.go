package source

import (
	"fmt"
	"path/filepath"

	"github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/operator-framework/rukpak/internal/controllers/v1alpha2/controllers/utils"
	"github.com/spf13/afero"
)

// getCachedContentPath returns the name of the cached directory if exists.
func getCachedContentPath(bdaName string, bundleSrc *v1alpha2.BundleDeplopymentSource, base afero.Fs) (string, error) {
	cachedDirName := utils.GetCacheDirName(bdaName, *bundleSrc)

	if ok, err := afero.DirExists(base, filepath.Join(CacheDir, cachedDirName)); err != nil {
		return "", fmt.Errorf("error finding cache dir %v", err)
	} else if !ok {
		return "", nil
	}
	return cachedDirName, nil
}
