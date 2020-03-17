package src

import (
	"crypto/sha1"
	"encoding/hex"
	"io"
	"os"
	"path"
	"strings"

	"github.com/blang/vfs"
	"github.com/blang/vfs/prefixfs"
	"github.com/sirupsen/logrus"
	"github.com/tinylib/msgp/msgp"
)

type StateConfig interface {
	GetLogLevel() logrus.Level
	GetCacheDir() string
	GetConfigFile() string
}

type State struct {
	config   Config
	stateVfs vfs.Filesystem
}

func NewState(config StateConfig) (*State, error) {
	logrus.SetLevel(config.GetLogLevel())

	logrus.Infof("PORTER_CACHE=%s", config.GetCacheDir())
	logrus.Infof("PORTER_CONFIG=%s", config.GetConfigFile())

	cacheDir := config.GetCacheDir()
	_ = os.MkdirAll(cacheDir, 0755)
	stateVfs := prefixfs.Create(vfs.OS(), cacheDir)

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

	return &State{
		config:   stateConfig,
		stateVfs: stateVfs,
	}, nil
}

func (s *State) Close() {}

func (s *State) cacheSave(bucket string, key string, data []byte) error {
	var payload []byte
	payload = msgp.AppendString(payload, key)
	payload = msgp.AppendBytes(payload, data)

	return safeWrite(s.stateVfs, s.cacheFile(bucket, key), func(w io.Writer) error {
		_, err := w.Write(payload)
		return err
	})
}

func (s *State) cacheRemove(bucket string, key string) error {
	return s.stateVfs.Remove(s.cacheFile(bucket, key))
}

func (s *State) cacheLoad(bucket string, key string) ([]byte, bool, error) {
	cached, err := vfs.ReadFile(s.stateVfs, s.cacheFile(bucket, key))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	_, cached, err = msgp.ReadStringBytes(cached)
	if err != nil {
		return nil, false, err
	}
	value, _, err := msgp.ReadBytesBytes(cached, nil)
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func (s *State) cacheForEach(bucket string, f func(key string, value []byte) error) error {
	dir := bucket
	items, err := s.stateVfs.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, item := range items {
		if item.IsDir() || strings.IndexByte(item.Name(), '~') > 0 {
			continue
		}
		cached, err := vfs.ReadFile(s.stateVfs, path.Join(dir, item.Name()))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		key, cached, err := msgp.ReadStringBytes(cached)
		if err != nil {
			continue
		}
		value, cached, err := msgp.ReadBytesBytes(cached, nil)
		if err != nil {
			continue
		}
		if len(cached) != 0 {
			continue
		}
		if err := f(key, value); err != nil {
			return err
		}
	}
	return nil
}

func (s *State) cacheFile(bucket string, key string) string {
	hash := sha1.Sum([]byte(key))
	return path.Join(bucket, hex.EncodeToString(hash[:]))
}
