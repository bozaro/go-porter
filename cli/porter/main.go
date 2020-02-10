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
		_ = argv
		state, err := src.NewState(argv)
		if err != nil {
			return err
		}
		return state.Pull(ctx, c.Args()...)
	},
}

func main() {
	if err := cli.Root(root,
		cli.Tree(cmdPull),
	).Run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
