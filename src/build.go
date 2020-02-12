package src

import (
	"context"
	"os"
	"path"

	"github.com/davecgh/go-spew/spew"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
	"github.com/opencontainers/go-digest"
)

func (s *State) Build(ctx context.Context, dockerFile string, contextPath string) (digest.Digest, error) {
	if dockerFile == "" {
		dockerFile = path.Join(contextPath, "Dockerfile")
	}

	parsed, err := s.parseDockerFile(dockerFile)
	if err != nil {
		return "", err
	}
	spew.Dump(parsed)

	return "", nil
}

func (s *State) parseDockerFile(dockerFile string) (*parser.Result, error) {
	file, err := os.Open(dockerFile)
	if err != nil {
		return nil, nil
	}
	defer file.Close()

	return parser.Parse(file)
}
