package test

import (
	"context"
	"github.com/joomcode/go-porter/src"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"testing"
)

type TestStateConfig struct {
	LogLevel    logrus.Level
	CacheDir    string
	ConfigFile  string
	MemoryCache bool
}

var defaultConfig = TestStateConfig{
	LogLevel:    logrus.InfoLevel,
	CacheDir:    "",
	ConfigFile:  "",
	MemoryCache: true,
}

func (c TestStateConfig) GetLogLevel() logrus.Level {
	return c.LogLevel
}

func (c TestStateConfig) GetCacheDir() string {
	return c.CacheDir
}

func (c TestStateConfig) GetConfigFile() string {
	return c.ConfigFile
}

func (c TestStateConfig) GetMemoryCache() bool {
	return c.MemoryCache
}

type TestBuildArgs struct {
	Dockerfile string
	Target     string
	Tag        string
}

func (t TestBuildArgs) GetDockerfile() string {
	return t.Dockerfile
}

func (t TestBuildArgs) GetTarget() string {
	return t.Target
}

func (t TestBuildArgs) GetTag() string {
	return t.Tag
}

func (t TestBuildArgs) GetPlatform() string {
	return ""
}

func TestBuildEmpty(t *testing.T) {
	state, err := src.NewState(defaultConfig)
	assert.NoError(t, err)

	ctx := context.Background()

	_, err = state.Build(ctx, TestBuildArgs{}, "")
	assert.EqualError(t, err, "open Dockerfile: no such file or directory")
}
