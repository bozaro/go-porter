package src

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/blang/vfs"
	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/joomcode/errorx"
)

var bucketManifest = "manifest.v1"

func (s *State) Pull(ctx context.Context, image name.Reference, allowCached bool) (*schema2.DeserializedManifest, error) {
	manifest, err := s.PullManifest(ctx, image, allowCached)
	if err != nil {
		return nil, err
	}
	if _, err := s.DownloadBlob(ctx, image, manifest.Config); err != nil {
		return nil, err
	}
	for _, layer := range manifest.Layers {
		_, err := s.DownloadBlob(ctx, image, layer)
		if err != nil {
			return nil, err
		}
	}
	return manifest, nil
}

func (s *State) PullManifest(ctx context.Context, image name.Reference, allowCached bool) (*schema2.DeserializedManifest, error) {
	if allowCached {
		cached, err := s.LoadManifest(ctx, image)
		if err != nil {
			return nil, err
		}
		if cached != nil {
			return cached, nil
		}
	}

	desc, err := remote.Image(image, s.RemoveOptions(image)...)
	if err != nil {
		return nil, err
	}

	var manifest schema2.DeserializedManifest
	rawManifest, err := desc.RawManifest()
	if err != nil {
		return nil, err
	}
	if err := manifest.UnmarshalJSON(rawManifest); err != nil {
		return nil, err
	}
	if err := s.SaveManifest(ctx, &manifest, image); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func (s *State) LoadManifest(ctx context.Context, image name.Reference) (*schema2.DeserializedManifest, error) {
	var cached []byte
	cached, found, err := s.cacheLoad(bucketManifest, image.Name())
	if err != nil || !found {
		return nil, err
	}
	var manifest schema2.DeserializedManifest
	if err := manifest.UnmarshalJSON(cached); err == nil {
		return &manifest, nil
	}
	return nil, nil
}

func (s *State) SaveManifest(ctx context.Context, manifest *schema2.DeserializedManifest, image name.Reference) error {
	cached, err := manifest.MarshalJSON()
	if err != nil {
		return err
	}
	return s.cacheSave(bucketManifest, image.Name(), cached)
}

func (s *State) blobName(blob distribution.Descriptor, suffix string) string {
	digest := blob.Digest
	prefix := path.Join(digest.Algorithm().String(), digest.Hex()[0:2], digest.Hex()[2:])
	if suffix == "" {
		suffix = s.mediaTypeSuffix(blob.MediaType)
	}
	return prefix + suffix
}

func (s *State) mediaTypeSuffix(mediaType string) string {
	switch mediaType {
	case "application/vnd.docker.container.image.v1+json":
		return ".json"
	case "application/vnd.docker.image.rootfs.diff.tar":
		return ".tar"
	case "application/vnd.docker.image.rootfs.diff.tar.gzip":
		return ".tar.gz"
	default:
		fmt.Println(mediaType)
		return ".bin"
	}
}

func (s *State) OpenBlob(ctx context.Context, blob distribution.Descriptor) (io.ReadCloser, error) {
	return vfs.Open(s.stateVfs, s.blobName(blob, ""))
}

func (s *State) ReadBlob(ctx context.Context, blob distribution.Descriptor) ([]byte, error) {
	return vfs.ReadFile(s.stateVfs, s.blobName(blob, ""))
}

func (s *State) DownloadBlob(ctx context.Context, image name.Reference, blob distribution.Descriptor) (string, error) {
	filename := s.blobName(blob, "")
	digest := blob.Digest
	_ = vfs.MkdirAll(s.stateVfs, path.Dir(filename), 0755)
	_, err := s.stateVfs.Stat(filename)
	if err == nil {
		// Already downloaded
		return filename, nil
	}
	if !os.IsNotExist(err) {
		return "", errorx.InternalError.Wrap(err, "can't get file state: %s", filename)
	}

	layerDigest := image.Context().Digest(blob.Digest.String())
	layer, err := remote.Layer(layerDigest, s.RemoveOptions(image)...)
	if err != nil {
		return "", err
	}

	reader, err := layer.Compressed()
	if err != nil {
		return "", err
	}
	defer reader.Close()

	if err := safeWrite(s.stateVfs, filename, func(w io.Writer) error {
		if _, err := io.Copy(w, reader); err != nil {
			return errorx.InternalError.Wrap(err, "error on downloading blob: %s", digest)
		}
		return nil
	}); err != nil {
		return "", err
	}
	return filename, nil
}
