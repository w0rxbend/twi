package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrPathIsDirectory reports that a path expected to be a file is a directory.
var ErrPathIsDirectory = errors.New("path is a directory")

// CheckReadableFile verifies that path names an existing readable file.
func CheckReadableFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return ErrPathIsDirectory
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return file.Close()
}

// ProbeWritableDir verifies that dir exists and accepts a short private file
// write. The probe file is removed before the function returns.
func ProbeWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	probePath := filepath.Join(dir, ".twi-doctor-write-test")
	file, err := os.OpenFile(probePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		probePath = filepath.Join(dir, fmt.Sprintf(".twi-doctor-write-test-%d", os.Getpid()))
		file, err = os.OpenFile(probePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	}
	if err != nil {
		return err
	}
	if _, err := file.Write([]byte("ok\n")); err != nil {
		_ = file.Close()
		_ = os.Remove(probePath)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(probePath)
		return err
	}
	return os.Remove(probePath)
}
