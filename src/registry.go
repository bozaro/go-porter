package src

import (
	"context"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/heroku/docker-registry-client/registry"
)

func (s *State) Registry(ctx context.Context, image  name.Reference) (*registry.Registry, error) {
	registryStr := image.Context().RegistryStr()
	if reg := func() *registry.Registry {
		s.registryLock.RLock()
		defer s.registryLock.RUnlock()
		return s.registry[registryStr]
	}(); reg != nil {
		return reg, nil
	}

	s.registryLock.Lock()
	defer s.registryLock.Unlock()

	if reg := s.registry[registryStr]; reg != nil {
		return reg, nil
	}

	url := "https://" + registryStr + "/"
	username := "" // anonymous
	password := "" // anonymous
	reg, err := registry.New(url, username, password)
	if err != nil {
		return nil, err
	}
	s.registry[registryStr] = reg
	return reg, nil
}
