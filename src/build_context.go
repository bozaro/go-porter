package src

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blang/vfs"
	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/uuid"
	"github.com/docker/go-units"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/joomcode/errorx"
	"github.com/klauspost/compress/gzip"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
)

type BuildContext struct {
	state       *State
	fs          FS
	contextPath string
	layers      []distribution.Descriptor
	configFile  v1.ConfigFile
}

type FileFilter func(header *tar.Header)

func NewBuildContext(ctx context.Context, state *State, baseName string, contextPath string) (*BuildContext, error) {
	if baseName == "scratch" {
		return &BuildContext{
			state:       state,
			contextPath: contextPath,
			fs: FS{
				Base: state.EmptyLayer(),
			},
		}, nil
	}

	baseImage, err := name.ParseReference(baseName)
	if err != nil {
		return nil, err
	}

	baseManifest, err := state.Pull(ctx, baseImage, true)
	if err != nil {
		return nil, err
	}

	var imageManifest v1.ConfigFile
	blob, err := state.ReadBlob(ctx, baseManifest.Config)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(blob, &imageManifest); err != nil {
		return nil, err
	}

	root := state.EmptyLayer()
	for _, layer := range baseManifest.Layers {
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
		layers:     baseManifest.Layers,
		configFile: imageManifest,
	}, nil
}

func (b *BuildContext) BuildManifest(ctx context.Context) (*schema2.DeserializedManifest, error) {
	if err := b.FlushDelta(ctx); err != nil {
		return nil, err
	}

	descriptor, err := b.SaveImageManifest(ctx)
	if err != nil {
		return nil, err
	}
	return schema2.FromStruct(schema2.Manifest{
		Versioned: schema2.SchemaVersion,
		Config:    *descriptor,
		Layers:    b.layers,
	})
}

func (b *BuildContext) ApplyCommand(cmd instructions.Command) error {
	logrus.Infof("Apply command: %s", cmd)
	b.configFile.History = append(b.configFile.History, v1.History{
		Created: v1.Time{
			Time: time.Now().UTC(),
		},
		CreatedBy:  fmt.Sprintf("%s", cmd.Name()),
		EmptyLayer: true,
	})
	switch cmd := cmd.(type) {
	case *instructions.CopyCommand:
		return b.applyCopyCommand(cmd)
	case *instructions.EntrypointCommand:
		b.applyEntrypointCommand(cmd)
	case *instructions.EnvCommand:
		b.applyEnvCommand(cmd)
	case *instructions.HealthCheckCommand:
		b.configFile.Config.Healthcheck = &v1.HealthConfig{
			Test:        cmd.Health.Test,
			Interval:    cmd.Health.Interval,
			Timeout:     cmd.Health.Timeout,
			StartPeriod: cmd.Health.StartPeriod,
			Retries:     cmd.Health.Retries,
		}
	case *instructions.LabelCommand:
		if b.configFile.Config.Labels == nil {
			b.configFile.Config.Labels = make(map[string]string)
		}
		for _, pair := range cmd.Labels {
			b.configFile.Config.Labels[pair.Key] = pair.Value
		}
	case *instructions.WorkdirCommand:
		b.configFile.Config.WorkingDir = cmd.Path
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

	hash, err := v1.NewHash(digest.String())
	if err != nil {
		return err
	}
	b.configFile.RootFS.DiffIDs = append(b.configFile.RootFS.DiffIDs, hash)
	b.layers = append(b.layers, *layer)
	b.fs.Delta = nil
	logrus.Infof("layer flushed: %s, %s, %v", layer.Digest, units.HumanSize(float64(layer.Size)), time.Now().Sub(t))

	history := b.configFile.History
	history[len(history)-1].EmptyLayer = false
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
	tempFile := path.Join("~" + uuid.Generate().String() + ".tar.gz")
	hashTr := sha256.New()
	hashGz := sha256.New()

	fs := b.state.stateVfs
	defer fs.Remove(tempFile)

	f, err := vfs.Create(fs, tempFile)
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
	_ = vfs.MkdirAll(fs, path.Dir(target), 0755)
	if err := fs.Rename(tempFile, target); err != nil {
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

func (b *BuildContext) SaveImageManifest(ctx context.Context) (*distribution.Descriptor, error) {
	data, err := json.Marshal(b.configFile)
	if err != nil {
		return nil, err
	}
	sum256 := sha256.Sum256(data)
	descriptor := distribution.Descriptor{
		MediaType: "application/vnd.docker.container.image.v1+json",
		Size:      int64(len(data)),
		Digest:    digest.NewDigestFromBytes(digest.SHA256, sum256[:]),
	}
	filename := b.state.blobName(descriptor, "")

	if err := safeWrite(b.state.stateVfs, filename, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	}); err != nil {
		return nil, err
	}

	return &descriptor, nil
}

func (b *BuildContext) applyEnvCommand(cmd *instructions.EnvCommand) {
	keys := map[string]struct{}{}
	for _, pair := range cmd.Env {
		keys[pair.Key] = struct{}{}
	}
	config := &b.configFile.Config
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
		args = append(getShell(b.configFile.Config, b.configFile.OS), args...)
	}
	b.configFile.Config.Cmd = nil
	b.configFile.Config.Entrypoint = args
}

func (b *BuildContext) applyCopyCommand(cmd *instructions.CopyCommand) error {
	if cmd.From != "" {
		// TODO: Not implemented copy from other docker image
		return errorx.NotImplemented.New(cmd.String())
	}
	dest := cmd.DestPath
	if !path.IsAbs(dest) {
		dest = path.Join("/", b.configFile.Config.WorkingDir, dest)
	}
	dest, err := b.fs.EvalSymlinks(dest)
	if err != nil {
		return err
	}
	dir := strings.HasSuffix(cmd.DestPath, "/")
	if node := b.fs.Get(dest); node != nil {
		dir = node.Typeflag == tar.TypeDir
	}

	var filter FileFilter
	if cmd.Chown != "" {
		m := regexp.MustCompile(`^(\d+):(\d+)$`).FindStringSubmatch(cmd.Chown)
		if len(m) == 0 {
			return errorx.IllegalArgument.New("illegal chown: %s", cmd.Chown)
		}
		uid, _ := strconv.Atoi(m[1])
		gid, _ := strconv.Atoi(m[2])
		filter = func(header *tar.Header) {
			header.Uid = uid
			header.Gid = gid
		}
	}

	for _, source := range cmd.SourcePaths {
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
			if err := b.addDir(dest, nil, filter); err != nil {
				return err
			}
			for {
				for _, dir := range queue {
					files, err := ioutil.ReadDir(path.Join(full, dir))
					if err != nil {
						return err
					}
					for _, file := range files {
						if file.IsDir() {
							next = append(next, path.Join(dir, file.Name()))
							if err := b.addDir(path.Join(dest, dir, file.Name()), file, filter); err != nil {
								return err
							}
							continue
						}
						if err := b.addFile(path.Join(dest, dir, file.Name()), path.Join(full, dir, file.Name()), filter); err != nil {
							return err
						}
					}
				}

				queue = next
				next = make([]string, 0, 100)

				if len(queue) <= 0 {
					break
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

func (b *BuildContext) addDir(dest string, info os.FileInfo, filters ...FileFilter) error {
	var mode os.FileMode = 0755
	if info != nil {
		mode = info.Mode()
	}
	header := tar.Header{
		Name:     dest,
		Typeflag: tar.TypeDir,
		Mode:     int64(mode),
	}
	for _, filter := range filters {
		if filter != nil {
			filter(&header)
		}
	}
	return b.fs.Add(&TreeNode{
		Header: header,
	})
}

func (b *BuildContext) addFile(dest string, source string, filters ...FileFilter) error {
	stat, err := os.Lstat(source)
	if err != nil {
		return err
	}

	if stat.Mode()&os.ModeDevice != 0 || stat.Mode()&os.ModeCharDevice != 0 || stat.Mode()&os.ModeSocket != 0 {
		// TODO
		return nil
	}

	fmt.Println(source, "->", dest)

	header := tar.Header{
		Name:     dest,
		Typeflag: tar.TypeReg,
		Size:     stat.Size(),
		Mode:     int64(stat.Mode()),
	}

	if stat.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(source)
		if err != nil {
			return err
		}

		header.Typeflag = tar.TypeSymlink
		header.Linkname = target
	}

	for _, filter := range filters {
		if filter != nil {
			filter(&header)
		}
	}
	return b.fs.Add(&TreeNode{
		Header: header,
		Source: source,
	})
}

func getShell(config v1.Config, os string) []string {
	if len(config.Shell) == 0 {
		return append([]string{}, defaultShellForOS(os)[:]...)
	}
	return append([]string{}, config.Shell[:]...)
}

func defaultShellForOS(os string) []string {
	return []string{"/bin/sh", "-c"}
}
