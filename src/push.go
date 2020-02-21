package src

import (
	"context"
)

func (s *State) Push(ctx context.Context, images ...string) error {
	infos := make([]*ImageInfo, 0, len(images))
	// Resolve images
	for _, image := range images {
		info, err := s.ResolveImage(image)
		if err != nil {
			return err
		}
		infos = append(infos, info)
	}
	// Push manifests
	for _, info := range infos {
		manifest, err := s.LoadManifest(ctx, info)
		if err != nil {
			return err
		}

		hub, err := s.Registry(ctx, info)
		if err != nil {
			return err
		}
		for _, layer := range manifest.Layers {
			if err := func() error {
				exists, err := hub.HasBlob(info.Repository, layer.Digest)
				if err != nil {
					return err
				}
				if exists {
					return nil
				}

				r, err := s.OpenBlob(ctx, layer)
				if err != nil {
					return err
				}
				defer r.Close()

				if err := hub.UploadBlob(info.Repository, layer.Digest, r); err != nil {
					return err
				}
				return nil
			}(); err != nil {
				return err
			}
		}
		if err := hub.PutManifest(info.Repository, info.Reference, manifest); err != nil {
			return err
		}
	}
	return nil
}
