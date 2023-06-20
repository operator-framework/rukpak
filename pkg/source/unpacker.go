package source

import (
	"context"
	"fmt"
	"io/fs"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Unpacker unpacks source content, either synchronously or asynchronously and
// returns a Result, which conveys information about the progress of unpacking
// the source content.
//
// If a Source unpacks content asynchronously, it should register one or more
// watches with a controller to ensure that sources referencing this source
// can be reconciled as progress updates are available.
//
// For asynchronous Sources, multiple calls to Unpack should be made until the
// returned result includes state StateUnpacked.
//
// NOTE: A source is meant to be agnostic to specific source formats and
// specifications. A source should treat a source root directory as an opaque
// file tree and delegate source format concerns to source parsers.
type Unpacker interface {
	Unpack(context.Context, *Source, client.Object) (*Result, error)
}

// Result conveys progress information about unpacking source content.
type Result struct {
	// FS contains the full filesystem of a source's root directory.
	FS fs.FS

	// ResolvedSource is a reproducible view of a Source.
	// When possible, source implementations should return a ResolvedSource
	// that pins the Source such that future fetches of the source content can
	// be guaranteed to fetch the exact same source content as the original
	// unpack.
	//
	// For example, resolved image sources should reference a container image
	// digest rather than an image tag, and git sources should reference a
	// commit hash rather than a branch or tag.
	ResolvedSource *Source

	// State is the current state of unpacking the source content.
	State State

	// Message is contextual information about the progress of unpacking the
	// source content.
	Message string
}

type State string

const (
	// StatePending conveys that a request for unpacking a source has been
	// acknowledged, but not yet started.
	StatePending State = "Pending"

	// StateUnpacking conveys that the source is currently unpacking a source.
	// This state should be used when the source contents are being downloaded
	// and processed.
	StateUnpacking State = "Unpacking"

	// StateUnpacked conveys that the source has been successfully unpacked.
	StateUnpacked State = "Unpacked"
)

type unpacker struct {
	sources map[SourceType]Unpacker
}

// NewUnpacker returns a new composite Source that unpacks sources using the source
// mapping provided by the configured sources.
func NewUnpacker(sources map[SourceType]Unpacker) Unpacker {
	return &unpacker{sources: sources}
}

func (s *unpacker) Unpack(ctx context.Context, src *Source, obj client.Object) (*Result, error) {
	source, ok := s.sources[src.Type]
	if !ok {
		return nil, fmt.Errorf("source type %q not supported", src.Type)
	}
	return source.Unpack(ctx, src, obj)
}
