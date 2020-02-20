package src

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"path"
	"sort"

	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/uuid"
	"github.com/joomcode/errorx"
	"github.com/klauspost/compress/gzip"
	"github.com/opencontainers/go-digest"
	bolt "go.etcd.io/bbolt"
)

var bucketUnpacked = []byte("unpacked.v2")

func (s *State) Save(ctx context.Context, w io.Writer, images ...string) error {
	infos := make([]*ImageInfo, 0, len(images))
	tags := make(map[string]digest.Digest)
	// Resolve images
	for _, image := range images {
		info, err := s.ResolveImage(image)
		if err != nil {
			return err
		}
		infos = append(infos, info)
	}
	// Get manifests
	manifests := make([]*schema2.DeserializedManifest, 0, len(infos))
	for _, info := range infos {
		manifest, err := s.Pull(ctx, info, true)
		if err != nil {
			return err
		}
		manifests = append(manifests, manifest)

		tags [info.Name] = manifest.Config.Digest
	}

	// Export layers
	t := tar.NewWriter(w)
	configs, err := s.writeLayers(ctx, t, manifests...)
	if err != nil {
		return err
	}

	// Export manifests
	if err := s.writeImageManifests(ctx, t, configs); err != nil {
		return err
	}

	imageTags := make(map[string]*DeserializedImageManifest)
	for tag, digest := range tags {
		imageTags[tag] = configs[digest]
	}
	if err := s.writeExportManifest(ctx, t, tags, configs); err != nil {
		return err
	}

	if err := t.Close(); err != nil {
		return err
	}
	return nil
}

func (s *State) writeExportManifest(ctx context.Context, w *tar.Writer, tags map[string]digest.Digest, configs map[digest.Digest]*DeserializedImageManifest) error {
	type manifestItem struct {
		Config   string
		RepoTags []string
		Layers   []string
	}

	exportImages := make(map[digest.Digest]*manifestItem)
	exportList := make([]*manifestItem, 0, len(configs))
	for tag, hash := range tags {
		exportImage := exportImages[hash]
		if exportImage == nil {
			config := configs[hash]
			if config == nil {
				return errorx.InternalError.New("can't find config for digest: %s", hash)
			}
			layers := make([]string, 0, len(config.RootFS.DiffIDs))
			for _, layer := range config.RootFS.DiffIDs {
				layers = append(layers, layer.Hex()+"/layer.tar")
			}
			exportImage = &manifestItem{
				Config: hash.Hex() + ".json",
				Layers: layers,
			}
			exportImages[hash] = exportImage
			exportList = append(exportList, exportImage)
		}
		exportImage.RepoTags = append(exportImage.RepoTags, tag)
	}
	for _, exportImage := range exportImages {
		sort.Strings(exportImage.RepoTags)
	}
	sort.Slice(exportList, func(i, j int) bool {
		return exportList[i].Config < exportList[j].Config
	})

	data, err := json.Marshal(exportList)
	if err != nil {
		return err
	}
	if err := w.WriteHeader(&tar.Header{
		Name:     "manifest.json",
		Size:     int64(len(data)),
		Typeflag: tar.TypeReg,
		Mode:     0644,
	}); err != nil {
		return err
	}

	if _, err := w.Write(data); err != nil {
		return err
	}
	return nil
}

func (s *State) writeLayers(ctx context.Context, w *tar.Writer, manifests ...*schema2.DeserializedManifest) (map[digest.Digest]*DeserializedImageManifest, error) {
	configs, err := s.loadImageManifests(ctx, manifests...)
	if err != nil {
		return nil, err
	}

	// Unpack layers
	layers := map[digest.Digest]distribution.Descriptor{}
	for _, manifest := range manifests {
		for _, layer := range manifest.Layers {
			unpacked, err := s.UnpackedLayer(ctx, layer)
			if err != nil {
				return nil, err
			}
			layers[unpacked.Digest] = *unpacked
		}
	}

	need := map[digest.Digest]struct{}{}
	var queue []digest.Digest
	for _, manifest := range configs {
		for _, layer := range manifest.RootFS.DiffIDs {
			if _, ok := need[layer]; ok {
				continue
			}
			need[layer] = struct{}{}
			queue = append(queue, layer)
		}
	}
	sort.Slice(queue, func(i, j int) bool {
		return queue[i] < queue[j]
	})
	for _, layer := range queue {
		unpacked, ok := layers[ layer]
		if !ok {
			return nil, errorx.IllegalState.New("can't find unpacked layer: %s", layer)
		}
		if err := s.writeLayer(ctx, w, unpacked); err != nil {
			return nil, err
		}
	}
	return configs, nil
}

func (s *State) loadImageManifests(ctx context.Context, manifests ...*schema2.DeserializedManifest) (map[digest.Digest]*DeserializedImageManifest, error) {
	result := map[digest.Digest]*DeserializedImageManifest{}
	for _, manifest := range manifests {
		if _, ok := result[manifest.Config.Digest]; ok {
			continue
		}

		blob, err := s.ReadBlob(ctx, manifest.Config)
		if err != nil {
			return nil, err
		}

		var imageManifest DeserializedImageManifest
		if err := json.Unmarshal(blob, &imageManifest); err != nil {
			return nil, err
		}
		result[manifest.Config.Digest] = &imageManifest
	}
	return result, nil
}

func (s *State) writeImageManifests(ctx context.Context, w *tar.Writer, configs map[digest.Digest]*DeserializedImageManifest) error {
	queue := make([]digest.Digest, 0, len(configs))
	for id := range configs {
		queue = append(queue, id)
	}
	sort.Slice(queue, func(i, j int) bool {
		return queue[i] < queue[j]
	})
	for _, id := range queue {
		blob, err := configs[id].Payload()
		if err != nil {
			return err
		}

		if err := w.WriteHeader(&tar.Header{
			Name:     id.Hex() + ".json",
			Size:     int64(len(blob)),
			Typeflag: tar.TypeReg,
			Mode:     0644,
		}); err != nil {
			return err
		}

		if _, err := w.Write(blob); err != nil {
			return err
		}
	}
	return nil
}

func (s *State) UnpackedLayer(ctx context.Context, layer distribution.Descriptor) (*distribution.Descriptor, error) {
	var cached []byte
	if err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketUnpacked)
		if b == nil {
			return nil
		}
		cached = b.Get([]byte(layer.Digest))
		return nil
	}); err != nil {
		return nil, err
	}

	var unpackedDesc *distribution.Descriptor
	if len(cached) > 0 {
		var desc distribution.Descriptor
		if err := json.Unmarshal(cached, &desc); err == nil {
			if stat, err := os.Stat(s.blobName(desc, "")); err == nil && !stat.IsDir() {
				unpackedDesc = &desc
			}
		}
	}

	if unpackedDesc == nil {
		tempFile := path.Join(s.stateDir, "~"+uuid.Generate().String()+".tar")
		defer os.Remove(tempFile)

		rf, err := os.Open(s.blobName(layer, ""))
		if err != nil {
			return nil, err
		}
		defer rf.Close()

		z, err := gzip.NewReader(rf)
		if err != nil {
			return nil, err
		}

		wf, err := os.Create(tempFile)
		hash := sha256.New()
		size, err := io.Copy(io.MultiWriter(wf, hash), z)
		if err != nil {
			return nil, err
		}

		sum256 := hash.Sum(nil)
		unpackedDesc = &distribution.Descriptor{
			MediaType: "application/vnd.docker.image.rootfs.diff.tar",
			Digest:    digest.NewDigestFromBytes(digest.SHA256, sum256[:]),
			Size:      size,
		}

		unpackedFile := s.blobName(*unpackedDesc, "")
		os.MkdirAll(path.Dir(unpackedFile), 0755)
		if err := os.Rename(tempFile, unpackedFile); err != nil {
			return nil, err
		}

		cached, err = json.Marshal(unpackedDesc)
		if err != nil {
			return nil, err
		}

		if err := s.db.Update(func(tx *bolt.Tx) error {
			b, err := tx.CreateBucketIfNotExists(bucketUnpacked)
			if err != nil {
				return err
			}
			return b.Put([]byte(layer.Digest), cached)
		}); err != nil {
			return nil, err
		}
	}
	return unpackedDesc, nil
}

func (s *State) writeLayer(ctx context.Context, w *tar.Writer, unpacked distribution.Descriptor) error {
	hash := unpacked.Digest.Hex()
	if err := w.WriteHeader(&tar.Header{
		Name:     hash + "/",
		Size:     0,
		Typeflag: tar.TypeDir,
		Mode:     0755,
	}); err != nil {
		return err
	}

	if err := w.WriteHeader(&tar.Header{
		Name:     hash + "/layer.tar",
		Size:     unpacked.Size,
		Typeflag: tar.TypeReg,
		Mode:     0644,
	}); err != nil {
		return err
	}

	r, err := s.OpenBlob(ctx, unpacked)
	if err != nil {
		return err
	}
	defer r.Close()

	if _, err := io.Copy(w, r); err != nil {
		return err
	}
	return nil
}
