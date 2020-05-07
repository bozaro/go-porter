package src

import (
	"context"
	"io/ioutil"
	"os"
	"path"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"gopkg.in/yaml.v2"
)

func (s *State) Login(ctx context.Context, username string, password string, server string) error {
	repo, err := name.NewRegistry(server)
	if err != nil {
		return err
	}

	if s.config.Auths == nil {
		s.config.Auths = make(map[string]authn.AuthConfig)
	}
	s.config.Auths[repo.RegistryStr()] = authn.AuthConfig{
		Username: username,
		Password: password,
	}
	_ = os.MkdirAll(path.Dir(s.configFile), 0755)

	config, err := yaml.Marshal(s.config)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(s.configFile, config, 0600)
}
