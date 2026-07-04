//go:build windows

package storage

import (
	"context"
	"errors"
	"fmt"
	"unsafe"

	"github.com/w0rxbend/twi/internal/auth"
	"golang.org/x/sys/windows"
)

const (
	// WindowsCredentialTargetName is the stable Windows Credential Manager
	// target name used for Twitch OAuth credentials.
	WindowsCredentialTargetName = "w0rxbend/twi/twitch-oauth"
	// WindowsCredentialTypeGeneric is the Win32 CRED_TYPE_GENERIC value.
	WindowsCredentialTypeGeneric uint32 = 1
	// WindowsCredentialPersistLocalMachine is the Win32
	// CRED_PERSIST_LOCAL_MACHINE value.
	WindowsCredentialPersistLocalMachine uint32 = 2

	maxWindowsCredentialBlobBytes = 5 * 512
)

// WindowsCredentialManagerStore persists Twitch OAuth credentials through the
// current user's native Windows Credential Manager credential set.
type WindowsCredentialManagerStore struct {
	target   string
	provider windowsCredentialProvider
}

var _ CredentialStore = (*WindowsCredentialManagerStore)(nil)

// NewWindowsCredentialManagerStore creates the default Windows Credential
// Manager store.
func NewWindowsCredentialManagerStore() (*WindowsCredentialManagerStore, error) {
	return newWindowsCredentialManagerStoreWithProvider(WindowsCredentialTargetName, nativeWindowsCredentialProvider{})
}

func newWindowsCredentialManagerStoreWithProvider(target string, provider windowsCredentialProvider) (*WindowsCredentialManagerStore, error) {
	if target == "" {
		return nil, errors.New("Windows Credential Manager target name is required")
	}
	if provider == nil {
		return nil, errors.New("Windows Credential Manager provider is required")
	}
	return &WindowsCredentialManagerStore{target: target, provider: provider}, nil
}

// TargetName returns the Windows Credential Manager target name.
func (s *WindowsCredentialManagerStore) TargetName() string {
	if s == nil {
		return ""
	}
	return s.target
}

// StoreLabel returns the user-facing name for diagnostics.
func (s *WindowsCredentialManagerStore) StoreLabel() string {
	return "Windows Credential Manager"
}

// StoreLocation returns the user-facing storage target for diagnostics.
func (s *WindowsCredentialManagerStore) StoreLocation() string {
	if s == nil {
		return ""
	}
	return "target " + s.target
}

// LoadCredentials reads and parses the versioned Twitch credential payload
// from Windows Credential Manager.
func (s *WindowsCredentialManagerStore) LoadCredentials(ctx context.Context) (CredentialRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return CredentialRecord{}, false, err
	}
	if s == nil {
		return CredentialRecord{}, false, nil
	}

	data, ok, err := s.provider.Read(s.target, WindowsCredentialTypeGeneric)
	if err != nil {
		return CredentialRecord{}, false, credentialOperationError("load Windows credential", s.target, err, auth.Redactor{})
	}
	if !ok {
		return CredentialRecord{}, false, nil
	}
	if len(data) > maxWindowsCredentialBlobBytes {
		return CredentialRecord{}, false, credentialMalformedPayloadError("load Windows credential", s.target, errors.New("credential blob is too large"))
	}
	record, err := ParseCredentialFile(data)
	if err != nil {
		return CredentialRecord{}, false, credentialMalformedPayloadError("load Windows credential", s.target, err)
	}
	return record, true, nil
}

// SaveCredentials writes the versioned Twitch credential payload to Windows
// Credential Manager with local-machine persistence for the current user.
func (s *WindowsCredentialManagerStore) SaveCredentials(ctx context.Context, record CredentialRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return nil
	}

	data, err := MarshalCredentialFile(record)
	if err != nil {
		return credentialOperationError("marshal Windows credential", s.target, err, record.Redactor())
	}
	if len(data) > maxWindowsCredentialBlobBytes {
		return credentialOperationError("save Windows credential", s.target, fmt.Errorf("%w: credential blob exceeds Windows Credential Manager generic credential limit", ErrMalformedCredentialFile), record.Redactor())
	}
	if err := s.provider.Write(windowsCredentialWriteRequest{
		TargetName: s.target,
		Type:       WindowsCredentialTypeGeneric,
		Persist:    WindowsCredentialPersistLocalMachine,
		Blob:       data,
	}); err != nil {
		return credentialOperationError("save Windows credential", s.target, err, record.Redactor())
	}
	return nil
}

// DeleteCredentials removes the Windows Credential Manager entry. Missing
// credentials are treated as an empty store.
func (s *WindowsCredentialManagerStore) DeleteCredentials(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return nil
	}
	if _, err := s.provider.Delete(s.target, WindowsCredentialTypeGeneric); err != nil {
		return credentialOperationError("delete Windows credential", s.target, err, auth.Redactor{})
	}
	return nil
}

type windowsCredentialProvider interface {
	Write(windowsCredentialWriteRequest) error
	Read(targetName string, typ uint32) ([]byte, bool, error)
	Delete(targetName string, typ uint32) (bool, error)
}

type windowsCredentialWriteRequest struct {
	TargetName string
	Type       uint32
	Persist    uint32
	Blob       []byte
}

type nativeWindowsCredentialProvider struct{}

func (nativeWindowsCredentialProvider) Write(req windowsCredentialWriteRequest) error {
	targetName, err := windows.UTF16PtrFromString(req.TargetName)
	if err != nil {
		return err
	}
	userName, err := windows.UTF16PtrFromString("twi")
	if err != nil {
		return err
	}

	var blob *byte
	if len(req.Blob) > 0 {
		blob = &req.Blob[0]
	}
	credential := windowsCredential{
		Type:               req.Type,
		TargetName:         targetName,
		CredentialBlobSize: uint32(len(req.Blob)),
		CredentialBlob:     blob,
		Persist:            req.Persist,
		UserName:           userName,
	}

	r1, _, callErr := procCredWriteW.Call(uintptr(unsafe.Pointer(&credential)), 0)
	if r1 == 0 {
		return windowsCredentialCallError(callErr)
	}
	return nil
}

func (nativeWindowsCredentialProvider) Read(targetName string, typ uint32) ([]byte, bool, error) {
	targetNamePtr, err := windows.UTF16PtrFromString(targetName)
	if err != nil {
		return nil, false, err
	}

	var credential *windowsCredential
	r1, _, callErr := procCredReadW.Call(
		uintptr(unsafe.Pointer(targetNamePtr)),
		uintptr(typ),
		0,
		uintptr(unsafe.Pointer(&credential)),
	)
	if r1 == 0 {
		err := windowsCredentialCallError(callErr)
		if windowsCredentialNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(credential)))

	if credential == nil {
		return nil, false, errors.New("Windows Credential Manager returned an empty credential pointer")
	}
	size := int(credential.CredentialBlobSize)
	if size < 0 || uint32(size) != credential.CredentialBlobSize {
		return nil, false, errors.New("Windows Credential Manager returned an invalid credential blob size")
	}
	if size == 0 {
		return nil, true, nil
	}
	if credential.CredentialBlob == nil {
		return nil, false, errors.New("Windows Credential Manager returned a nil credential blob")
	}
	data := unsafe.Slice(credential.CredentialBlob, size)
	return append([]byte(nil), data...), true, nil
}

func (nativeWindowsCredentialProvider) Delete(targetName string, typ uint32) (bool, error) {
	targetNamePtr, err := windows.UTF16PtrFromString(targetName)
	if err != nil {
		return false, err
	}

	r1, _, callErr := procCredDeleteW.Call(
		uintptr(unsafe.Pointer(targetNamePtr)),
		uintptr(typ),
		0,
	)
	if r1 == 0 {
		err := windowsCredentialCallError(callErr)
		if windowsCredentialNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

type windowsCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

var (
	modadvapi32     = windows.NewLazySystemDLL("advapi32.dll")
	procCredWriteW  = modadvapi32.NewProc("CredWriteW")
	procCredReadW   = modadvapi32.NewProc("CredReadW")
	procCredDeleteW = modadvapi32.NewProc("CredDeleteW")
	procCredFree    = modadvapi32.NewProc("CredFree")
)

func windowsCredentialCallError(err error) error {
	if err == nil || errors.Is(err, windows.ERROR_SUCCESS) {
		return errors.New("Windows Credential Manager call failed")
	}
	return err
}

func windowsCredentialNotFound(err error) bool {
	return errors.Is(err, windows.ERROR_NOT_FOUND)
}
