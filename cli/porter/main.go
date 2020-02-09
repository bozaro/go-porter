package main

import (
	"context"
	"fmt"
	"github.com/joomcode/go-porter/src"
	"github.com/mkideal/cli"
	"os"
)

type CmdRootT struct {
	cli.Helper
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
	return CmdRootT{
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
		return src.Pull(ctx, c.Args()...)
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
