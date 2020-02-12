package src

import (
	"context"

	"github.com/heroku/docker-registry-client/registry"
)

func (s *State) Registry(ctx context.Context, imageInfo *ImageInfo) (*registry.Registry, error) {
	if reg := func() *registry.Registry {
		s.registryLock.RLock()
		defer s.registryLock.RUnlock()
		return s.registry[imageInfo.Domain]
	}(); reg != nil {
		return reg, nil
	}

	s.registryLock.Lock()
	defer s.registryLock.Unlock()

	if reg := s.registry[imageInfo.Domain]; reg != nil {
		return reg, nil
	}

	url := "https://" + imageInfo.Domain + "/"
	username := "" // anonymous
	password := "" // anonymous
	reg, err := registry.New(url, username, password)
	if err != nil {
		return nil, err
	}
	s.registry[imageInfo.Domain] = reg
	return reg, nil
}
