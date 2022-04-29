package storage

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nlepage/go-tarfs"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ Storage = &LocalDirectory{}

const DefaultBundleCacheDir = "/var/cache/bundles"

var DefaultLocalDirectory = &LocalDirectory{RootDirectory: DefaultBundleCacheDir}

type LocalDirectory struct {
	RootDirectory string
}

func (s *LocalDirectory) Load(_ context.Context, owner client.Object) (fs.FS, error) {
	bundlePath := filepath.Join(s.RootDirectory, fmt.Sprintf("%s.tgz", owner.GetUID()))
	bundleFile, err := os.Open(bundlePath)
	if err != nil {
		return nil, err
	}
	defer bundleFile.Close()
	tarReader, err := gzip.NewReader(bundleFile)
	if err != nil {
		return nil, err
	}
	return tarfs.New(tarReader)
}

func (s *LocalDirectory) Store(_ context.Context, owner client.Object, bundle fs.FS) error {
	buf := &bytes.Buffer{}
	gzw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gzw)
	if err := fs.WalkDir(bundle, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("get file info for %q: %w", path, err)
		}

		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("build tar file info header for %q: %w", path, err)
		}
		h.Uid = 0
		h.Gid = 0
		h.Uname = ""
		h.Gname = ""
		h.Name = path

		if err := tw.WriteHeader(h); err != nil {
			return fmt.Errorf("write tar header for %q: %w", path, err)
		}
		if d.IsDir() {
			return nil
		}
		f, err := bundle.Open(path)
		if err != nil {
			return fmt.Errorf("open file %q: %w", path, err)
		}
		if _, err := io.Copy(tw, f); err != nil {
			return fmt.Errorf("write tar data for %q: %w", path, err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("generate tar.gz for bundle %q: %w", owner.GetName(), err)
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gzw.Close(); err != nil {
		return err
	}

	bundlePath := filepath.Join(s.RootDirectory, fmt.Sprintf("%s.tgz", owner.GetUID()))
	bundleFile, err := os.Create(bundlePath)
	if err != nil {
		return err
	}
	defer bundleFile.Close()

	if _, err := io.Copy(bundleFile, buf); err != nil {
		return err
	}
	return nil
}

func (s *LocalDirectory) Delete(_ context.Context, owner client.Object) error {
	bundlePath := filepath.Join(s.RootDirectory, fmt.Sprintf("%s.tgz", owner.GetUID()))
	return ignoreNotExist(os.Remove(bundlePath))
}

func ignoreNotExist(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
