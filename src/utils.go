package src

import (
	"github.com/joomcode/errorx"
	"io"
	"os"
	"path"
	"strconv"
)

func safeWrite(filename string, task func(w io.Writer) error) error {
	if dir := path.Dir(filename); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			if !os.IsExist(err) {
				return err
			}
		}
	}
	for i := 0; ; i++ {
		tmp := filename + "~" + strconv.Itoa(i)
		f, err := os.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return errorx.InternalError.Wrap(err, "can't create temporary file: %s", tmp)
		}
		defer f.Close()
		if err := task(f); err != nil {
			os.Remove(tmp)
			return err
		}
		if err := f.Close(); err != nil {
			os.Remove(tmp)
			return errorx.InternalError.Wrap(err, "error on closing file: %s", tmp)
		}
		if err := os.Rename(tmp, filename); err != nil {
			os.Remove(tmp)
			return errorx.InternalError.Wrap(err, "error on rename file: %s -> %s", tmp, filename)
		}
		return nil
	}
}
