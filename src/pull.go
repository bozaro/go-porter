package src

import (
	"context"
	"io"
	"os"
	"path"
	"strconv"

	"github.com/docker/distribution/manifest/schema1"
	"github.com/joomcode/errorx"
	"github.com/opencontainers/go-digest"
	bolt "go.etcd.io/bbolt"
)

var bucketManifest = []byte("manifest.v1")

func (s *State) Pull(ctx context.Context, allowCached bool, imageNames ...string) error {
	for _, imageName := range imageNames {
		info, err := s.ResolveImage(imageName)
		if err != nil {
			return err
		}

		manifest, err := s.Manifest(ctx, info, allowCached)
		if err != nil {
			return err
		}

		for _, layer := range manifest.FSLayers {
			_, err := s.DownloadBlob(ctx, info, layer.BlobSum)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *State) Manifest(ctx context.Context, imageInfo *ImageInfo, allowCached bool) (*schema1.SignedManifest, error) {
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
		var manifest schema1.SignedManifest
		if err := manifest.UnmarshalJSON(cached); err == nil {
			return &manifest, nil
		}
	}

	hub, err := s.Registry(ctx, imageInfo)
	if err != nil {
		return nil, err
	}

	manifest, err := hub.Manifest(imageInfo.Repository, "latest")
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

func (s *State) DownloadBlob(ctx context.Context, imageInfo *ImageInfo, digest digest.Digest) (string, error) {
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
