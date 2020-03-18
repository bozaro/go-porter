package src

import (
	"os"
	"path"

	"github.com/blang/vfs"
)

type mergeFS struct {
	base  vfs.Filesystem
	delta vfs.Filesystem
}

func (m *mergeFS) PathSeparator() uint8 {
	return m.delta.PathSeparator()
}

func (m mergeFS) OpenFile(name string, flag int, perm os.FileMode) (vfs.File, error) {
	if flag&os.O_CREATE != 0 {
		dir := path.Dir(name)
		if stat, err := m.base.Stat(dir); err == nil && stat.IsDir() {
			if err := vfs.MkdirAll(m.delta, dir, 0755); err != nil {
				return nil, err
			}
		}
	}
	file, err := m.delta.OpenFile(name, flag, perm)
	if !os.IsNotExist(err) {
		return file, err
	}
	return m.base.OpenFile(name, flag, perm)
}

func (m mergeFS) Remove(name string) error {
	return m.delta.Remove(name)
}

func (m mergeFS) Rename(oldpath, newpath string) error {
	dir := path.Dir(newpath)
	if _, err := m.Stat(oldpath); err == nil {
		if stat, err := m.base.Stat(dir); err == nil && stat.IsDir() {
			if err := vfs.MkdirAll(m.delta, dir, 0755); err != nil {
				return err
			}
		}
	}
	return m.delta.Rename(oldpath, newpath)
}

func (m mergeFS) Mkdir(name string, perm os.FileMode) error {
	dir := path.Dir(name)
	if dir != "." {
		if stat, err := m.base.Stat(dir); err == nil && stat.IsDir() {
			if err := vfs.MkdirAll(m.delta, dir, 0755); err != nil {
				return err
			}
		}
	}
	return m.delta.Mkdir(name, perm)
}

func (m mergeFS) Stat(name string) (os.FileInfo, error) {
	info, err := m.delta.Stat(name)
	if !os.IsNotExist(err) {
		return info, err
	}
	return m.base.Stat(name)
}

func (m mergeFS) Lstat(name string) (os.FileInfo, error) {
	info, err := m.delta.Lstat(name)
	if !os.IsNotExist(err) {
		return info, err
	}
	return m.base.Lstat(name)
}

func (m mergeFS) ReadDir(path string) ([]os.FileInfo, error) {
	deltaFiles, err := m.delta.ReadDir(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	baseFiles, err := m.base.ReadDir(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	found := map[string]struct{}{}
	result := make([]os.FileInfo, 0, len(deltaFiles)+len(baseFiles))
	for _, item := range deltaFiles {
		result = append(result, item)
		found[item.Name()] = struct{}{}
	}
	for _, item := range baseFiles {
		if _, ok := found[item.Name()]; !ok {
			result = append(result, item)
		}
	}
	return result, nil
}

func CreateOverlayFS(base vfs.Filesystem, delta vfs.Filesystem) vfs.Filesystem {
	return &mergeFS{
		base:  vfs.ReadOnly(base),
		delta: delta,
	}
}
