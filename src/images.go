package src

import (
	"context"

	"github.com/docker/distribution/manifest/schema2"
	"github.com/google/go-containerregistry/pkg/name"
	bolt "go.etcd.io/bbolt"
)

func (s *State) GetImages(ctx context.Context) (map[name.Reference]*schema2.DeserializedManifest, error) {
	images := make(map[name.Reference]*schema2.DeserializedManifest)
	if err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketManifest)
		if b == nil {
			return nil
		}
		return b.ForEach(func(key []byte, value []byte) error {
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
		})
	}); err != nil {
		return nil, err
	}
	return images, nil
}
