package src

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"github.com/docker/distribution"
	"io"
	"strings"
)

type TreeNode struct {
	tar.Header
	Child map[string]*TreeNode
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

	root := &TreeNode{}
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

	fullpath := ""
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
		//item.Header.Name = fullpath + name
		node.Child[name] = item
		node = item
		fullpath = fullpath + name + "/"
	}
}
