package src

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/joomcode/errorx"
	"github.com/moby/buildkit/frontend/dockerfile/dockerfile2llb"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/sirupsen/logrus"
)

type BuildContext struct {
	state         *State
	layers        []distribution.Descriptor
	imageManifest dockerfile2llb.Image
}

func NewBuildContext(ctx context.Context, state *State, manifest *schema2.DeserializedManifest) (*BuildContext, error) {
	var imageManifest dockerfile2llb.Image
	blob, err := state.ReadBlob(ctx, manifest.Config)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(blob, &imageManifest); err != nil {
		return nil, err
	}
	for _, layer := range manifest.Layers {
		_, err := state.LayerTree(ctx, layer)
		if err != nil {
			return nil, err
		}
	}
	return &BuildContext{
		state:         state,
		layers:        manifest.Layers,
		imageManifest: imageManifest,
	}, nil
}

func (b *BuildContext) SaveForDocker(ctx context.Context, filename string, tag string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func() {
		if f != nil {
			_ = f.Close()
			f = nil
		}
	}()

	w := tar.NewWriter(f)

	if err := b.writeLayers(ctx, w); err != nil {
		return err
	}

	// Write image manifest
	hash, err := b.writeImageManifest(ctx, w)
	if err != nil {
		return nil
	}

	// Write repositories
	if err := b.writeManifest(ctx, w, tag, hash); err != nil {
		return nil
	}

	// Close tar
	if err := w.Close(); err != nil {
		return nil
	}
	// Close file
	if err := f.Close(); err != nil {
		f = nil
		return err
	}
	f = nil
	return nil
}

func (b *BuildContext) ApplyCommand(cmd instructions.Command) error {
	logrus.Infof("Apply command: %s", cmd)
	switch cmd := cmd.(type) {
	case *instructions.CopyCommand:
		// TODO:
		spew.Dump(cmd)
	case *instructions.EntrypointCommand:
		b.applyEntrypointCommand(cmd)
	case *instructions.EnvCommand:
		b.applyEnvCommand(cmd)
	case *instructions.LabelCommand:
		if b.imageManifest.Config.Labels == nil {
			b.imageManifest.Config.Labels = make(map[string]string)
		}
		for _, pair := range cmd.Labels {
			b.imageManifest.Config.Labels[pair.Key] = pair.Value
		}
	case *instructions.WorkdirCommand:
		b.imageManifest.Config.WorkingDir = cmd.Path
	default:
		logrus.Errorf("Unsupported command [%s]: %s", reflect.TypeOf(cmd), cmd)
	}
	return nil
}

func (b *BuildContext) writeImageManifest(ctx context.Context, w *tar.Writer) (string, error) {
	data, err := json.Marshal(b.imageManifest)
	if err != nil {
		return "", nil
	}
	sum256 := sha256.Sum256(data)
	hash := hex.EncodeToString(sum256[:])

	if err := w.WriteHeader(&tar.Header{
		Name:     hash + ".json",
		Size:     int64(len(data)),
		Mode:     0644,
		Typeflag: tar.TypeReg,
	}); err != nil {
		return "", err
	}

	if _, err := w.Write(data); err != nil {
		return "", err
	}

	return hash, nil
}

func (b *BuildContext) writeManifest(ctx context.Context, w *tar.Writer, tag string, hash string) error {
	type manifestItem struct {
		Config   string
		RepoTags []string
		Layers   []string
	}

	var layers []string

	for _, layer := range b.imageManifest.RootFS.DiffIDs {
		layers = append(layers, layer.Hex()+"/layer.tar")
	}

	items := []*manifestItem{
		{
			Config:   hash + ".json",
			RepoTags: []string{tag},
			Layers:   layers,
		},
	}

	data, err := json.Marshal(items)
	if err != nil {
		return err
	}

	if err := w.WriteHeader(&tar.Header{
		Name: "manifest.json",
		Size: int64(len(data)),
		Mode: 0644,
	}); err != nil {
		return err
	}

	if _, err := w.Write(data); err != nil {
		return err
	}

	return nil
}

func (b *BuildContext) writeLayers(ctx context.Context, w *tar.Writer) error {
	need := map[string]struct{}{}
	for _, layer := range b.imageManifest.RootFS.DiffIDs {
		need[layer.Hex()] = struct{}{}
	}
	for _, layer := range b.layers {
		diffID, err := b.writeLayer(ctx, w, layer)
		if err != nil {
			return nil
		}
		if _, ok := need[diffID]; !ok {
			return errorx.InternalError.New("Layer is not need for image: %s", layer.Digest)
		}
		delete(need, diffID)
	}
	for diffID := range need {
		return errorx.InternalError.New("Layer not exported: %v", diffID)
	}
	return nil
}

func (b *BuildContext) writeLayer(ctx context.Context, w *tar.Writer, layer distribution.Descriptor) (string, error) {
	hash, size, err := b.calcLayerHash(ctx, layer)
	if err != nil {
		return "", err
	}

	if err := w.WriteHeader(&tar.Header{
		Name:     hash + "/",
		Size:     size,
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}); err != nil {
		return "", err
	}

	if err := w.WriteHeader(&tar.Header{
		Name:     hash + "/layer.tar",
		Size:     size,
		Mode:     0644,
		Typeflag: tar.TypeReg,
	}); err != nil {
		return "", err
	}

	r, err := b.state.OpenBlob(ctx, layer)
	if err != nil {
		return "", err
	}
	defer r.Close()

	z, err := gzip.NewReader(r)
	if err != nil {
		return "", err
	}
	for {
		var data [64 * 1024]byte
		l, err := z.Read(data[:])
		if err != nil && err != io.EOF {
			return "", err
		}
		if _, werr := w.Write(data[:l]); werr != nil {
			return "", werr
		}
		if err == io.EOF {
			break
		}
	}
	return hash, nil
}

func (b *BuildContext) calcLayerHash(ctx context.Context, layer distribution.Descriptor) (string, int64, error) {
	r, err := b.state.OpenBlob(ctx, layer)
	if err != nil {
		return "", 0, err
	}
	defer r.Close()

	z, err := gzip.NewReader(r)
	if err != nil {
		return "", 0, err
	}
	var size int64
	sum256 := sha256.New()
	for {
		var data [64 * 1024]byte
		l, err := z.Read(data[:])
		if err != nil && err != io.EOF {
			return "", 0, err
		}
		size += int64(l)
		sum256.Write(data[:l])
		if err == io.EOF {
			break
		}
	}
	return hex.EncodeToString(sum256.Sum(nil)), size, nil
}

func (b *BuildContext) applyEnvCommand(cmd *instructions.EnvCommand) {
	keys := map[string]struct{}{}
	for _, pair := range cmd.Env {
		keys[pair.Key] = struct{}{}
	}
	config := &b.imageManifest.Config
	result := make([]string, 0, len(config.Env)+len(cmd.Env))
	for _, env := range config.Env {
		key := strings.Split(env, "=")[0]
		if _, ok := keys[key]; ok {
			continue
		}
		result = append(result, env)
	}
	for _, pair := range cmd.Env {
		result = append(result, pair.Key+"="+pair.Value)
	}
	config.Env = result
}

func (b *BuildContext) applyEntrypointCommand(cmd *instructions.EntrypointCommand) {
	var args []string = cmd.CmdLine
	if cmd.PrependShell {
		args = append(getShell(b.imageManifest.Config, b.imageManifest.OS), args...)
	}
	b.imageManifest.Config.Entrypoint = args
}

func getShell(config dockerfile2llb.ImageConfig, os string) []string {
	if len(config.Shell) == 0 {
		return append([]string{}, defaultShellForOS(os)[:]...)
	}
	return append([]string{}, config.Shell[:]...)
}

func defaultShellForOS(os string) []string {
	return []string{"/bin/sh", "-c"}
}
