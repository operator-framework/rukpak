/*
Copyright 2023.

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

package util

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/spf13/afero"
)

func AferoFSToTarGZ(w io.Writer, fsys afero.Fs) error {
	gzw := gzip.NewWriter(w)
	tw := tar.NewWriter(gzw)

	if err := afero.Walk(fsys, ".", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.Mode().Type()&os.ModeSymlink != 0 {
			return nil
		}
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("get file info for %q: %v", path, err)
		}
		h.Uid = 0
		h.Gid = 0
		h.Uname = ""
		h.Gname = ""
		h.Name = path
		if err := tw.WriteHeader(h); err != nil {
			return fmt.Errorf("write tar header for %q: %v", path, err)
		}
		if info.IsDir() {
			return nil
		}
		f, err := fsys.Open(path)
		if err != nil {
			return fmt.Errorf("open file %q: %v", path, err)
		}
		if _, err := io.Copy(tw, f); err != nil {
			return fmt.Errorf("write tar data for %q: %v", path, err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("generate tar.gz from FS: %v", err)
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gzw.Close()
}
