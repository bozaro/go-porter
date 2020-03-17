module github.com/joomcode/go-porter

go 1.13

require (
	github.com/Bowery/prompt v0.0.0-20190916142128-fa8279994f75 // indirect
	github.com/blang/vfs v1.0.0
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/docker v1.14.0-0.20190319215453-e7b5f7dbe98c
	github.com/docker/go-units v0.4.0
	github.com/dustin/go-humanize v1.0.0
	github.com/google/go-cmp v0.4.0 // indirect
	github.com/google/go-containerregistry v0.0.0-20200225041405-6950943e71a1
	github.com/joomcode/errorx v1.0.0
	github.com/klauspost/compress v1.4.1
	github.com/klauspost/cpuid v1.2.0 // indirect
	github.com/labstack/gommon v0.3.0 // indirect
	github.com/mattn/go-colorable v0.1.4 // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/mkideal/cli v0.0.3
	github.com/mkideal/pkg v0.0.0-20170503154153-3e188c9e7ecc // indirect
	github.com/moby/buildkit v0.6.4
	github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/philhofer/fwd v1.0.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/sirupsen/logrus v1.4.2
	github.com/tinylib/msgp v1.1.1
	golang.org/x/crypto v0.0.0-20200208060501-ecb85df21340 // indirect
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e // indirect
	golang.org/x/sys v0.0.0-20200223170610-d5e6a3e2c0ae // indirect
	gopkg.in/yaml.v2 v2.2.4
	gotest.tools/v3 v3.0.2 // indirect
)

replace github.com/containerd/containerd => github.com/containerd/containerd v1.3.0

replace github.com/opencontainers/runc => github.com/opencontainers/runc v1.0.0-rc10

replace github.com/docker/docker => github.com/docker/docker v1.4.2-0.20200225044217-9fee52d54415

replace golang.org/x/crypto v0.0.0-20190129210102-0709b304e793 => golang.org/x/crypto v0.0.0-20191011191535-87dc89f01550
