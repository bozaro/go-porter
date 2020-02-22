package src

import (
	"context"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func (s *State) Push(ctx context.Context, images ...string) error {
	infos := make([]name.Reference, 0, len(images))
	// Resolve images
	for _, image := range images {
		info, err := name.ParseReference(image)
		if err != nil {
			return err
		}
		infos = append(infos, info)
	}
	// Push manifests
	for _, image := range infos {
		manifest, err := s.LoadManifest(ctx, image)
		if err != nil {
			return err
		}

		if err := remote.Write(image, s.NewImage(ctx, manifest), s.RemoveOptions(image)...); err != nil {
			return err
		}
	}
	return nil
}
