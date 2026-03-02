package security_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supportapplibs/go-lib/security"
)

func TestEncryptDecrypt(t *testing.T) {
	os.Setenv("SECURITY_KEY", "abcdefgh12345678")

	tests := []struct {
		name      string
		plaintext string
	}{
		{
			name:      "Simple String",
			plaintext: "Arnesya",
		},
		{
			name:      "Empty String",
			plaintext: "",
		},
		{
			name:      "single character",
			plaintext: "a",
		},
		{
			name:      "Long String",
			plaintext: "The moon beautiful, isn't it?",
		},
		{
			name:      "special characters",
			plaintext: "!@#$%^&*()_+-=[]{}|;':\",./<>?",
		},
		{
			name:      "Unicode characters",
			plaintext: "Hello 世界 🌍",
		},
		{
			name:      "Numeric string",
			plaintext: "1234567890",
		},
		{
			name:      "Whitespace",
			plaintext: "   spaces   \t\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encrypt
			ciphertext, err := security.Encrypt(tt.plaintext)
			require.NoError(t, err)
			assert.Greater(t, len(ciphertext), 32, "ciphertext should be longer than IV (32 hex chars)")

			// Decrypt
			decrypted, err := security.Decrypt(ciphertext)
			require.NoError(t, err)
			assert.Equal(t, tt.plaintext, decrypted)
		})
	}
}
