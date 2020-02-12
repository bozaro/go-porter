package src

import (
	"os"
	"path"
	"sync"
	"time"

	"github.com/heroku/docker-registry-client/registry"
	"github.com/joomcode/errorx"
	bolt "go.etcd.io/bbolt"
)

type StateConfig interface {
	GetStateDir() string
}

type State struct {
	stateDir string
	db       *bolt.DB

	registryLock sync.RWMutex
	registry     map[string]*registry.Registry
}

func NewState(config StateConfig) (*State, error) {
	stateDir := config.GetStateDir()
	_ = os.MkdirAll(stateDir, 0755)

	stateFile := path.Join(stateDir, "state.db")
	db, err := bolt.Open(stateFile, 0644, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, errorx.InternalError.New("can't open cache: %s", stateFile)
	}

	return &State{
		stateDir: stateDir,
		db:       db,
		registry: map[string]*registry.Registry{},
	}, nil
}

func (s *State) Close() {
	if s.db != nil {
		_ = s.db.Close()
		s.db = nil
	}
}
