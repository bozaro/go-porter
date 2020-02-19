package src

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/uuid"
	"github.com/docker/go-units"
	"github.com/joomcode/errorx"
	"github.com/klauspost/compress/gzip"
	"github.com/moby/buildkit/frontend/dockerfile/dockerfile2llb"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
	"time"
)

type BuildContext struct {
	state         *State
	fs            FS
	contextPath   string
	layers        []distribution.Descriptor
	imageManifest dockerfile2llb.Image
}

func NewBuildContext(ctx context.Context, state *State, manifest *schema2.DeserializedManifest, contextPath string) (*BuildContext, error) {
	var imageManifest dockerfile2llb.Image
	blob, err := state.ReadBlob(ctx, manifest.Config)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(blob, &imageManifest); err != nil {
		return nil, err
	}
	root := state.EmptyLayer()
	for _, layer := range manifest.Layers {
		fsdiff, err := state.LayerTree(ctx, layer)
		if err != nil {
			return nil, err
		}
		root.ApplyDiff(fsdiff)
	}
	return &BuildContext{
		state:       state,
		contextPath: contextPath,
		fs: FS{
			Base: root,
		},
		layers:        manifest.Layers,
		imageManifest: imageManifest,
	}, nil
}

func (b *BuildContext) SaveForDocker(ctx context.Context, filename string, tag string) error {
	if err := b.FlushDelta(ctx); err != nil {
		return nil
	}

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
		return b.applyCopyCommand(cmd)
	case *instructions.EntrypointCommand:
		b.applyEntrypointCommand(cmd)
	case *instructions.EnvCommand:
		b.applyEnvCommand(cmd)
	case *instructions.HealthCheckCommand:
		b.imageManifest.Config.Healthcheck = &dockerfile2llb.HealthConfig{
			Test:        cmd.Health.Test,
			Interval:    cmd.Health.Interval,
			Timeout:     cmd.Health.Timeout,
			StartPeriod: cmd.Health.StartPeriod,
			Retries:     cmd.Health.Retries,
		}
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

func (b *BuildContext) FlushDelta(ctx context.Context) error {
	if b.fs.Delta == nil || len(b.fs.Delta.Child) == 0 {
		return nil
	}
	t := time.Now()
	logrus.Info("flushing layer...")
	digest, layer, err := b.writeDeltaLayer(ctx)
	if err != nil {
		return err
	}

	b.imageManifest.RootFS.DiffIDs = append(b.imageManifest.RootFS.DiffIDs, digest)
	b.layers = append(b.layers, *layer)
	b.fs.Delta = nil
	logrus.Infof("layer flushed: %s, %s, %v", layer.Digest, units.HumanSize(float64(layer.Size)), time.Now().Sub(t))
	return nil
}

func (b *BuildContext) fileSHA256(ctx context.Context, file string) (digest.Digest, error) {
	hash := sha256.New()
	f, err := os.Open(file)
	if err != nil {
		return digest.Digest(""), err
	}
	defer f.Close()
	if _, err := io.Copy(hash, f); err != nil {
		return digest.Digest(""), err
	}
	return digest.NewDigest(digest.SHA256, hash), nil
}

func (b *BuildContext) writeDeltaLayer(ctx context.Context) (digest.Digest, *distribution.Descriptor, error) {
	tempFile := path.Join(b.state.stateDir, "~"+uuid.Generate().String()+".tar.gz")
	hashTr := sha256.New()
	hashGz := sha256.New()

	defer os.Remove(tempFile)

	f, err := os.Create(tempFile)
	if err != nil {
		return "", nil, err
	}
	defer f.Close()

	gz, err := gzip.NewWriterLevel(io.MultiWriter(f, hashGz), gzip.DefaultCompression)
	if err != nil {
		return "", nil, err
	}

	t := tar.NewWriter(io.MultiWriter(gz, hashTr))
	if err := b.writeDir(t, b.fs.Delta); err != nil {
		return "", nil, err
	}
	if err := t.Close(); err != nil {
		return "", nil, err
	}
	if err := gz.Close(); err != nil {
		return "", nil, err
	}
	size, err := f.Seek(0, 1)
	if err != nil {
		return "", nil, err
	}

	desc := distribution.Descriptor{
		MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
		Size:      size,
		Digest:    digest.NewDigestFromBytes(digest.SHA256, hashGz.Sum(nil)),
	}
	target := b.state.blobName(desc, "")
	_ = os.MkdirAll(path.Dir(target), 0755)
	if err := os.Rename(tempFile, target); err != nil {
		return "", nil, err
	}

	return digest.NewDigestFromBytes(digest.SHA256, hashTr.Sum(nil)), &desc, nil
}

func (b *BuildContext) writeDir(t *tar.Writer, dir *TreeNode) error {
	names := make([]string, 0, len(dir.Child))
	for name := range dir.Child {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		node := dir.Child[name]
		if err := t.WriteHeader(&node.Header); err != nil {
			return nil
		}
		if node.Typeflag == tar.TypeReg {
			if err := func() error {
				f, err := os.Open(node.Source)
				if err != nil {
					return err
				}
				defer f.Close()

				if _, err := io.Copy(t, f); err != nil {
					return err
				}
				return nil
			}(); err != nil {
				return err
			}
		}
		if node.Typeflag == tar.TypeDir {
			if err := b.writeDir(t, node); err != nil {
				return err
			}
		}
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

func (b *BuildContext) applyCopyCommand(cmd *instructions.CopyCommand) error {
	if cmd.From != "" {
		// TODO: Not implemented copy from other docker image
		return errorx.NotImplemented.New(cmd.String())
	}
	dest := cmd.Dest()
	if !path.IsAbs(dest) {
		dest = path.Join("/", b.imageManifest.Config.WorkingDir, dest)
	}
	dest, err := b.fs.EvalSymlinks(dest)
	if err != nil {
		return err
	}
	dir := strings.HasSuffix(cmd.Dest(), "/")
	if node := b.fs.Get(dest); node != nil {
		dir = node.Typeflag == tar.TypeDir
	}
	for _, source := range cmd.Sources() {
		full := source
		if !path.IsAbs(full) {
			full = path.Join(b.contextPath, full)
		}
		stat, err := os.Stat(full)
		if err != nil {
			return err
		}

		if stat.IsDir() {
			if !dir {
				return errorx.IllegalState.New("target must be a directory: %s", dest)
			}
			queue := make([]string, 0, 100)
			queue = append(queue, "")
			next := make([]string, 0, 100)
			for _, dir := range queue {
				files, err := ioutil.ReadDir(path.Join(full, dir))
				if err != nil {
					return err
				}
				for _, file := range files {
					if file.IsDir() {
						next = append(next, path.Join(dir, file.Name()))
						if err := b.addDir(path.Join(dest, dir, file.Name()), file); err != nil {
							return err
						}
						continue
					}
					if err := b.addFile(path.Join(dest, dir, file.Name()), path.Join(full, dir, file.Name())); err != nil {
						return err
					}
				}
			}
		} else {
			target := dest
			if dir {
				target = path.Join(target, path.Base(source))
			}
			if err := b.addFile(target, full); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *BuildContext) addDir(dest string, info os.FileInfo) error {
	return b.fs.Add(&TreeNode{
		Header: tar.Header{
			Name:     dest,
			Typeflag: tar.TypeDir,
		},
	})
}

func (b *BuildContext) addFile(dest string, source string) error {
	stat, err := os.Stat(source)
	if err != nil {
		return err
	}
	fmt.Println(source, "->", dest)
	return b.fs.Add(&TreeNode{
		Header: tar.Header{
			Name:     dest,
			Typeflag: tar.TypeReg,
			Size:     stat.Size(),
			Mode:     int64(stat.Mode()),
		},
		Source: source,
	})
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
