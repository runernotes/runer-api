package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestComputeSHA256_KnownInput verifies the function produces the correct
// hex-encoded SHA-256 digest for a well-known input value.
func TestComputeSHA256_KnownInput(t *testing.T) {
	// SHA-256("hello") is a well-known test vector.
	got := ComputeSHA256("hello")
	assert.Equal(t, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", got)
}

// TestComputeSHA256_EmptyInput verifies the function handles an empty string
// and returns the known SHA-256 digest for the empty message.
func TestComputeSHA256_EmptyInput(t *testing.T) {
	got := ComputeSHA256("")
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", got)
}

// TestComputeSHA256_Deterministic verifies that calling the function twice with
// the same input always produces the same output.
func TestComputeSHA256_Deterministic(t *testing.T) {
	input := "some-opaque-token-value"
	first := ComputeSHA256(input)
	second := ComputeSHA256(input)
	assert.Equal(t, first, second, "SHA-256 must be deterministic for the same input")
}

// TestComputeSHA256_DifferentInputs_DifferentOutput verifies that two distinct
// inputs produce different digests (collision resistance sanity check).
func TestComputeSHA256_DifferentInputs_DifferentOutput(t *testing.T) {
	a := ComputeSHA256("token-a")
	b := ComputeSHA256("token-b")
	assert.NotEqual(t, a, b, "different inputs must produce different SHA-256 digests")
}
