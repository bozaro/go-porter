package src

import (
	"context"
	"io"

	"github.com/docker/distribution"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

type stateLayer struct {
	ctx        context.Context
	state      *State
	descriptor distribution.Descriptor
}

func (s *State) NewLayer(ctx context.Context, descriptor distribution.Descriptor) v1.Layer {
	return &stateLayer{
		ctx:        ctx,
		state:      s,
		descriptor: descriptor,
	}
}

func (l stateLayer) Digest() (v1.Hash, error) {
	return v1.NewHash(l.descriptor.Digest.String())
}

func (l stateLayer) DiffID() (v1.Hash, error) {
	panic("implement me")
}

func (l stateLayer) Compressed() (io.ReadCloser, error) {
	return l.state.OpenBlob(l.ctx, l.descriptor)
}

func (l stateLayer) Uncompressed() (io.ReadCloser, error) {
	panic("implement me")
}

func (l stateLayer) Size() (int64, error) {
	return l.descriptor.Size, nil
}

func (l stateLayer) MediaType() (types.MediaType, error) {
	return types.MediaType(l.descriptor.MediaType), nil
}
