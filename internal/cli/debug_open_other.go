//go:build !unix

package cli

import (
	"errors"
	"os"
)

const (
	debugLogOpenUsesNoFollow = false
	debugLogOpenPlatformNote = "Non-Unix debug log opening does not provide Unix O_NOFOLLOW or exact owner-only ACL guarantees; it rejects unsafe paths it can observe before and after open."
)

func openDebugLogFile(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_APPEND|os.O_WRONLY, debugLogFileMode)
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

	// Without a portable no-follow open flag, this fallback can only reject a
	// symlink that is visible before open and then validate the opened file.
	if err := validateDebugLogPath(path); err != nil {
		return nil, err
	}
	file, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return nil, debugLogOpenFileError(path, err)
	}
	if err := validateOpenedDebugLogFile(path, file); err != nil {
		return closeDebugLogFileWithError(file, err)
	}
	return file, nil
}

func debugLogOpenErrorIsSymlink(error) bool {
	return false
}
