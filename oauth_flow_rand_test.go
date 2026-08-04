package warmbly

import (
	"crypto/rand"
	"errors"
	"testing"
)

// randFailReader always fails, used to drive the crypto/rand failure branch by
// temporarily replacing rand.Reader.
type randFailReader struct{}

func (randFailReader) Read([]byte) (int, error) { return 0, errors.New("warmbly test: rand failure") }

// TestOAuthFlowGenerateVerifierPanicsOnRandError covers GenerateVerifier's
// defensive panic when crypto/rand cannot supply entropy.
func TestOAuthFlowGenerateVerifierPanicsOnRandError(t *testing.T) {
	old := rand.Reader
	rand.Reader = randFailReader{}
	defer func() { rand.Reader = old }()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected GenerateVerifier to panic when crypto/rand fails")
		}
	}()
	_ = GenerateVerifier()
}
