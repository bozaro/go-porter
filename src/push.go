package src

import (
	"context"
)

func (s *State) Push(ctx context.Context, images ...string) error {
	/*infos := make([]name.Reference, 0, len(images))
	// Resolve images
	for _, image := range images {
		info, err := name.ParseReference(image)
		if err != nil {
			return err
		}
		info, _ = name.ParseReference(info.Name())
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
		repository := info.Context().RepositoryStr()
		for _, layer := range manifest.Layers {
				if err := func() error {
				exists, err := hub.HasBlob(repository, layer.Digest)
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

				if err := hub.UploadBlob(repository, layer.Digest, r); err != nil {
					return err
				}
				return nil
			}(); err != nil {
				return err
			}
		}
		if err := hub.PutManifest(repository, info.Identifier(), manifest); err != nil {
			return err
		}
	}*/
	return nil
}
