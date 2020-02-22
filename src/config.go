package src

import (
	"io"

	"github.com/google/go-containerregistry/pkg/authn"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Auths map[string]authn.AuthConfig `json:"auths"`
}

func (c *Config) Load(reader io.Reader) error {
	decoder := yaml.NewDecoder(reader)
	decoder.SetStrict(true)
	return decoder.Decode(c)
}
