package manifest

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// File represents a list of zero or more Kubernetes manifests from a named file.
type File struct {
	// The kubernetes manifests. These can be cast to their typed representations.
	Objects []client.Object

	// Whether or not the file is a directory
	IsDirectory bool

	// When the file the manifests are from was last modified.
	ModTime time.Time
}

// This and the related structs are largely adapted from `fstest.MapFile`
// https://cs.opensource.google/go/go/+/refs/tags/go1.18.2:src/testing/fstest/mapfs.go
type fileInfo struct {
	name string
	file File
}

func (i *fileInfo) Name() string               { return i.name }
func (i *fileInfo) Size() int64                { return int64(len(i.file.Objects)) } // TODO(ryantking): Is this okay?
func (i *fileInfo) Type() fs.FileMode          { return i.Mode().Type() }            // TODO(ryantking): Verify this
func (i *fileInfo) ModTime() time.Time         { return i.file.ModTime }
func (i *fileInfo) IsDir() bool                { return i.file.IsDirectory }
func (i *fileInfo) Sys() interface{}           { return nil } // TODO(ryantking): Capture and store this somewhere
func (i *fileInfo) Info() (fs.FileInfo, error) { return i, nil }

func (i *fileInfo) Mode() fs.FileMode {
	if i.file.IsDirectory {
		return os.ModeDir
	}

	return os.ModePerm // TODO(ryantking): Verify this
}

type openFile struct {
	fileInfo
	path           string
	r              io.Reader
	objNdx         int
	remainingBytes int
}

func (f openFile) Stat() (fs.FileInfo, error) {
	return &f.fileInfo, nil
}

func (f openFile) Close() error {
	return nil
}

// Read returns kubernetes manifest as YAML.
// It lazily marshalls its manifests as it needs to so all object are read in a continuous stream.
// TODO(ryantking): Check if we need to manually insert ----- between objects
func (f *openFile) Read(b []byte) (int, error) {
	// If we have fewer remaining bytes than the capacity of the buffer and more objects to read, prepare it for reading.
	if f.remainingBytes < cap(b) && f.objNdx < len(f.file.Objects) {
		if err := f.nextObject(); err != nil {
			return 0, err
		}
	}

	return f.read(b)
}

// ErrObjectOffset is thrown when an offset is requested that is larger than the number of available objects.
var ErrObjectOffset = errors.New("object offset is greater than the number of objects")

// Seek moves the file to the object with the given offset.
// The whence value is ignored. TODO(ryantking): Can we give it a purpose?
// TODO(ryantking): Is this okay? Does this drift too far from the FS semantics to be a good API?
func (f *openFile) Seek(offset int64, whence int) (int64, error) {
	if offset >= int64(len(f.file.Objects)) {
		return 0, &fs.PathError{Op: "readAt", Path: f.path, Err: errors.New("object offset out of range")}
	}

	// Set the index, zero out the remaining bytes, and prepare the next object
	f.objNdx = int(offset)
	f.remainingBytes = 0
	return 0, f.nextObject() // TODO(ryantking): Should the first return value have a purpose?
}

// ReadAt reads starting at the object with the offset number.
// TODO(ryantking): Is this okay? Does this drift too far from the FS semantics to be a good API?
func (f *openFile) ReadAt(b []byte, offset int64) (int, error) {
	if _, err := f.Seek(offset, 0); err != nil {
		return 0, err
	}

	return f.read(b)
}

// read some bytes and then decrement our tracker
func (f *openFile) read(b []byte) (int, error) {
	n, err := f.r.Read(b)
	if err != nil {
		return 0, err
	}
	f.remainingBytes -= n
	return n, nil
}

// nextObject prepares the nextObject to be read.
func (f *openFile) nextObject() error {
	obj := f.file.Objects[f.objNdx]
	f.objNdx++
	b, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}
	f.remainingBytes += len(b)
	// nesting MultiReaders is okay since the function optimizes for that usecase.
	f.r = io.MultiReader(f.r, bytes.NewReader(b))
	return nil
}
