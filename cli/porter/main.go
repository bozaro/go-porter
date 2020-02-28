package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"runtime/debug"
	"strings"
	"text/tabwriter"

	"github.com/dustin/go-humanize"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/joomcode/errorx"
	"github.com/joomcode/go-porter/src"
	"github.com/mkideal/cli"
)

type CmdRootT struct {
	cli.Helper
	CacheDir   string `cli:"cache" usage:"State directory"`
	ConfigFile string `cli:"config" usage:"Configuration file"`
}

func (c CmdRootT) GetCacheDir() string {
	return c.CacheDir
}

func (c CmdRootT) GetConfigFile() string {
	return c.ConfigFile
}

type cmdPullT struct {
	CmdRootT
	Cached bool `cli:"cached" usage:"Don't refresh cached manifest files'"`
}

type cmdBuildT struct {
	CmdRootT
	Dockerfile string `cli:"f,file" usage:"Name of the Dockerfile"`
	Tag        string `cli:"*t,tag" usage:"Name and optionally a tag in the 'name:tag' format"`
	Target     string `cli:"target" usage:"Set the target build stage to build"`
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
	Argv: func() interface{} { return new(CmdRootT) },
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
		CacheDir:   path.Join(cacheDir, strings.ReplaceAll(buildInfo.Main.Path, "/", ".")),
		ConfigFile: path.Join(configDir, path.Base(buildInfo.Main.Path)+".yaml"),
	}
}

var cmdPull = &cli.Command{
	Name: "pull",
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

var cmdBuild = &cli.Command{
	Name: "build",
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
		return nil
	},
}

var cmdImages = NewImagesCommand("images")
var cmdImageLs = NewImagesCommand("ls")

func NewImagesCommand(name string) *cli.Command {
	return &cli.Command{
		Name: name,
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

var cmdSave = NewImageSaveCommand("save")
var cmdImageSave = NewImageSaveCommand("save")

func NewImageSaveCommand(name string) *cli.Command {
	return &cli.Command{
		Name: name,
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

var cmdPush = &cli.Command{
	Name: "push",
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

var cmdTag = &cli.Command{
	Name: "tag",
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

func main() {
	if err := cli.Root(root,
		cli.Tree(cmdBuild),
		cli.Tree(cmdImages),
		cli.Tree(cmdImage,
			cli.Tree(cmdImageLs),
			cli.Tree(cmdImageSave),
		),
		cli.Tree(cmdPull),
		cli.Tree(cmdPush),
		cli.Tree(cmdSave),
		cli.Tree(cmdTag),
	).Run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
