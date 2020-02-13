package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"runtime/debug"
	"strings"

	"github.com/joomcode/go-porter/src"
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
	Target string `cli:"t,target" usage:"Target directory"`
	Cache  bool   `cli:"cache" usage:"Don't refresh cached manifest files'"`
}

type cmdBuildT struct {
	CmdRootT
	Dockerfile string `cli:"f,file" usage:"Name of the Dockerfile"`
	Target     string `cli:"target" usage:"Set the target build stage to build"`
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
		return state.Pull(ctx, argv.Cache, c.Args()...)
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
		digest, err := state.Build(ctx, argv, c.Args()[0])
		if err != nil {
			return err
		}
		fmt.Println(digest)
		return nil
	},
}

func main() {
	if err := cli.Root(root,
		cli.Tree(cmdPull),
		cli.Tree(cmdBuild),
	).Run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
