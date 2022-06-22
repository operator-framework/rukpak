package bundle

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// ObjectFile represents a manifest flie that contains one or more Kubernetes objects.
//
// The objects will be stored as type `T`.
// If `T` is an interface type, the objects can be cast to their specific structure type.
type ObjectFile[T runtime.Object] struct {
	Objects []T

	fs.File
	data io.Reader
}

// NewObjectFile parses the kubernetes objects contained in the file according to the scheme.
// If `strict` is enabled, object types the scheme is not aware of will throw an error.
func NewObjectFile[T runtime.Object](f fs.File, scheme *runtime.Scheme, strict bool) (*ObjectFile[T], error) {
	var (
		file           = ObjectFile[T]{File: f, data: f}
		r    io.Reader = f
	)

	// If the file can't be seeked in, copy its bytes so the file can be read to
	// parse the objects and again by the consumer of the file.
	if _, ok := f.(io.Seeker); !ok {
		var buf bytes.Buffer
		r = io.TeeReader(f, &buf)
		file.data = &buf
	}
	objs, err := parseObjects[T](r, scheme, strict)
	if err != nil {
		return nil, err
	}
	file.Objects = objs

	// If the file was a seeker, seek back to the start
	if seeker, ok := file.data.(io.Seeker); ok {
		if _, err := seeker.Seek(0, 0); err != nil {
			return nil, err
		}
	}

	return &file, nil
}

// Read bytes from the file.
func (f ObjectFile[T]) Read(p []byte) (int, error) {
	return f.data.Read(p)
}

func parseObjects[T runtime.Object](r io.Reader, scheme *runtime.Scheme, strict bool) ([]T, error) {
	objs := make([]T, 0, 1)
	dec := yaml.NewYAMLOrJSONDecoder(r, 1024)
	for {
		unstructuredObj := new(unstructured.Unstructured)
		if err := dec.Decode(unstructuredObj); errors.Is(err, io.EOF) {
			return objs, nil
		} else if err != nil {
			return nil, err
		}

		// When the object type is not recognized, store the unstructured object
		if !scheme.Recognizes(unstructuredObj.GroupVersionKind()) {
			if strict {
				return nil, fmt.Errorf("unrecognized object type: %s", unstructuredObj.GroupVersionKind().String())
			}

			// TODO(ryantking): this will blow up if you try read a file of an
			// unrecognized type typed with a different concrete type
			objs = append(objs, runtime.Object(unstructuredObj).(T))
			continue
		}

		// When the object is recognized, convert it so its typed
		obj, err := scheme.New(unstructuredObj.GroupVersionKind())
		if err != nil {
			return nil, err
		}
		if err := scheme.Convert(unstructuredObj, obj, nil); err != nil {
			return nil, err
		}
		obj.GetObjectKind().SetGroupVersionKind(unstructuredObj.GroupVersionKind())
		objs = append(objs, obj.(T)) // TODO(ryantking): This can probably be finagled to blow up
	}
}
