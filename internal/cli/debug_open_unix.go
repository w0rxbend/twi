//go:build unix

package cli

import (
	"errors"
	"os"
	"syscall"
)

const (
	debugLogOpenUsesNoFollow = true
	debugLogOpenPlatformNote = "Unix debug log files are opened with O_NOFOLLOW on the final path and validated through the opened file descriptor."
)

func openDebugLogFile(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_APPEND|os.O_WRONLY|syscall.O_NOFOLLOW, debugLogFileMode)
	if err == nil {
		if err := file.Chmod(debugLogFileMode); err != nil {
			return closeDebugLogFileWithError(file, debugLogOperationError("set permissions on", path, err))
		}
		if err := validateOpenedDebugLogFile(path, file); err != nil {
			return closeDebugLogFileWithError(file, err)
		}
		return file, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, debugLogOpenFileError(path, err)
	}

	file, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, debugLogOpenFileError(path, err)
	}
	if err := validateOpenedDebugLogFile(path, file); err != nil {
		return closeDebugLogFileWithError(file, err)
	}
	return file, nil
}

func debugLogOpenErrorIsSymlink(err error) bool {
	return errors.Is(err, syscall.ELOOP)
}
