package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"runtime/debug"
	"strings"
	"text/tabwriter"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/dustin/go-humanize"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/joomcode/errorx"
	"github.com/joomcode/go-porter/src"
	"github.com/mkideal/cli"
	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
)

type CmdRootT struct {
	cli.Helper
	CacheDir    string `cli:"cache" usage:"State directory" dft:"$PORTER_CACHE"`
	ConfigFile  string `cli:"config" usage:"Configuration file" dft:"$PORTER_CONFIG"`
	LogLevel    string `cli:"log" usage:"Log level (panic, fatal, error, warn, info, debug)" dft:"error"`
	MemoryCache bool   `cli:"memory-cache" usage:"Keep all state changes only in memory"`
}

func (c CmdRootT) GetCacheDir() string {
	return c.CacheDir
}

func (c CmdRootT) GetMemoryCache() bool {
	return c.MemoryCache
}

func (c CmdRootT) GetConfigFile() string {
	return c.ConfigFile
}

func (c CmdRootT) GetLogLevel() logrus.Level {
	level, err := logrus.ParseLevel(c.LogLevel)
	if err != nil {
		panic(err)
	}
	return level
}

type cmdPullT struct {
	CmdRootT
	Cached bool `cli:"cached" usage:"Don't refresh cached manifest files'"`
}

type cmdBuildT struct {
	CmdRootT
	Dockerfile string `cli:"f,file" usage:"Name of the Dockerfile"`
	Tag        string `cli:"t,tag" usage:"Name and optionally a tag in the 'name:tag' format"`
	Target     string `cli:"target" usage:"Set the target build stage to build"`
	Push       bool   `cli:"push" usage:"Push docker image after build"`
	Platform   string `cli:"platform" usage:"Set target platform for build"`
}

type cmdLoginT struct {
	CmdRootT
	Password string `cli:"*p,password" usage:"Password"`
	Username string `cli:"*u,username" usage:"Username"`
}

type cmdImageLsT struct {
	CmdRootT
}

type cmdSaveT struct {
	CmdRootT
	Output string `cli:"o,output" usage:"Write to a file, instead of STDOUT"`
}

type cmdPushT struct {
	CmdRootT
}

type cmdTagT struct {
	CmdRootT
}

var root = &cli.Command{
	Desc: "https://github.com/joomcode/go-porter",
	Argv: func() interface{} {
		argv := newCmdRoot()
		return &argv
	},
	Fn: func(ctx *cli.Context) error {
		ctx.WriteUsage()
		os.Exit(1)
		return nil
	},
}

var cmdImage = &cli.Command{
	Name: "image",
	Desc: "Manage images",
	Fn: func(ctx *cli.Context) error {
		ctx.WriteUsage()
		os.Exit(1)
		return nil
	},
}

func (c cmdBuildT) GetDockerfile() string {
	return c.Dockerfile
}

func (c cmdBuildT) GetTarget() string {
	return c.Target
}

func (c cmdBuildT) GetTag() string {
	return c.Tag
}

func (c cmdBuildT) GetPlatform() string {
	return c.Platform
}

func newCmdRoot() CmdRootT {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		panic("can't read build info")
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		panic(err)
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		panic(err)
	}
	return CmdRootT{
		CacheDir:   pathConfig("PORTER_CACHE", path.Join(cacheDir, strings.ReplaceAll(buildInfo.Main.Path, "/", "."))),
		ConfigFile: pathConfig("PORTER_CONFIG", path.Join(configDir, path.Base(buildInfo.Main.Path)+".yaml")),
		LogLevel:   "error",
	}
}

func pathConfig(env string, def string) string {
	value := os.Getenv(env)
	if value == "" {
		value = def
	}
	if !path.IsAbs(value) {
		cwd, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		value = path.Join(cwd, value)
	}
	return path.Clean(value)
}

func NewImageRemoveCommand(cmd string) *cli.Command {
	return &cli.Command{
		Name: cmd,
		Desc: "Remove one or more images",
		Argv: func() interface{} {
			return &cmdPullT{
				CmdRootT: newCmdRoot(),
			}
		},
		NumArg:      cli.AtLeast(1),
		CanSubRoute: true,
		Fn: func(c *cli.Context) error {
			argv := c.Argv().(*cmdPullT)
			ctx := context.Background()
			state, err := src.NewState(argv)
			if err != nil {
				return err
			}
			defer state.Close()

			return state.Remove(ctx, c.Args()...)
		},
	}
}

func NewImagePullCommand(cmd string) *cli.Command {
	return &cli.Command{
		Name: cmd,
		Desc: "Pull an image from a registry",
		Argv: func() interface{} {
			return &cmdPullT{
				CmdRootT: newCmdRoot(),
			}
		},
		NumArg:      cli.AtLeast(1),
		CanSubRoute: true,
		Fn: func(c *cli.Context) error {
			argv := c.Argv().(*cmdPullT)
			ctx := context.Background()
			state, err := src.NewState(argv)
			if err != nil {
				return err
			}
			defer state.Close()

			for _, imageName := range c.Args() {
				image, err := name.ParseReference(imageName)
				if err != nil {
					return err
				}
				if _, err := state.Pull(ctx, image, argv.Cached); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func NewLoginCommand(cmd string) *cli.Command {
	return &cli.Command{
		Name: cmd,
		Desc: "Log in to a Docker registry",
		Argv: func() interface{} {
			return &cmdLoginT{
				CmdRootT: newCmdRoot(),
			}
		},
		NumArg:      cli.ExactN(1),
		CanSubRoute: true,
		Fn: func(c *cli.Context) error {
			argv := c.Argv().(*cmdLoginT)
			ctx := context.Background()
			state, err := src.NewState(argv)
			if err != nil {
				return err
			}
			defer state.Close()

			if err := state.Login(ctx, argv.Username, argv.Password, c.Args()[0]); err != nil {
				return err
			}
			return nil
		},
	}
}

func NewImageBuildCommand(cmd string) *cli.Command {
	return &cli.Command{
		Name: cmd,
		Desc: "Build an image from a Dockerfile",
		Argv: func() interface{} {
			return &cmdBuildT{
				CmdRootT: newCmdRoot(),
			}
		},
		NumArg:      cli.ExactN(1),
		CanSubRoute: true,
		Fn: func(c *cli.Context) error {
			argv := c.Argv().(*cmdBuildT)
			if argv.Push && (argv.GetTag() == "") {
				return errorx.IllegalArgument.New("tag is required for push docker images")
			}

			ctx := context.Background()
			state, err := src.NewState(argv)
			if err != nil {
				return err
			}
			defer state.Close()

			digest, err := state.Build(ctx, argv, c.Args()[0])
			if err != nil {
				return err
			}
			fmt.Println(digest)
			if argv.Push {
				if err := state.Push(ctx, argv.GetTag()); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func NewImageListCommand(cmd string) *cli.Command {
	return &cli.Command{
		Name: cmd,
		Desc: "List images",
		Argv: func() interface{} {
			return &cmdImageLsT{
				CmdRootT: newCmdRoot(),
			}
		},
		CanSubRoute: true,
		Fn: func(c *cli.Context) error {
			argv := c.Argv().(*cmdImageLsT)
			ctx := context.Background()
			state, err := src.NewState(argv)
			if err != nil {
				return err
			}
			defer state.Close()

			images, err := state.GetImages(ctx)
			if err != nil {
				return err
			}

			w := tabwriter.NewWriter(os.Stdout, 1, 0, 3, ' ', 0)
			fmt.Fprintln(w, strings.Join([]string{
				"REPOSITORY",
				"TAG",
				"IMAGE ID",
				"SIZE",
			}, "\t"))
			var lines []string
			for image, manifest := range images {
				var size int64
				for _, layer := range manifest.Layers {
					size += layer.Size
				}
				lines = append(lines, strings.Join([]string{
					image.Context().RegistryStr() + "/" + image.Context().RepositoryStr(),
					image.Identifier(),
					manifest.Config.Digest.Hex()[0:12],
					humanize.Bytes(uint64(size)),
				}, "\t"))
			}
			for _, line := range lines {
				fmt.Fprintln(w, line)
			}
			w.Flush()
			return nil
		},
	}
}

func NewImageInspectCommand(cmd string) *cli.Command {
	return &cli.Command{
		Name: cmd,
		Desc: "Return low-level information on Docker objects",
		Argv: func() interface{} {
			return &cmdImageLsT{
				CmdRootT: newCmdRoot(),
			}
		},
		NumArg:      cli.AtLeast(1),
		CanSubRoute: true,
		Fn: func(c *cli.Context) error {
			argv := c.Argv().(*cmdImageLsT)
			ctx := context.Background()
			state, err := src.NewState(argv)
			if err != nil {
				return err
			}
			defer state.Close()

			inspectedByID := make(map[digest.Digest]*types.ImageInspect)
			inspected := make([]*types.ImageInspect, 0, len(c.Args()))
			for _, image := range c.Args() {
				info, err := name.ParseReference(image)
				if err != nil {
					return err
				}

				manifest, err := state.LoadManifest(ctx, info)
				if err != nil {
					return err
				}
				if manifest == nil {
					return errorx.IllegalArgument.New("image not found: %s", info.Name())
				}

				inspect := inspectedByID[manifest.Config.Digest]
				if inspect == nil {
					var imageManifest v1.ConfigFile
					configBlob, err := state.ReadBlob(ctx, manifest.Config)
					if err != nil {
						return err
					}
					if err := json.Unmarshal(configBlob, &imageManifest); err != nil {
						return err
					}
					layers := make([]string, 0, len(imageManifest.RootFS.DiffIDs))
					for _, layer := range imageManifest.RootFS.DiffIDs {
						layers = append(layers, layer.String())
					}
					var size int64
					for _, layer := range manifest.Layers {
						size += layer.Size
					}
					inspect = &types.ImageInspect{
						ID:            manifest.Config.Digest.String(),
						Created:       imageManifest.Created.String(),
						DockerVersion: imageManifest.DockerVersion,
						Architecture:  imageManifest.Architecture,
						Author:        imageManifest.Author,
						Os:            imageManifest.OS,
						OsVersion:     imageManifest.OSVersion,
						Config: &container.Config{
							Env:         imageManifest.Config.Env,
							Cmd:         imageManifest.Config.Cmd,
							ArgsEscaped: imageManifest.Config.ArgsEscaped,
							Entrypoint:  imageManifest.Config.Entrypoint,
							WorkingDir:  imageManifest.Config.WorkingDir,
							Labels:      imageManifest.Config.Labels,
							User:        imageManifest.Config.User,
						},
						RootFS: types.RootFS{
							Type:   imageManifest.RootFS.Type,
							Layers: layers,
						},
						Size:        size,
						VirtualSize: size,
					}
					inspected = append(inspected, inspect)
					inspectedByID[manifest.Config.Digest] = inspect
				}
				inspected = append(inspected, inspect)
				inspect.RepoTags = append(inspect.RepoTags, info.String())
			}
			payload, err := json.MarshalIndent(inspected, "", "    ")
			if err != nil {
				return err
			}
			fmt.Println(string(payload))
			return nil
		},
	}
}

func NewImageSaveCommand(cmd string) *cli.Command {
	return &cli.Command{
		Name: cmd,
		Desc: "Save one or more images to a tar archive (streamed to STDOUT by default)",
		Argv: func() interface{} {
			return &cmdSaveT{
				CmdRootT: newCmdRoot(),
			}
		},
		NumArg:      cli.AtLeast(1),
		CanSubRoute: true,
		Fn: func(c *cli.Context) error {
			argv := c.Argv().(*cmdSaveT)
			ctx := context.Background()
			state, err := src.NewState(argv)
			if err != nil {
				return err
			}
			defer state.Close()

			w := os.Stdout
			if argv.Output != "" {
				f, err := os.Create(argv.Output)
				if err != nil {
					return err
				}
				defer f.Close()
				w = f
			}
			if w == nil {
				return errorx.IllegalArgument.New("stdout is not exists")
			}
			if err := state.Save(ctx, w, c.Args()...); err != nil {
				return err
			}
			return nil
		},
	}
}

func NewImagePushCommand(cmd string) *cli.Command {
	return &cli.Command{
		Name: cmd,
		Desc: "Push one or more images to a registry",
		Argv: func() interface{} {
			return &cmdPushT{
				CmdRootT: newCmdRoot(),
			}
		},
		NumArg:      cli.AtLeast(1),
		CanSubRoute: true,
		Fn: func(c *cli.Context) error {
			argv := c.Argv().(*cmdPushT)
			ctx := context.Background()
			state, err := src.NewState(argv)
			if err != nil {
				return err
			}
			defer state.Close()

			if err := state.Push(ctx, c.Args()...); err != nil {
				return err
			}
			return nil
		},
	}
}

func NewImageTagCommand(cmd string) *cli.Command {
	return &cli.Command{
		Name: cmd,
		Desc: "Create a tag TARGET_IMAGE that refers to SOURCE_IMAGE",
		Argv: func() interface{} {
			return &cmdTagT{
				CmdRootT: newCmdRoot(),
			}
		},
		NumArg:      cli.ExactN(2),
		CanSubRoute: true,
		Fn: func(c *cli.Context) error {
			argv := c.Argv().(*cmdTagT)
			ctx := context.Background()
			state, err := src.NewState(argv)
			if err != nil {
				return err
			}
			defer state.Close()

			if err := state.Tag(ctx, c.Args()[0], c.Args()[1]); err != nil {
				return err
			}
			return nil
		},
	}
}

func main() {
	cli.SetUsageStyle(cli.ManualStyle)
	if err := cli.Root(root,
		cli.Tree(NewImageBuildCommand("build")),
		cli.Tree(NewImageListCommand("images")),
		cli.Tree(NewImageInspectCommand("inspect")),
		cli.Tree(cmdImage,
			cli.Tree(NewImageBuildCommand("build")),
			cli.Tree(NewImageInspectCommand("inspect")),
			cli.Tree(NewImageListCommand("ls")),
			cli.Tree(NewImagePullCommand("pull")),
			cli.Tree(NewImagePushCommand("push")),
			cli.Tree(NewImageRemoveCommand("rm")),
			cli.Tree(NewImageSaveCommand("save")),
			cli.Tree(NewImageTagCommand("tag")),
		),
		cli.Tree(NewLoginCommand("login")),
		cli.Tree(NewImagePullCommand("pull")),
		cli.Tree(NewImagePushCommand("push")),
		cli.Tree(NewImageRemoveCommand("rmi")),
		cli.Tree(NewImageSaveCommand("save")),
		cli.Tree(NewImageTagCommand("tag")),
	).Run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
