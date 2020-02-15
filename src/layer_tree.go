package src

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"github.com/docker/distribution"
	"io"
	"strings"
)

const deletePrefix = ".wh."

type TreeNode struct {
	tar.Header
	Child map[string]*TreeNode
}

func (s *State) EmptyLayer() *TreeNode {
	return &TreeNode{
		Header: tar.Header{
			Typeflag: tar.TypeDir,
		},
	}
}

func (s *State) LayerTree(ctx context.Context, blob distribution.Descriptor) (*TreeNode, error) {
	f, err := s.OpenBlob(ctx, blob)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}

	root := s.EmptyLayer()
	t := tar.NewReader(r)
	for {
		item, err := t.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		root.Add(item)
		for {
			var data [65536]byte
			if _, err := t.Read(data[:]); err != nil {
				if err == io.EOF {
					break
				}
				return nil, err
			}
		}
	}
	return root, nil
}

func (t *TreeNode) Add(tarItem *tar.Header) {
	full := strings.TrimRight(strings.TrimLeft(tarItem.Name, "/"), "/")
	node := t

	fullpath := node.Name
	for full != "" {
		if node.Child == nil {
			node.Child = make(map[string]*TreeNode)
		}

		idx := strings.IndexByte(full, '/')
		var name string
		if idx >= 0 {
			name = full[:idx]
			full = full[idx+1:]
		} else {
			name = full
			full = ""
		}

		item := node.Child[name]
		if item != nil {
			if item.Typeflag != tar.TypeDir || (tarItem.Typeflag != tar.TypeDir && idx < 0) {
				item = nil
			}
		}
		if item == nil {
			item = &TreeNode{
				Header: tar.Header{
					Mode:     0755,
					Typeflag: tar.TypeDir,
				},
			}
		}
		if idx < 0 {
			item.Header = *tarItem
		}
		item.Header.Name = fullpath + name
		node.Child[name] = item
		node = item
		fullpath = fullpath + name + "/"
	}
}

func (t *TreeNode) ApplyDiff(diff *TreeNode) {
	if t.Typeflag != tar.TypeDir || diff.Typeflag != tar.TypeDir {
		t.Child = nil
	}
	t.Header = diff.Header
	if diff.Typeflag == tar.TypeDir {
		for name, item := range diff.Child {
			if strings.HasPrefix(name, deletePrefix) {
				name = name[len(deletePrefix):]
				delete (t.Child, name)
				if len(t.Child) == 0 {
					t.Child = nil
				}
				continue
			}
			old := t.Child[name]
			if old == nil {
				if t.Child == nil {
					t.Child = map[string]*TreeNode{}
				}
				t.Child[name] = item
				continue
			}
			old.ApplyDiff(item)
		}
	}
}
