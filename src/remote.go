package src

import (
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func (s *State) RemoveOptions(ref name.Reference) []remote.Option {
	return nil
}
