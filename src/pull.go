package src

import (
	"context"
	"github.com/heroku/docker-registry-client/registry"
	"io/ioutil"
)

func Pull(ctx context.Context, images ...string) error {
	url := "https://registry-1.docker.io/"
	username := "" // anonymous
	password := "" // anonymous
	hub, err := registry.New(url, username, password)
	if err != nil {
		return err
	}

	manifest, err := hub.Manifest("library/alpine", "latest")
	if err != nil {
		return err
	}

	for _, layer := range manifest.FSLayers {
		reader, err := hub.DownloadBlob("library/alpine", layer.BlobSum)
		if err != nil {
			return err
		}
		content, err := ioutil.ReadAll(reader)
		if err != nil {
			return err
		}
		ioutil.WriteFile(layer.BlobSum.Hex()+".tgz", content, 0644)
		reader.Close()
	}
	return nil
}
