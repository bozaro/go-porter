package src

import (
	"os"
	"path"
	"time"

	"github.com/joomcode/errorx"
	bolt "go.etcd.io/bbolt"
)

type StateConfig interface {
	GetCacheDir() string
	GetConfigFile() string
}

type State struct {
	config   Config
	stateDir string
	db       *bolt.DB
}

func NewState(config StateConfig) (*State, error) {
	cacheDir := config.GetCacheDir()
	_ = os.MkdirAll(cacheDir, 0755)

	configFile := config.GetConfigFile()
	var stateConfig Config
	if f, err := os.Open(configFile); err == nil {
		defer f.Close()
		if err := stateConfig.Load(f); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	stateFile := path.Join(cacheDir, "state.db")
	db, err := bolt.Open(stateFile, 0644, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, errorx.InternalError.New("can't open cache: %s", stateFile)
	}

	return &State{
		config:   stateConfig,
		stateDir: cacheDir,
		db:       db,
	}, nil
}

func (s *State) Close() {
	if s.db != nil {
		_ = s.db.Close()
		s.db = nil
	}
}
