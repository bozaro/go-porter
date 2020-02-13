package src

import (
	"context"
	"os"
	"path"

	"github.com/davecgh/go-spew/spew"
	"github.com/joomcode/errorx"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
	"github.com/opencontainers/go-digest"
)

type BuildArgs interface {
	GetDockerfile() string
	GetTarget() string
}

func (s *State) Build(ctx context.Context, args BuildArgs, contextPath string) (digest.Digest, error) {
	dockerFile := args.GetDockerfile()
	if dockerFile == "" {
		dockerFile = path.Join(contextPath, "Dockerfile")
	}

	stages, err := s.parseDockerFile(dockerFile)
	if err != nil {
		return "", err
	}

	stage, err := s.extractStage(stages, args.GetTarget())
	if err != nil {
		return "", err
	}
	spew.Dump(stage)

	return "", nil
}

func (s *State) extractStage(stages []*instructions.Stage, name string) (*instructions.Stage, error) {
	var result *instructions.Stage
	for {
		var found *instructions.Stage
		if name != "" {
			for i, stage := range stages {
				if stage.Name == name {
					found = stage
					stages = stages[:i]
					break
				}
			}
		} else {
			found = stages[len(stages)-1]
			stages = stages[:len(stages)-1]
		}
		if found == nil {
			break
		}
		result = s.mergeStage(found, result)
		name = result.BaseName
		if name == "" {
			break
		}
	}
	if result == nil {
		return nil, errorx.InternalError.New("can't find state with name: %s", name)
	}
	return result, nil
}

func (s *State) mergeStage(base *instructions.Stage, post *instructions.Stage) *instructions.Stage {
	if post == nil {
		return base
	}
	commands := make([]instructions.Command, 0, len(base.Commands)+len(post.Commands))
	commands = append(commands, base.Commands...)
	commands = append(commands, post.Commands...)
	first := func(a string, b string) string {
		if a != "" {
			return a
		}
		return b
	}
	return &instructions.Stage{
		Name:       post.Name,
		Commands:   commands,
		BaseName:   base.BaseName,
		SourceCode: base.SourceCode,
		Platform:   first(post.Platform, base.Platform),
	}
}

func (s *State) parseDockerFile(dockerFile string) ([]*instructions.Stage, error) {
	file, err := os.Open(dockerFile)
	if err != nil {
		return nil, nil
	}
	defer file.Close()

	parsed, err := parser.Parse(file)
	if err != nil {
		return nil, err
	}
	var stages []*instructions.Stage
	for _, child := range parsed.AST.Children {
		instruction, err := instructions.ParseInstruction(child)
		if err != nil {
			return nil, err
		}
		if stage, ok := instruction.(*instructions.Stage); ok {
			stages = append(stages, stage)
			continue
		}
		if len(stages) == 0 {
			return nil, errorx.IllegalFormat.New("FROM must be first directive in Dockerfile")
		}
		if command, ok := instruction.(instructions.Command); ok {
			stage := stages[len(stages)-1]
			stage.Commands = append(stage.Commands, command)
			continue
		}
		return nil, errorx.InternalError.New("unexpected instruction: %s", child.Original)
	}
	return stages, nil
}
