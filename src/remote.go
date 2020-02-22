package src

import (
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func (s *State) RemoveOptions(ref name.Reference) []remote.Option {
	var opts []remote.Option
	if authConfig, ok := s.config.Auths[ref.Context().RegistryStr()]; ok {
		opts = append(opts, remote.WithAuth(authn.FromConfig(authConfig)))
	}
	return opts
}
