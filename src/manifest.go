package src

import (
	"encoding/json"

	"github.com/joomcode/errorx"
	"github.com/moby/buildkit/frontend/dockerfile/dockerfile2llb"
)

type DeserializedImageManifest struct {
	dockerfile2llb.Image
	canonical [] byte
}

// UnmarshalJSON populates a new Manifest struct from JSON data.
func (m *DeserializedImageManifest) UnmarshalJSON(b []byte) error {
	m.canonical = make([]byte, len(b))
	// store manifest in canonical
	copy(m.canonical, b)

	// Unmarshal canonical JSON into Manifest object
	var manifest dockerfile2llb.Image
	if err := json.Unmarshal(m.canonical, &manifest); err != nil {
		return err
	}

	m.Image = manifest
	return nil
}

// MarshalJSON returns the contents of canonical. If canonical is empty,
// marshals the inner contents.
func (m *DeserializedImageManifest) MarshalJSON() ([]byte, error) {
	if len(m.canonical) > 0 {
		return m.canonical, nil
	}

	return nil, errorx.IllegalState.New("JSON representation not initialized in DeserializedManifest")
}

// Payload returns the raw content of the manifest.
func (m DeserializedImageManifest) Payload() ([]byte, error) {
	return m.canonical, nil
}
