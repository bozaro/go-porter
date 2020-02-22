package src

import (
	"context"

	"github.com/docker/distribution/manifest/schema2"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

type stateImage struct {
	ctx      context.Context
	state    *State
	manifest *schema2.DeserializedManifest
}

func (s *State) NewImage(ctx context.Context, manifest *schema2.DeserializedManifest) v1.Image {
	return &stateImage{
		ctx:      ctx,
		state:    s,
		manifest: manifest,
	}
}

func (s stateImage) Layers() ([]v1.Layer, error) {
	layers := make([]v1.Layer, 0, len(s.manifest.Layers))
	for _, layer := range s.manifest.Layers {
		layers = append(layers, s.state.NewLayer(s.ctx, layer))
	}
	return layers, nil
}

func (s stateImage) MediaType() (types.MediaType, error) {
	return types.MediaType(s.manifest.MediaType), nil
}

func (s stateImage) Size() (int64, error) {
	panic("implement me")
}

func (s stateImage) ConfigName() (v1.Hash, error) {
	panic("implement me")
}

func (s stateImage) ConfigFile() (*v1.ConfigFile, error) {
	panic("implement me")
}

func (s stateImage) RawConfigFile() ([]byte, error) {
	return s.state.ReadBlob(s.ctx, s.manifest.Config)
}

func (s stateImage) Digest() (v1.Hash, error) {
	panic("implement me")
}

func (s stateImage) Manifest() (*v1.Manifest, error) {
	panic("implement me")
}

func (s stateImage) RawManifest() ([]byte, error) {
	return s.manifest.MarshalJSON()
}

func (s stateImage) LayerByDigest(v1.Hash) (v1.Layer, error) {
	panic("implement me")
}

func (s stateImage) LayerByDiffID(v1.Hash) (v1.Layer, error) {
	panic("implement me")
}
