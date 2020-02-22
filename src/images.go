package src

import (
	"context"

	"github.com/docker/distribution/manifest/schema2"
	"github.com/google/go-containerregistry/pkg/name"
)

func (s *State) GetImages(ctx context.Context) (map[name.Reference]*schema2.DeserializedManifest, error) {
	images := make(map[name.Reference]*schema2.DeserializedManifest)
	if err := s.cacheForEach(bucketManifest, func(key string, value []byte) error {
		var manifest schema2.DeserializedManifest
		image, err := name.ParseReference(string(key))
		if err != nil {
			return err
		}
		if err := manifest.UnmarshalJSON(value); err != nil {
			return err
		}
		images[image] = &manifest
		return nil
	}); err != nil {
		return nil, err
	}
	return images, nil
}
