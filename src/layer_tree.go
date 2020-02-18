package src

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"github.com/docker/distribution"
	"github.com/docker/docker/pkg/archive"
	"io"
	"io/ioutil"
	"strings"
)

type TreeNode struct {
	tar.Header
	Child  map[string]*TreeNode
	Source string
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

	treeFile := s.blobName(blob, ".tree")
	if cached, err := ioutil.ReadFile(treeFile); err == nil {
		var root TreeNode
		if err := json.Unmarshal(cached, &root); err == nil {
			return &root, nil
		}
	}

	root := s.EmptyLayer()
	r, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}

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

	if err := safeWrite(treeFile, func(w io.Writer) error {
		cache, err := json.Marshal(root)
		if err != nil {
			return err
		}
		if _, err := w.Write(cache); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
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
	if t.Child != nil {
		if _, ok := diff.Child[archive.WhiteoutOpaqueDir]; ok {
			t.Child = nil
		}
	}
	t.Header = diff.Header
	if diff.Typeflag == tar.TypeDir {
		for name, item := range diff.Child {
			if strings.HasPrefix(name, archive.WhiteoutMetaPrefix) {
				continue
			}
			if strings.HasPrefix(name, archive.WhiteoutPrefix) {
				name = name[len(archive.WhiteoutPrefix):]
				delete(t.Child, name)
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
				old = &TreeNode{
					Header: item.Header,
				}
				t.Child[name] = old
			}
			old.ApplyDiff(item)
		}
	}
}
