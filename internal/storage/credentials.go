package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/w0rxbend/twi/internal/auth"
)

const (
	// CredentialFileRecordVersion is the JSON record version used by the
	// restrictive file fallback credential store.
	CredentialFileRecordVersion = 1
	// CredentialDirectoryMode is the mode used for directories that contain
	// persisted credential material.
	CredentialDirectoryMode fs.FileMode = 0o700
	// CredentialFileMode is the only mode used when creating fallback
	// credential files.
	CredentialFileMode fs.FileMode = 0o600
)

var (
	// ErrInsecureCredentialPermissions reports a file or directory mode that is
	// unsuitable for credential storage.
	ErrInsecureCredentialPermissions = errors.New("insecure credential permissions")
	// ErrUnsupportedCredentialFileFormat reports a credential record with an
	// unsupported schema version.
	ErrUnsupportedCredentialFileFormat = errors.New("unsupported credential file format")
)

// CredentialStore is the internal boundary for persisted Twitch OAuth
// credentials. Implementations must keep raw token values out of formatted
// errors, logs, diagnostics, and default structured output.
type CredentialStore interface {
	LoadCredentials(context.Context) (CredentialRecord, bool, error)
	SaveCredentials(context.Context, CredentialRecord) error
	DeleteCredentials(context.Context) error
}

// CredentialRecord is the storage-owned auth DTO. Tokens remain auth.Secret
// values so default fmt and JSON encoding redact them; fallback file storage
// must use MarshalCredentialFile to deliberately reveal token values.
type CredentialRecord struct {
	UserID       string
	Login        string
	DisplayName  string
	ClientID     string
	AccessToken  auth.Secret
	RefreshToken auth.Secret
	TokenType    string
	Scopes       []auth.Scope
	ExpiresAt    time.Time
	UpdatedAt    time.Time
}

// CredentialRecordFromLoginResult converts a completed login into the storage
// DTO without deciding where it will be persisted.
func CredentialRecordFromLoginResult(result auth.LoginResult, clientID string, updatedAt time.Time) CredentialRecord {
	scopes := result.Scopes
	if len(scopes) == 0 {
		scopes = result.Tokens.Scopes
	}
	return CredentialRecord{
		UserID:       result.Identity.UserID,
		Login:        result.Identity.Login,
		DisplayName:  result.Identity.DisplayName,
		ClientID:     clientID,
		AccessToken:  result.Tokens.AccessToken,
		RefreshToken: result.Tokens.RefreshToken,
		TokenType:    result.Tokens.TokenType,
		Scopes:       cloneCredentialScopes(scopes),
		ExpiresAt:    result.Tokens.ExpiresAt,
		UpdatedAt:    updatedAt,
	}
}

// Redactor returns an auth redactor configured with all secret values in the
// record.
func (r CredentialRecord) Redactor() auth.Redactor {
	return auth.NewRedactor(r.AccessToken, r.RefreshToken)
}

// Clone returns a deep copy of the record's mutable fields.
func (r CredentialRecord) Clone() CredentialRecord {
	r.Scopes = cloneCredentialScopes(r.Scopes)
	return r
}

// CredentialFilePlan describes the restrictive local JSON file fallback. This
// plan is intentionally separate from the current flat config file and from any
// future OS keychain backend.
type CredentialFilePlan struct {
	Path          string
	DirectoryMode fs.FileMode
	FileMode      fs.FileMode
	FormatVersion int
	Migration     CredentialMigration
}

// CredentialMigration documents how existing flat config/env credentials move
// into storage. T009 defines explicit migration only; T010 owns any file I/O.
type CredentialMigration string

const (
	// CredentialMigrationExplicitOnly means login/setup may save credentials
	// after user action, but config/env secrets are not copied automatically.
	CredentialMigrationExplicitOnly CredentialMigration = "explicit-only"
)

// DefaultCredentialFilePath returns the platform config-directory path for the
// fallback credential file.
func DefaultCredentialFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "twi", "credentials.json"), nil
}

// NewCredentialFilePlan returns the default fallback file plan for path. When
// path is empty, the platform config-directory credential path is used.
func NewCredentialFilePlan(path string) (CredentialFilePlan, error) {
	if path == "" {
		defaultPath, err := DefaultCredentialFilePath()
		if err != nil {
			return CredentialFilePlan{}, err
		}
		path = defaultPath
	}
	plan := CredentialFilePlan{
		Path:          path,
		DirectoryMode: CredentialDirectoryMode,
		FileMode:      CredentialFileMode,
		FormatVersion: CredentialFileRecordVersion,
		Migration:     CredentialMigrationExplicitOnly,
	}
	return plan, plan.Validate()
}

// Validate reports whether the plan uses a path, version, and restrictive
// modes suitable for storing credential material.
func (p CredentialFilePlan) Validate() error {
	if filepath.Clean(p.Path) == "." || p.Path == "" {
		return errors.New("credential file path is required")
	}
	if err := ValidateCredentialDirectoryMode(p.DirectoryMode); err != nil {
		return fmt.Errorf("credential directory mode: %w", err)
	}
	if err := ValidateCredentialFileMode(p.FileMode); err != nil {
		return fmt.Errorf("credential file mode: %w", err)
	}
	if p.FormatVersion != CredentialFileRecordVersion {
		return fmt.Errorf("%w: %d", ErrUnsupportedCredentialFileFormat, p.FormatVersion)
	}
	if p.Migration != CredentialMigrationExplicitOnly {
		return fmt.Errorf("unsupported credential migration policy: %s", p.Migration)
	}
	return nil
}

// ValidateCredentialDirectoryMode rejects credential directories that do not
// match the exact restrictive fallback directory mode.
func ValidateCredentialDirectoryMode(mode fs.FileMode) error {
	if mode.Perm() != CredentialDirectoryMode {
		return fmt.Errorf("%w: directory mode %s", ErrInsecureCredentialPermissions, mode.Perm())
	}
	return nil
}

// ValidateCredentialFileMode rejects credential files that do not match the
// exact restrictive fallback file mode.
func ValidateCredentialFileMode(mode fs.FileMode) error {
	if mode.Perm() != CredentialFileMode {
		return fmt.Errorf("%w: file mode %s", ErrInsecureCredentialPermissions, mode.Perm())
	}
	return nil
}

// MarshalCredentialFile encodes the raw fallback credential file. This is the
// storage-owned reveal path for auth.Secret values; callers must not use it for
// diagnostics, logs, or user-facing output.
func MarshalCredentialFile(record CredentialRecord) ([]byte, error) {
	data, err := json.MarshalIndent(credentialFileFromRecord(record), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// ParseCredentialFile decodes the fallback credential file format without
// exposing token values in errors.
func ParseCredentialFile(data []byte) (CredentialRecord, error) {
	var file credentialFileRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return CredentialRecord{}, fmt.Errorf("decode credential file: %w", err)
	}
	if file.Version != CredentialFileRecordVersion {
		return CredentialRecord{}, fmt.Errorf("%w: %d", ErrUnsupportedCredentialFileFormat, file.Version)
	}
	record, err := file.toRecord()
	if err != nil {
		return CredentialRecord{}, err
	}
	return record, nil
}

// MemoryCredentialStore is a stateful fake CredentialStore for tests.
type MemoryCredentialStore struct {
	mu        sync.RWMutex
	record    CredentialRecord
	present   bool
	loadErr   error
	saveErr   error
	deleteErr error
	saves     []CredentialRecord
	deletes   int
}

var _ CredentialStore = (*MemoryCredentialStore)(nil)

// NewMemoryCredentialStore returns an empty in-memory credential fake.
func NewMemoryCredentialStore() *MemoryCredentialStore {
	return &MemoryCredentialStore{}
}

// SetCredentials seeds the in-memory credential record.
func (s *MemoryCredentialStore) SetCredentials(record CredentialRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.record = record.Clone()
	s.present = true
}

// SetErrors configures errors returned by load, save, and delete operations.
func (s *MemoryCredentialStore) SetErrors(loadErr, saveErr, deleteErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadErr = loadErr
	s.saveErr = saveErr
	s.deleteErr = deleteErr
}

// SavedRecords returns snapshots of records passed to SaveCredentials.
func (s *MemoryCredentialStore) SavedRecords() []CredentialRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneCredentialRecords(s.saves)
}

// DeleteCount returns the number of successful delete calls.
func (s *MemoryCredentialStore) DeleteCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.deletes
}

func (s *MemoryCredentialStore) LoadCredentials(ctx context.Context) (CredentialRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return CredentialRecord{}, false, err
	}
	if s == nil {
		return CredentialRecord{}, false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.loadErr != nil {
		return CredentialRecord{}, false, s.loadErr
	}
	return s.record.Clone(), s.present, nil
}

func (s *MemoryCredentialStore) SaveCredentials(ctx context.Context, record CredentialRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	s.record = record.Clone()
	s.present = true
	s.saves = append(s.saves, record.Clone())
	return nil
}

func (s *MemoryCredentialStore) DeleteCredentials(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.record = CredentialRecord{}
	s.present = false
	s.deletes++
	return nil
}

// FailingCredentialStore is a fake CredentialStore that returns Err from every
// operation after honoring context cancellation.
type FailingCredentialStore struct {
	Err error
}

var _ CredentialStore = FailingCredentialStore{}

func (s FailingCredentialStore) LoadCredentials(ctx context.Context) (CredentialRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return CredentialRecord{}, false, err
	}
	return CredentialRecord{}, false, s.err()
}

func (s FailingCredentialStore) SaveCredentials(ctx context.Context, _ CredentialRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.err()
}

func (s FailingCredentialStore) DeleteCredentials(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.err()
}

func (s FailingCredentialStore) err() error {
	if s.Err != nil {
		return s.Err
	}
	return errors.New("credential store unavailable")
}

type credentialFileRecord struct {
	Version int                  `json:"version"`
	Twitch  credentialFileTwitch `json:"twitch"`
}

type credentialFileTwitch struct {
	UserID       string   `json:"user_id,omitempty"`
	Login        string   `json:"login,omitempty"`
	DisplayName  string   `json:"display_name,omitempty"`
	ClientID     string   `json:"client_id,omitempty"`
	AccessToken  string   `json:"access_token,omitempty"`
	RefreshToken string   `json:"refresh_token,omitempty"`
	TokenType    string   `json:"token_type,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	ExpiresAt    string   `json:"expires_at,omitempty"`
	UpdatedAt    string   `json:"updated_at,omitempty"`
}

func credentialFileFromRecord(record CredentialRecord) credentialFileRecord {
	return credentialFileRecord{
		Version: CredentialFileRecordVersion,
		Twitch: credentialFileTwitch{
			UserID:       record.UserID,
			Login:        record.Login,
			DisplayName:  record.DisplayName,
			ClientID:     record.ClientID,
			AccessToken:  record.AccessToken.Reveal(),
			RefreshToken: record.RefreshToken.Reveal(),
			TokenType:    record.TokenType,
			Scopes:       auth.ScopeValues(record.Scopes),
			ExpiresAt:    formatCredentialTime(record.ExpiresAt),
			UpdatedAt:    formatCredentialTime(record.UpdatedAt),
		},
	}
}

func (f credentialFileRecord) toRecord() (CredentialRecord, error) {
	expiresAt, err := parseCredentialTime("expires_at", f.Twitch.ExpiresAt)
	if err != nil {
		return CredentialRecord{}, err
	}
	updatedAt, err := parseCredentialTime("updated_at", f.Twitch.UpdatedAt)
	if err != nil {
		return CredentialRecord{}, err
	}
	return CredentialRecord{
		UserID:       f.Twitch.UserID,
		Login:        f.Twitch.Login,
		DisplayName:  f.Twitch.DisplayName,
		ClientID:     f.Twitch.ClientID,
		AccessToken:  auth.NewSecret(f.Twitch.AccessToken),
		RefreshToken: auth.NewSecret(f.Twitch.RefreshToken),
		TokenType:    f.Twitch.TokenType,
		Scopes:       auth.Scopes(f.Twitch.Scopes...),
		ExpiresAt:    expiresAt,
		UpdatedAt:    updatedAt,
	}, nil
}

func formatCredentialTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339Nano)
}

func parseCredentialTime(field, value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid credential %s", field)
	}
	return parsed, nil
}

func cloneCredentialScopes(scopes []auth.Scope) []auth.Scope {
	if len(scopes) == 0 {
		return nil
	}
	return append([]auth.Scope(nil), scopes...)
}

func cloneCredentialRecords(records []CredentialRecord) []CredentialRecord {
	if len(records) == 0 {
		return nil
	}
	clones := make([]CredentialRecord, len(records))
	for i, record := range records {
		clones[i] = record.Clone()
	}
	return clones
}
