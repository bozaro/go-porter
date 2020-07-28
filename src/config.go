package src

import (
	"io"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"gopkg.in/yaml.v2"
)

const DefaultMinTemporaryAge = 5 * time.Minute

type Config struct {
	Auths           map[string]authn.AuthConfig `json:"auths"`
	MinTemporaryAge time.Duration               `json:"minTemporaryAge"`
}

func (c *Config) Load(reader io.Reader) error {
	decoder := yaml.NewDecoder(reader)
	decoder.SetStrict(true)
	return decoder.Decode(c)
}

func (c *Config) GetMinTemporaryAge() time.Duration {
	if c.MinTemporaryAge <= 0 {
		return DefaultMinTemporaryAge
	}
	return c.MinTemporaryAge
}
