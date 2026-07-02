package twitch

import (
	"context"
	"errors"
	"sync"
)

// ErrFakeTokenValidatorNoResult is returned when a fake has no queued or
// default outcome for a validation request.
var ErrFakeTokenValidatorNoResult = errors.New("fake token validator has no result")

// FakeTokenValidationOutcome is one queued fake validation response.
type FakeTokenValidationOutcome struct {
	Result TokenValidationResult
	Err    error
}

// FakeTokenValidator is an in-memory TokenValidator for app and CLI tests.
type FakeTokenValidator struct {
	mu sync.Mutex

	requests []TokenCredentials
	outcomes []FakeTokenValidationOutcome
	def      FakeTokenValidationOutcome
	hasDef   bool
}

var _ TokenValidator = (*FakeTokenValidator)(nil)

// NewFakeTokenValidator creates a fake validator with optional queued outcomes.
func NewFakeTokenValidator(outcomes ...FakeTokenValidationOutcome) *FakeTokenValidator {
	validator := &FakeTokenValidator{}
	validator.outcomes = append(validator.outcomes, outcomes...)
	return validator
}

// ValidateToken records credentials and returns the next queued or default
// outcome. It returns ctx.Err before recording when the context is canceled.
func (v *FakeTokenValidator) ValidateToken(ctx context.Context, credentials TokenCredentials) (TokenValidationResult, error) {
	if err := ctx.Err(); err != nil {
		return TokenValidationResult{}, err
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	v.requests = append(v.requests, credentials)
	if len(v.outcomes) > 0 {
		next := v.outcomes[0]
		v.outcomes = v.outcomes[1:]
		return next.Result, next.Err
	}
	if v.hasDef {
		return v.def.Result, v.def.Err
	}
	return TokenValidationResult{}, ErrFakeTokenValidatorNoResult
}

// Queue appends an outcome returned by a future ValidateToken call.
func (v *FakeTokenValidator) Queue(result TokenValidationResult, err error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.outcomes = append(v.outcomes, FakeTokenValidationOutcome{Result: result, Err: err})
}

// SetDefault sets the outcome returned after all queued outcomes are consumed.
func (v *FakeTokenValidator) SetDefault(result TokenValidationResult, err error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.def = FakeTokenValidationOutcome{Result: result, Err: err}
	v.hasDef = true
}

// Requests returns a copy of credential requests recorded by the fake. The
// returned values may contain secrets and should only be used in tests.
func (v *FakeTokenValidator) Requests() []TokenCredentials {
	v.mu.Lock()
	defer v.mu.Unlock()

	requests := make([]TokenCredentials, len(v.requests))
	copy(requests, v.requests)
	return requests
}
