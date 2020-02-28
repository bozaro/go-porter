package src

import (
	"context"
	"io/ioutil"
	"os"
	"path"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sirupsen/logrus"
)

func (s *State) Remove(ctx context.Context, images ...string) error {
	infos := make([]name.Reference, 0, len(images))
	// Resolve images
	for _, image := range images {
		info, err := name.ParseReference(image)
		if err != nil {
			return err
		}
		if err := s.cacheRemove(bucketManifest, info.Name()); err != nil && !os.IsNotExist(err) {
			return err
		}
		infos = append(infos, info)
	}

	files, err := s.findAllBlobFiles(ctx)
	if err != nil {
		return err
	}

	manifests, err := s.GetImages(ctx)
	if err != nil {
		return err
	}

	used := map[string]struct{}{}
	for image, manifest := range manifests {
		cacheFile := s.cacheFile(bucketManifest, image.Name())
		if _, ok := used [cacheFile]; ok {
			continue
		}
		used[cacheFile] = struct{}{}

		configBlob := s.blobName(manifest.Config, "")
		if _, ok := used[configBlob]; !ok {
			used[configBlob] = struct{}{}
		}

		for _, layer := range manifest.Layers {
			layerBlob := s.blobName(layer, "")
			if _, ok := used[layerBlob]; !ok {
				used[layerBlob] = struct{}{}

				unpacked, err := s.UnpackedLayer(ctx, layer)
				if err != nil {
					return err
				}
				if unpacked != nil {
					used[s.blobName(*unpacked, "")] = struct{}{}
				}
			}

			layerTree := s.blobName(layer, ".tree")
			if _, ok := used[layerTree]; !ok {
				used[layerTree] = struct{}{}
			}
		}
	}

	for _, file := range files {
		if _, ok := used[file]; ok {
			logrus.Debugf("%s - keep", file)
			continue
		}
		logrus.Infof("%s - remove", file)
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (s *State) findAllBlobFiles(ctx context.Context) ([]string, error) {
	queue := make([]string, 0, 1024)
	queue = append(queue, s.stateDir)

	result := make([]string, 0, 1024)
	for len(queue) > 0 {
		next := make([]string, 0, 1024)
		for _, dir := range queue {
			list, err := ioutil.ReadDir(dir)
			if err != nil {
				return nil, err
			}
			for _, item := range list {
				if item.IsDir() {
					next = append(next, path.Join(dir, item.Name()))
					continue
				}
				result = append(result, path.Join(dir, item.Name()))
			}
		}
		queue = next
	}
	return result, nil
}
