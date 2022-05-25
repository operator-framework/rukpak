package bundle

import (
	"bytes"
	"io"
	"io/fs"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var _ fs.File = openManifestFile{}

// ManifestFile represents a single file in a bundle.
type ManifestFile struct {
	Objs    []client.Object
	Mode    fs.FileMode
	ModTime time.Time
}

type manifestFileInfo struct {
	ManifestFile
	name string
}

func (i *manifestFileInfo) Name() string               { return i.name }
func (i *manifestFileInfo) Size() int64                { return 0 } // TODO(ryantking): Calculate this
func (i *manifestFileInfo) Mode() fs.FileMode          { return i.Mode() }
func (i *manifestFileInfo) Type() fs.FileMode          { return i.Mode().Type() }
func (i *manifestFileInfo) ModTime() time.Time         { return i.ManifestFile.ModTime }
func (i *manifestFileInfo) IsDir() bool                { return i.Mode()&fs.ModeDir != 0 }
func (i *manifestFileInfo) Sys() interface{}           { return nil }
func (i *manifestFileInfo) Info() (fs.FileInfo, error) { return i, nil }

type openManifestFile struct {
	manifestFileInfo
	path      string
	r         io.Reader
	objNdx    int
	remaining int
}

func (f openManifestFile) Stat() (fs.FileInfo, error) {
	return &f.manifestFileInfo, nil
}

func (f openManifestFile) Close() error {
	return nil
}

func (f openManifestFile) Read(b []byte) (int, error) {
	if f.remaining > len(b) || f.objNdx == len(f.Objs) {
		// We have enough bytes to fill the buffer or are at our last object
		return f.read(b)
	}

	// Marshal the next object and queue it up to be read
	obj := f.Objs[f.objNdx]
	f.objNdx++
	b, err := yaml.Marshal(obj)
	if err != nil {
		return 0, err
	}
	f.remaining += len(b)
	f.r = io.MultiReader(f.r, bytes.NewReader(b))
	return f.read(b)
}

func (f openManifestFile) Seek(offset int64, whence int) (int64, error) {
	// TODO(ryantking): What behavior do we want here? Should offset be the object itnex instead of byte index?
	return 0, fs.ErrInvalid
}

func (f openManifestFile) ReadAt(b []byte, offset int64) (int, error) {
	// TODO(ryantking): What behavior do we want here? Should offset be the object itnex instead of byte index?
	return 0, fs.ErrInvalid
}

func (f openManifestFile) read(b []byte) (int, error) {
	n, err := f.r.Read(b)
	if err != nil {
		return 0, err
	}
	f.remaining -= n
	return n, nil
}

type manifestDir struct {
	manifestFileInfo
	path    string
	entries []manifestFileInfo
	offset  int
}

func (d *manifestDir) Stat() (fs.FileInfo, error) { return &d.manifestFileInfo, nil }
func (d *manifestDir) Close() error               { return nil }
func (d *manifestDir) Read(b []byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.path, Err: fs.ErrInvalid}
}

func (d *manifestDir) ReadDir(count int) ([]fs.DirEntry, error) {
	n := len(d.entries) - d.offset
	if n == 0 && count > 0 {
		return nil, io.EOF
	}
	if count > 0 && n > count {
		n = count
	}
	list := make([]fs.DirEntry, n)
	for i := range list {
		list[i] = &d.entries[d.offset+i]
	}
	d.offset += n
	return list, nil
}
