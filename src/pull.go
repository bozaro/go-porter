package src

import (
	"context"
	"io"
	"os"
	"path"
	"strconv"

	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/joomcode/errorx"
	bolt "go.etcd.io/bbolt"
)

var bucketManifest = []byte("manifest.v2")

func (s *State) Pull(ctx context.Context, imageName string, allowCached bool) (*schema2.DeserializedManifest, error) {
	info, err := s.ResolveImage(imageName)
	if err != nil {
		return nil, err
	}

	manifest, err := s.Manifest(ctx, info, allowCached)
	if err != nil {
		return nil, err
	}
	if _, err := s.DownloadBlob(ctx, info, manifest.Config); err != nil {
		return nil, err
	}
	for _, layer := range manifest.Layers {
		_, err := s.DownloadBlob(ctx, info, layer)
		if err != nil {
			return nil, err
		}
	}
	return manifest, nil
}

func (s *State) Manifest(ctx context.Context, imageInfo *ImageInfo, allowCached bool) (*schema2.DeserializedManifest, error) {
	var cached []byte
	if allowCached {
		if err := s.db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket(bucketManifest)
			if b == nil {
				return nil
			}
			cached = b.Get([]byte(imageInfo.Name))
			return nil
		}); err != nil {
			return nil, err
		}
	}
	if len(cached) > 0 {
		var manifest schema2.DeserializedManifest
		if err := manifest.UnmarshalJSON(cached); err == nil {
			return &manifest, nil
		}
	}

	hub, err := s.Registry(ctx, imageInfo)
	if err != nil {
		return nil, err
	}
	manifest, err := hub.ManifestV2(imageInfo.Repository, imageInfo.Reference)
	if err != nil {
		return nil, err
	}
	cached, err = manifest.MarshalJSON()
	if err != nil {
		return nil, err
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucketManifest)
		if err != nil {
			return err
		}
		return b.Put([]byte(imageInfo.Name), cached)
	}); err != nil {
		return nil, err
	}
	return manifest, err
}

func (s *State) DownloadBlob(ctx context.Context, imageInfo *ImageInfo, blob distribution.Descriptor) (string, error) {
	digest := blob.Digest
	target := path.Join(s.stateDir, digest.Algorithm().String(), digest.Hex()[0:2], digest.Hex()[2:])
	_ = os.MkdirAll(path.Dir(target), 0755)
	_, err := os.Stat(target)
	if err == nil {
		// Already downloaded
		return target, nil
	}
	if !os.IsNotExist(err) {
		return "", errorx.InternalError.Wrap(err, "can't get file state: %s", target)
	}

	hub, err := s.Registry(ctx, imageInfo)
	if err != nil {
		return "", err
	}

	reader, err := hub.DownloadBlob(imageInfo.Repository, digest)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	for i := 0; ; i++ {
		tmp := target + "~" + strconv.Itoa(i)
		f, err := os.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", errorx.InternalError.Wrap(err, "can't create temporary file: %s", tmp)
		}
		defer f.Close()
		if _, err := io.Copy(f, reader); err != nil {
			os.Remove(tmp)
			return "", errorx.InternalError.Wrap(err, "error on downloading blob: %s", digest)
		}
		if err := f.Close(); err != nil {
			os.Remove(tmp)
			return "", errorx.InternalError.Wrap(err, "error on closing file: %s", tmp)
		}
		if err := os.Rename(tmp, target); err != nil {
			os.Remove(tmp)
			return "", errorx.InternalError.Wrap(err, "error on rename file: %s -> %s", tmp, target)
		}
		return target, nil
	}
}
