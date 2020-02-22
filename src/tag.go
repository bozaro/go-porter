package src

import (
	"context"
	"github.com/google/go-containerregistry/pkg/name"

	"github.com/joomcode/errorx"
)

func (s *State) Tag(ctx context.Context, source string, target string) error {
	sourceInfo, err := name.ParseReference(source)
	if err != nil {
		return err
	}
	targetInfo, err := name.ParseReference(target)
	if err != nil {
		return err
	}

	manifest, err := s.LoadManifest(ctx, sourceInfo)
	if err != nil {
		return err
	}
	if manifest == nil {
		return errorx.IllegalArgument.New("can't find manifest for tag: %s", sourceInfo.Name())
	}

	if err := s.SaveManifest(ctx, manifest, targetInfo); err != nil {
		return err
	}
	return nil
}
