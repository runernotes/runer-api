package validator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testStruct is a minimal struct used solely to exercise the validator in tests.
// It mirrors the validation tag patterns used across the application (required + email).
type testStruct struct {
	Email string `validate:"required,email"`
	Name  string `validate:"required"`
}

// TestValidate_ValidStruct_NoError verifies that a fully populated, valid struct
// passes validation without error.
func TestValidate_ValidStruct_NoError(t *testing.T) {
	v := New()
	s := testStruct{Email: "alice@example.com", Name: "Alice"}
	require.NoError(t, v.Validate(s))
}

// TestValidate_MissingRequiredField_ReturnsError verifies that an empty required
// field causes validation to return a non-nil error.
func TestValidate_MissingRequiredField_ReturnsError(t *testing.T) {
	v := New()
	// Name is missing — must be caught by the "required" tag.
	s := testStruct{Email: "alice@example.com", Name: ""}
	assert.Error(t, v.Validate(s))
}

// TestValidate_InvalidEmail_ReturnsError verifies that the "email" tag rejects
// strings that are not valid email addresses.
func TestValidate_InvalidEmail_ReturnsError(t *testing.T) {
	v := New()
	s := testStruct{Email: "not-an-email", Name: "Alice"}
	assert.Error(t, v.Validate(s))
}

// TestValidate_AllFieldsMissing_ReturnsError verifies that a zero-value struct
// with all required fields absent is rejected.
func TestValidate_AllFieldsMissing_ReturnsError(t *testing.T) {
	v := New()
	var s testStruct
	assert.Error(t, v.Validate(s))
}
