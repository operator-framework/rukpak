package manifest

import (
	"io"
	"io/fs"
)

type dir struct {
	fileInfo
	path    string
	entries []fileInfo
	offset  int
}

func (d *dir) Stat() (fs.FileInfo, error) { return &d.fileInfo, nil }

func (d *dir) Close() error { return nil }

func (d *dir) Read(b []byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.path, Err: fs.ErrInvalid}
}

func (d *dir) ReadDir(count int) ([]fs.DirEntry, error) {
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
