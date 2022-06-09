package testutil

import (
	"embed"
	"io/fs"
	"strings"
	"testing/fstest"
)

//go:embed testdata/*
var testDataFS embed.FS

// NewRegistryV1FS returns a new test filesystem that contains a valid registry+v1 bundle.
func NewRegistryV1FS() fstest.MapFS {
	fsys, err := makeMapFS("testdata/registry-v1/")
	if err != nil {
		panic(err)
	}

	return fsys
}

// NewPlainV0FS returns a new test filesystem that contains a valid plain+v0 bundle.
func NewPlainV0FS() fstest.MapFS {
	fsys, err := makeMapFS("testdata/plain-v0/")
	if err != nil {
		panic(err)
	}

	return fsys
}

// embed.FS is only walkable from the root directory so this function accepts a prefix to filter out unwanted files.
// This looks like it could be a go bug if its not explicitly called out.
func makeMapFS(prefix string) (fstest.MapFS, error) {
	fsys := make(fstest.MapFS)
	if err := fs.WalkDir(testDataFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || !strings.HasPrefix(path, prefix) || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		b, err := fs.ReadFile(testDataFS, path)
		if err != nil {
			return err
		}

		fsys[strings.TrimPrefix(path, prefix)] = &fstest.MapFile{
			Data:    b,
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
			Sys:     info.Sys(),
		}
		return nil
	}); err != nil {
		panic(err)
	}

	return fsys, nil
}
