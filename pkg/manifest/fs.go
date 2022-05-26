package manifest

import (
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FS is a File System interface for Kubernetes manifests stored in a hierarchy.
//
// Files are stored in directories like a traditiional filesystem, but each file
// is backed by zero or more Kubernetes objects.
//
// The objects themselves can be cast to their specific type.
// TODO(ryantking): Should we support any other file system abstractions beyond `fs.FS`?
type FS map[string]File

// NewFS returns a new FS that has loaded in all the Kubernetes manifests found in the provided base filesystem.
// This function starts the search at the root of the filesystem and will error if it finds any non-manifest files.
func NewFS(baseFS fs.FS) (FS, error) {
	fsys := make(FS)
	if err := fs.WalkDir(baseFS, "/", func(path string, d fs.DirEntry, err error) error {
		// Directories don't need to be stored.
		// The FS/File operations know how to detect one exists based on paths.
		if err != nil || d.IsDir() {
			return err
		}

		return fsys.slurpFile(baseFS, d.Name())
	}); err != nil {
		return nil, err
	}

	return fsys, nil
}

func (fsys FS) slurpFile(baseFS fs.FS, path string) error {
	f, err := baseFS.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	objs, err := fsys.parseObjects(f)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		return err
	}

	fsys[path] = File{ModTime: info.ModTime(), Objects: objs}
	return nil
}

func (fsys FS) parseObjects(r io.Reader) ([]client.Object, error) {
	objs := make([]client.Object, 0, 1)
	dec := yaml.NewYAMLOrJSONDecoder(r, 1024)
	for {
		var unstructuredObj unstructured.Unstructured
		if err := dec.Decode(&unstructuredObj); errors.Is(err, io.EOF) {
			return objs, nil
		} else if err != nil {
			return nil, err
		}
		obj, err := scheme.New(unstructuredObj.GroupVersionKind())
		if err != nil {
			return nil, err
		}
		if err := scheme.Convert(unstructuredObj, &obj, nil); err != nil {
			return nil, err
		}
		objs = append(objs, obj.(client.Object))
	}
}

// Open returns a file pointing at the manifest identified by the filepath.
// Code adapetd from the `fstest.MapFS` `Open` function.
// https://cs.opensource.google/go/go/+/refs/tags/go1.18.2:src/testing/fstest/mapfs.go;l=47
func (fsys FS) Open(name string) (fs.File, error) {
	file, fileExists := fsys[name]
	if fileExists && !file.IsDirectory {
		// Normal file
		return &openFile{fileInfo: fileInfo{filepath.Base(name), file}, path: name}, nil
	}

	var list []fileInfo
	var elem string
	need := make(map[string]bool)
	if name == "." {
		elem = "."
		for fname, f := range fsys {
			i := strings.Index(fname, "/")
			if i < 0 {
				if fname != "." {
					list = append(list, fileInfo{fname, f})
				}
			} else {
				need[fname[:i]] = true
			}
		}
	} else {
		elem = name[strings.LastIndex(name, "/")+1:]
		prefix := name + "/"
		for fname, f := range fsys {
			if strings.HasPrefix(fname, prefix) {
				felem := fname[len(prefix):]
				i := strings.Index(felem, "/")
				if i < 0 {
					list = append(list, fileInfo{felem, f})
				} else {
					need[fname[len(prefix):len(prefix)+i]] = true
				}
			}
		}
		if !fileExists && list == nil && len(need) == 0 {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
	}
	for _, fi := range list {
		delete(need, fi.name)
	}
	for name := range need {
		list = append(list, fileInfo{name, File{IsDirectory: true}})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].name < list[j].name
	})

	if !fileExists {
		file = File{IsDirectory: true}
	}
	return &dir{fileInfo{elem, file}, name, list, 0}, nil
}
