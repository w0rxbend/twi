# 0007: Use Windows Credential Manager For Non-Unix Credentials

## Status

Accepted. Implemented by T010 for Windows builds.

## Context

`twi` currently persists OAuth credentials through `internal/storage.CredentialStore`.
The implemented local fallback is a Unix-only credential file with exact `0700`
directory permissions, exact `0600` file permissions, symlink rejection, and
no-follow opens for existing files.

The release target matrix currently builds Linux, macOS, and Windows binaries.
Linux and macOS are Go `unix` builds in this project and stay on the existing
file fallback. Windows is the only current release target that is non-Unix, and
the file fallback is disabled there because the project does not implement
Windows owner-only DACL checks, inheritance control, or reparse-point-safe file
opens.

The non-Unix decision must not create a weaker credential path just to make
`twi login` work on another platform. Any enabled backend must have a clear
owner-only access story, avoid path replacement attacks, fit the existing
storage boundary, and be testable without leaking Twitch tokens.

Relevant Windows platform facts from Microsoft documentation:

- `CredWrite` creates or updates a credential in the current user's credential
  set and associates it with the current token's logon session.
- `CredRead` reads a credential from the current user's credential set and
  requires callers to free the returned credential buffer.
- `CRED_TYPE_GENERIC` stores an application-defined credential blob securely
  without handing it to Windows authentication packages.
- `CRED_PERSIST_LOCAL_MACHINE` persists the credential for later logon sessions
  by the same user on the same computer, without roaming it to other computers.
- Windows reparse points are a file-system behavior; applications that open
  reparse-point paths need explicit handling such as `FILE_FLAG_OPEN_REPARSE_POINT`.

References:

- <https://learn.microsoft.com/en-us/windows/win32/api/wincred/nf-wincred-credwritea>
- <https://learn.microsoft.com/en-us/windows/win32/api/wincred/nf-wincred-credreadw>
- <https://learn.microsoft.com/en-us/windows/win32/api/wincred/ns-wincred-credentiala>
- <https://learn.microsoft.com/en-us/windows/win32/fileio/reparse-points-and-file-operations>
- <https://support.microsoft.com/en-us/windows/security/credential-manager-in-windows>

## Decision

Keep the existing Unix credential-file fallback for Linux and macOS. For
Windows, use the native Windows Credential Manager backend. Do not implement a
Windows JSON credential file for OAuth secrets.

The Windows backend must:

- implement `storage.CredentialStore` behind Windows build tags;
- use `CredWriteW`, `CredReadW`, `CredDeleteW`, and `CredFree` through
  Advapi32;
- store one `CRED_TYPE_GENERIC` credential under a stable `twi` target name,
  currently planned as `w0rxbend/twi/twitch-oauth`;
- use `CRED_PERSIST_LOCAL_MACHINE` so credentials persist across later logon
  sessions for the same Windows user on the same computer without requesting
  enterprise roaming;
- serialize only the storage-owned versioned Twitch credential payload in the
  credential blob;
- keep token values typed as `auth.Secret` outside the explicit storage reveal
  path;
- treat `ERROR_NOT_FOUND` as an empty store on load and delete;
- wrap all provider errors with the same redaction discipline used by the Unix
  fallback;
- keep environment variables and flat config credentials ahead of saved
  credentials in precedence.

Owner-only access is provided by the current user's Windows credential set
rather than by a project-managed file ACL. `twi` will not claim protection
against the same Windows user, local administrators, credential-export tools,
malware running in the user's session, or enterprise policy that permits
credential inspection. Users can view and delete the stored entry through
Windows Credential Manager.

DACL inheritance and reparse-point/no-follow requirements do not apply to the
selected Windows backend because it does not create, open, or delete a
user-controlled credential file path. T010 did not enable the current
credential-file fallback on Windows. If a future task proposes a Windows file
backend instead, it must be a separate decision with exact DACL, owner SID,
inheritance, final-handle validation, and reparse-point semantics before code
is enabled.

Other non-Unix GOOS targets are deferred. Adding a new non-Unix release target
must either select and implement a native credential backend with equivalent
support guarantees or keep saved credential persistence disabled for that
target.

## Dependency Tradeoffs

Do not add a broad third-party cross-platform keychain wrapper for the Windows
slice. The preferred implementation is a small Windows-specific adapter using
the Win32 Credential Manager API through `golang.org/x/sys/windows` or a
minimal syscall layer. `golang.org/x/sys` is already present transitively; if
the Windows backend imports it directly, keep `go.mod` and `go.sum` managed by
`go mod tidy`.

This keeps Linux/macOS behavior stable, avoids pulling in unrelated keychain
backends or desktop prompt behavior, and makes the Windows support surface small
enough to test and review.

## Testing And Support

T010 includes:

- deterministic unit tests for payload serialization, redaction, missing
  credentials, provider failures, save/load/delete behavior, overwrite behavior,
  and refresh-token persistence through the Windows store interface;
- Windows build-tag tests around a fake Wincred adapter so most behavior is
  testable without a real Windows host or real Twitch credentials;
- a Windows compile gate for `internal/storage`, `internal/cli`,
  `internal/config`, and `internal/auth`;
- a documented Windows-host smoke that writes a namespaced test credential,
  reads it, deletes it, verifies it is gone, and never stores real Twitch
  tokens;
- CLI tests proving `twi login` persists on Windows only after the backend is
  available and that unsupported non-Unix platforms still fail before OAuth
  starts.

Real Windows-host execution of the native smoke remains environment-dependent.
The opt-in smoke test uses a namespaced fake Twitch credential record and never
requires real Twitch credentials.

## Consequences

- Windows gets a concrete implementation without weakening the current storage
  boundary.
- The current non-Unix file fallback remains disabled.
- Linux and macOS credential-file behavior remains unchanged.
- Cross-platform support claims stay conservative until platform tests exist.
- Users on unsupported non-Unix targets must use env/config credentials until a
  backend is selected and implemented for that target.
