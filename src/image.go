package src

import (
	"github.com/docker/distribution/reference"
	"github.com/joomcode/errorx"
)

type ImageInfo struct {
	Name       string
	Domain     string
	Repository string
	Reference  string
}

func (s *State) ResolveImage(imageName string) (*ImageInfo, error) {
	ref, err := reference.Parse(imageName)
	if err != nil {
		return nil, err
	}
	named, ok := ref.(reference.Named)
	if !ok {
		return nil, errorx.IllegalArgument.New("expected named reference: %s", imageName)
	}
	tagged, ok := reference.TagNameOnly(named).(reference.NamedTagged)
	if !ok {
		return nil, errorx.IllegalArgument.New("expected tagged reference: %s", imageName)
	}

	domain := reference.Domain(named)
	path := reference.Path(named)
	repository := path
	if domain == "" {
		domain = "registry-1.docker.io"
		if repository != "" {
			repository = "library/" + repository
		} else {
			repository = "library"
		}
	}
	return &ImageInfo{
		Name:       tagged.String(),
		Domain:     domain,
		Repository: repository,
		Reference:  tagged.Tag(),
	}, nil
}
