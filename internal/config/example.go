package config

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Example is the annotated sample configuration written by WriteExample.
//
//go:embed config.example.yaml
var Example []byte

// WriteExample creates the sample configuration at path, refusing to
// overwrite an existing file.
func WriteExample(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s já existe", path)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, Example, 0o600)
}
