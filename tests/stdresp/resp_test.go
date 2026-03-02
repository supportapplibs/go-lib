package stdresp

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/supportapplibs/go-lib/stdresp"
)

// Test helper to create a Std instance
func newStd() *stdresp.Std {
	return &stdresp.Std{}
}

func TestWithDesc(t *testing.T) {
	tests := []struct {
		name string
		desc string
	}{
		{"empty description", ""},
		{"simple description", "Success"},
		{"long description", "This is a very long description with multiple words"},
		{"special characters", "Error: 404 - Not Found!"},
		{"unicode", "Terjadi kesalahan ❌"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			std := newStd()
			opt := stdresp.WithDesc(tt.desc)
			opt(std)
			assert.Equal(t, tt.desc, std.ResponseDesc)
		})
	}
}

func TestWithMaskedData(t *testing.T) {
	tests := []struct {
		name       string
		keysToMask []string
		data       any
		validate   func(t *testing.T, result any)
	}{
		{
			name:       "nil data returns nil",
			keysToMask: []string{"password"},
			data:       nil,
			validate: func(t *testing.T, result any) {
				assert.Nil(t, result)
			},
		},
		{
			name:       "empty keys returns unchanged",
			keysToMask: []string{},
			data:       map[string]string{"key": "value"},
			validate: func(t *testing.T, result any) {
				assert.NotNil(t, result)
			},
		},
		{
			name:       "mask single field",
			keysToMask: []string{"password"},
			data: map[string]any{
				"username": "john",
				"password": "secret123",
			},
			validate: func(t *testing.T, result any) {
				m := result.(map[string]any)
				assert.Equal(t, "john", m["username"])
				assert.NotEqual(t, "secret123", m["password"])
			},
		},
		{
			name:       "mask multiple fields",
			keysToMask: []string{"password", "token"},
			data: map[string]any{
				"username": "john",
				"password": "secret123",
				"token":    "abc123xyz",
				"email":    "john@test.com",
			},
			validate: func(t *testing.T, result any) {
				m := result.(map[string]any)
				assert.Equal(t, "john", m["username"])
				assert.NotEqual(t, "secret123", m["password"])
				assert.NotEqual(t, "abc123xyz", m["token"])
				assert.Equal(t, "john@test.com", m["email"])
			},
		},
		{
			name:       "case insensitive matching",
			keysToMask: []string{"PASSWORD"},
			data: map[string]any{
				"password": "secret",
				"Password": "secret2",
				"PASSWORD": "secret3",
			},
			validate: func(t *testing.T, result any) {
				m := result.(map[string]any)
				assert.NotEqual(t, "secret", m["password"])
				assert.NotEqual(t, "secret2", m["Password"])
				assert.NotEqual(t, "secret3", m["PASSWORD"])
			},
		},
		{
			name:       "nested object masking",
			keysToMask: []string{"password"},
			data: map[string]any{
				"user": map[string]any{
					"name":     "john",
					"password": "nested_secret",
				},
			},
			validate: func(t *testing.T, result any) {
				m := result.(map[string]any)
				user := m["user"].(map[string]any)
				assert.Equal(t, "john", user["name"])
				assert.NotEqual(t, "nested_secret", user["password"])
			},
		},
		{
			name:       "deeply nested masking",
			keysToMask: []string{"secret"},
			data: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": map[string]any{
							"secret": "deep_value",
							"public": "visible",
						},
					},
				},
			},
			validate: func(t *testing.T, result any) {
				m := result.(map[string]any)
				l1 := m["level1"].(map[string]any)
				l2 := l1["level2"].(map[string]any)
				l3 := l2["level3"].(map[string]any)
				assert.Equal(t, "visible", l3["public"])
				assert.NotEqual(t, "deep_value", l3["secret"])
			},
		},
		{
			name:       "array with objects",
			keysToMask: []string{"password"},
			data: map[string]any{
				"users": []any{
					map[string]any{
						"name":     "john",
						"password": "pass1",
					},
					map[string]any{
						"name":     "jane",
						"password": "pass2",
					},
				},
			},
			validate: func(t *testing.T, result any) {
				m := result.(map[string]any)
				users := m["users"].([]any)
				user1 := users[0].(map[string]any)
				user2 := users[1].(map[string]any)
				assert.NotEqual(t, "pass1", user1["password"])
				assert.NotEqual(t, "pass2", user2["password"])
			},
		},
		{
			name:       "empty string not masked",
			keysToMask: []string{"password"},
			data: map[string]any{
				"password": "",
			},
			validate: func(t *testing.T, result any) {
				m := result.(map[string]any)
				assert.Equal(t, "", m["password"])
			},
		},
		{
			name:       "non-string values not masked",
			keysToMask: []string{"count", "active"},
			data: map[string]any{
				"count":  42,
				"active": true,
			},
			validate: func(t *testing.T, result any) {
				m := result.(map[string]any)
				assert.Equal(t, float64(42), m["count"])
				assert.Equal(t, true, m["active"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			std := newStd()
			opt := stdresp.WithMaskedData(tt.keysToMask, tt.data)
			opt(std)
			tt.validate(t, std.ResponseData)
		})
	}
}

func TestWithErrThirdParty(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "with error message",
			err:     errors.New("third party service unavailable"),
			wantMsg: "third party service unavailable",
		},
		{
			name:    "empty error message",
			err:     errors.New(""),
			wantMsg: "Terjadi kesalahan, mohon coba beberapa saat lagi yaa... ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			std := newStd()
			opt := stdresp.WithErrThirdParty(tt.err)
			opt(std)

			assert.NotEmpty(t, std.ResponseCode)
			assert.Equal(t, tt.wantMsg, std.ResponseDesc)
		})
	}
}

func TestWithErrNotFound(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "with custom message",
			err:     errors.New("user not found"),
			wantMsg: "user not found",
		},
		{
			name:    "default message",
			err:     errors.New(""),
			wantMsg: "Data tidak ditemukan",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			std := newStd()
			opt := stdresp.WithErrNotFound(tt.err)
			opt(std)

			assert.NotEmpty(t, std.ResponseCode)
			assert.Equal(t, tt.wantMsg, std.ResponseDesc)
		})
	}
}

func TestWithErrAccess(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "with custom message",
			err:     errors.New("permission denied"),
			wantMsg: "permission denied",
		},
		{
			name:    "default message",
			err:     errors.New(""),
			wantMsg: "Akses tidak diijinkan",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			std := newStd()
			opt := stdresp.WithErrAccess(tt.err)
			opt(std)

			assert.NotEmpty(t, std.ResponseCode)
			assert.Equal(t, tt.wantMsg, std.ResponseDesc)
		})
	}
}

func TestWithErrHeader(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "with custom message",
			err:     errors.New("missing authorization header"),
			wantMsg: "missing authorization header",
		},
		{
			name:    "default message",
			err:     errors.New(""),
			wantMsg: "Header Request tidak valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			std := newStd()
			opt := stdresp.WithErrHeader(tt.err)
			opt(std)

			assert.NotEmpty(t, std.ResponseCode)
			assert.Equal(t, tt.wantMsg, std.ResponseDesc)
		})
	}
}

func TestWithErrInvalidRule(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "with custom message",
			err:     errors.New("validation failed"),
			wantMsg: "validation failed",
		},
		{
			name:    "default message",
			err:     errors.New(""),
			wantMsg: "Validasi tidak terpenuhi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			std := newStd()
			opt := stdresp.WithErrInvalidRule(tt.err)
			opt(std)

			assert.NotEmpty(t, std.ResponseCode)
			assert.Equal(t, tt.wantMsg, std.ResponseDesc)
		})
	}
}

func TestWithErrParameter(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "with custom message",
			err:     errors.New("invalid parameter format"),
			wantMsg: "invalid parameter format",
		},
		{
			name:    "default message",
			err:     errors.New(""),
			wantMsg: "Parameter tidak sesuai",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			std := newStd()
			opt := stdresp.WithErrParameter(tt.err)
			opt(std)

			assert.NotEmpty(t, std.ResponseCode)
			assert.Equal(t, tt.wantMsg, std.ResponseDesc)
		})
	}
}

func TestWithErrRuntime(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "with custom message",
			err:     errors.New("runtime panic recovered"),
			wantMsg: "runtime panic recovered",
		},
		{
			name:    "default message",
			err:     errors.New(""),
			wantMsg: "Runtime error happens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			std := newStd()
			opt := stdresp.WithErrRuntime(tt.err)
			opt(std)

			assert.NotEmpty(t, std.ResponseCode)
			assert.Equal(t, tt.wantMsg, std.ResponseDesc)
		})
	}
}

func TestWithErrUnsupport(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "with custom message",
			err:     errors.New("feature not supported"),
			wantMsg: "feature not supported",
		},
		{
			name:    "default message",
			err:     errors.New(""),
			wantMsg: "Tidak disupport",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			std := newStd()
			opt := stdresp.WithErrUnsupport(tt.err)
			opt(std)

			assert.NotEmpty(t, std.ResponseCode)
			assert.Equal(t, tt.wantMsg, std.ResponseDesc)
		})
	}
}

func TestWithErrWaiting(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "with custom message",
			err:     errors.New("processing in progress"),
			wantMsg: "processing in progress",
		},
		{
			name:    "default message",
			err:     errors.New(""),
			wantMsg: "Masih proses harap tunggu",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			std := newStd()
			opt := stdresp.WithErrWaiting(tt.err)
			opt(std)

			assert.NotEmpty(t, std.ResponseCode)
			assert.Equal(t, tt.wantMsg, std.ResponseDesc)
		})
	}
}

// Benchmark tests
func BenchmarkWithDesc(b *testing.B) {
	std := newStd()
	desc := "Test description"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		opt := stdresp.WithDesc(desc)
		opt(std)
	}
}

func BenchmarkWithMaskedData_Simple(b *testing.B) {
	data := map[string]any{
		"username": "john",
		"password": "secret123",
	}
	keys := []string{"password"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		std := newStd()
		opt := stdresp.WithMaskedData(keys, data)
		opt(std)
	}
}

func BenchmarkWithMaskedData_Complex(b *testing.B) {
	data := map[string]any{
		"user": map[string]any{
			"profile": map[string]any{
				"password": "secret",
				"token":    "token123",
			},
		},
		"items": []any{
			map[string]any{"password": "pass1"},
			map[string]any{"password": "pass2"},
		},
	}
	keys := []string{"password", "token"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		std := newStd()
		opt := stdresp.WithMaskedData(keys, data)
		opt(std)
	}
}
