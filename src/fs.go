package src

import (
	"archive/tar"
	"github.com/joomcode/errorx"
	"path"
	"strings"
)

type FS struct {
	Base  *TreeNode
	Delta *TreeNode
}

func (fs *FS) EvalSymlinks(target string) (string, error) {
	result := ""
	base := fs.Base
	delta := fs.Delta
	antiLoop := map[string]struct{}{}

	resolveLink := func(link *TreeNode, target string) (string, error) {
		if _, ok := antiLoop[link.Name]; ok {
			return "", errorx.IllegalState.New("loop detected by symlink: %s", link.Name)
		}

		base = fs.Base
		delta = fs.Delta
		result = ""

		linkPath := link.Linkname
		if !path.IsAbs(linkPath) {
			linkPath = path.Clean(path.Join(link.Name, linkPath))
		}
		return path.Join(linkPath, target), nil
	}

	for target != "" {
		i := strings.IndexByte(target, '/')
		var name string
		if i >= 0 {
			name = target[:i]
			target = target[i+1:]
		} else {
			name = target
			target = ""
		}
		if delta != nil {
			if delta.Typeflag != tar.TypeDir {
				return "", errorx.IllegalState.New("expected directory for: %s", result)
			}
			if base != nil && base.Typeflag != tar.TypeDir {
				base = nil
			}
		} else if base != nil {
			if base.Typeflag != tar.TypeDir {
				return "", errorx.IllegalState.New("expected directory for: %s", result)
			}
		}
		if name == "" || name == "." {
			if result != "" && !strings.HasSuffix(result, "/") {
				result += "/"
			}
			continue
		}
		if name == ".." {
			i := strings.LastIndex(result, "/")
			if i < 0 {
				return "", errorx.IllegalState.New("try `..` cd from root directory")
			}
			target = result[:i+1] + target
			base = fs.Base
			delta = fs.Delta
			result = ""
			continue
		}

		checkBase := true
		if delta != nil {
			if child, ok := delta.Child[name]; ok {
				if child != nil && child.Typeflag == tar.TypeSymlink {
					var err error
					target, err = resolveLink(child, target)
					if err != nil {
						return "", err
					}
					continue
				}
				checkBase = false
			}
		}

		if base != nil && checkBase {
			if child, ok := base.Child[name]; ok {
				if child != nil && child.Typeflag == tar.TypeSymlink {
					var err error
					target, err = resolveLink(child, target)
					if err != nil {
						return "", err
					}
					continue
				}
			}
		}

		if base != nil {
			base = base.Child[name]
		}
		if delta != nil {
			delta = delta.Child[name]
		}
		result += name
		if i >= 0 {
			result += "/"
		}
	}
	return result, nil
}

func (fs *FS) Get(target string) *TreeNode {
	target = path.Clean(target)
	base := fs.Base
	delta := fs.Delta
	for target != "" {
		i := strings.IndexByte(target, '/')
		var name string
		if i >= 0 {
			name = target[:i]
			target = target[i+1:]
		} else {
			name = target
			target = ""
		}
		if delta != nil {
			if delta.Typeflag != tar.TypeDir {
				return nil
			}
			if base != nil && base.Typeflag != tar.TypeDir {
				base = nil
			}
		} else if base != nil {
			if base.Typeflag != tar.TypeDir {
				return nil
			}
		}
		if base != nil {
			base = base.Child[name]
		}
		if delta != nil {
			delta = delta.Child[name]
		}
	}
	if delta != nil {
		return delta
	}
	return base
}
