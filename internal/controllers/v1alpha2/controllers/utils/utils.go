package utils

import (
	"encoding/base64"
	"fmt"

	"github.com/operator-framework/rukpak/api/v1alpha2"
)

// Perform a base64 encoding to get the directoryName to store caches
func GetCacheDirName(bdName string, bd v1alpha2.BundleDeplopymentSource) string {
	switch bd.Kind {
	case v1alpha2.SourceTypeImage:
		return encode(bdName, string(bd.Kind), bd.Image.ImageRef)
	case v1alpha2.SourceTypeGit:
		return encode(bdName, string(bd.Kind), bd.Git.Repository)
	case v1alpha2.SourceTypeHTTP:
		return encode(bdName, string(bd.Kind), bd.HTTP.URL)
	default:
		return ""
	}
}

func encode(str1, str2, str3 string) string {
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s-%s-%s", str1, str2, str3)))
}
