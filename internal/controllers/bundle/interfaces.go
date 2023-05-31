package bundle

import (
	"context"
	"io/fs"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

type Handler interface {
	Handle(context.Context, fs.FS, *rukpakv1alpha1.Bundle) (fs.FS, error)
}

type HandlerFunc func(context.Context, fs.FS, *rukpakv1alpha1.Bundle) (fs.FS, error)

func (f HandlerFunc) Handle(ctx context.Context, fsys fs.FS, b *rukpakv1alpha1.Bundle) (fs.FS, error) {
	return f(ctx, fsys, b)
}
