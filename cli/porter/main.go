package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"runtime/debug"
	"strings"

	"github.com/joomcode/go-porter/src"
	"github.com/joomcode/errorx"
	"github.com/mkideal/cli"
)

type CmdRootT struct {
	cli.Helper
	StateDir string `cli:"state" usage:"State directory"`
}

func (c CmdRootT) GetStateDir() string {
	return c.StateDir
}

type cmdPullT struct {
	CmdRootT
	Cache bool `cli:"cache" usage:"Don't refresh cached manifest files'"`
}

type cmdBuildT struct {
	CmdRootT
	Dockerfile string `cli:"f,file" usage:"Name of the Dockerfile"`
	Tag        string `cli:"*t,tag" usage:"Name and optionally a tag in the 'name:tag' format"`
	Target     string `cli:"target" usage:"Set the target build stage to build"`
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
	return CmdRootT{
		StateDir: path.Join(cacheDir, strings.ReplaceAll(buildInfo.Main.Path, "/", ".")),
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
			image, err := state.ResolveImage(imageName)
			if err != nil {
				return err
			}
			if _, err := state.Pull(ctx, image, argv.Cache); err != nil {
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

var cmdSave = &cli.Command{
	Name: "save",
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
		cli.Tree(cmdPull),
		cli.Tree(cmdPush),
		cli.Tree(cmdSave),
		cli.Tree(cmdTag),
	).Run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
