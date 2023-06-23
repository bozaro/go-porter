package src

import (
	"encoding/json"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/joomcode/errorx"
)

type DeserializedImageManifest struct {
	v1.ConfigFile
	canonical []byte
}

// UnmarshalJSON populates a new Manifest struct from JSON data.
func (m *DeserializedImageManifest) UnmarshalJSON(b []byte) error {
	m.canonical = make([]byte, len(b))
	// store manifest in canonical
	copy(m.canonical, b)

	// Unmarshal canonical JSON into Manifest object
	var manifest v1.ConfigFile
	if err := json.Unmarshal(m.canonical, &manifest); err != nil {
		return err
	}

	m.ConfigFile = manifest
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
