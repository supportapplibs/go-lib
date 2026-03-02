package stdresp

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/goravel/framework/facades"
	"github.com/supportapplibs/go-lib/debug"
	"github.com/supportapplibs/go-lib/security"
	"github.com/supportapplibs/go-lib/stderr"
)

// StdOpt option signature that accept and modify Std response object.
type StdOpt func(std *Std)

// WithDesc embed given string data to the standard response as field `responseDesc`.
func WithDesc(desc string) StdOpt {
	return func(s *Std) {
		s.ResponseDesc = desc
	}
}

// WithData embed given data object to the standard response as field `responseData`.
func WithData(data any) StdOpt {
	return func(s *Std) {

		switch strings.ToUpper(facades.Config().GetString("ENCRYPTION_MODE", "")) {
		case "":
			s.ResponseData = data
		case "DISABLED":
			s.ResponseData = data
		case "ENABLED":
			keyToEncryptStr := facades.Config().GetString("KEY_TO_ENCRYPT")
			var keyToEncrypt []string
			if keyToEncryptStr != "" {
				keyToEncrypt = strings.Split(keyToEncryptStr, ",")
			}
			s.ResponseData = maskData(keyToEncrypt, data)
		default:
			s.ResponseCode = stderr.ERROR_CODE_SYSTEM
			s.ResponseDesc = "Invalid ENCRYPTION_MODE: must be empty, 'ENABLED', or 'DISABLED'"
			s.ResponseData = nil
		}
	}
}

// WithMaskedData masks specified keys in the response data
func WithMaskedData(keysToMask []string, data any) StdOpt {
	return func(s *Std) {
		s.ResponseData = maskData(keysToMask, data)
	}
}

// WithErr embed given error to the standard response.
func WithErr(e error) StdOpt {
	fmt.Println("Test:", stderr.IsStdError(e))
	return func(s *Std) {
		// set default response code and description
		s.ResponseCode = stderr.ERROR_CODE_SYSTEM
		s.ResponseDesc = "Terjadi kesalahan, mohon coba beberapa saat lagi yaa... "

		fmt.Println("Error:", e)
		fmt.Println("Test:", stderr.IsStdError(e))
		// check if the error is created using stderr pkg
		if stderr.IsStdError(e) {
			// override the code and description
			s.ResponseCode = stderr.GetCode(e)

			// do trim space in case stacktrace is empty
			if (s.ResponseCode == stderr.ERROR_CODE_SYSTEM && debug.IsOn()) || s.ResponseCode != stderr.ERROR_CODE_SYSTEM {
				if facades.Config().GetBool("APP_DEBUG") {
					s.ResponseDesc = strings.TrimSpace(stderr.GetMsg(e) + " " + stderr.GetStackTrace(e))
				} else {
					s.ResponseDesc = strings.TrimSpace(stderr.GetMsg(e))
				}
			}

			// also if any, get the validation message too
			s.ResponseValidation = stderr.GetValidationErrorMsg(e)

			// get the http code
			s.httpCode = stderr.GetHttpCode(e)
			return
		}

		// capture in case its random error that's not constructed with stderr pkg
		//  but only print if the debug flag is on
		if debug.IsOn() {
			s.ResponseDesc = e.Error()
		}
	}
}

func WithErrThirdParty(e error) StdOpt {
	return func(s *Std) {
		s.ResponseCode = stderr.ERROR_CODE_THIRD_PARTY
		if e.Error() == "" {
			s.ResponseDesc = "Terjadi kesalahan, mohon coba beberapa saat lagi yaa... "
		} else {
			s.ResponseDesc = e.Error()
		}
	}
}

func WithErrNotFound(e error) StdOpt {
	return func(s *Std) {
		s.ResponseCode = stderr.ERROR_CODE_DATA_NOT_FOUND
		if e.Error() == "" {
			s.ResponseDesc = "Data tidak ditemukan"
		} else {
			s.ResponseDesc = e.Error()
		}
	}
}

func WithErrAccess(e error) StdOpt {
	return func(s *Std) {
		s.ResponseCode = stderr.ERROR_CODE_ACCESS_PERMISSION
		if e.Error() == "" {
			s.ResponseDesc = "Akses tidak diijinkan"
		} else {
			s.ResponseDesc = e.Error()
		}
	}
}

func WithErrHeader(e error) StdOpt {
	return func(s *Std) {
		s.ResponseCode = stderr.ERROR_CODE_INVALID_HEADER
		if e.Error() == "" {
			s.ResponseDesc = "Header Request tidak valid"
		} else {
			s.ResponseDesc = e.Error()
		}
	}
}

func WithErrInvalidRule(e error) StdOpt {
	return func(s *Std) {
		s.ResponseCode = stderr.ERROR_CODE_INVALID_RULE
		if e.Error() == "" {
			s.ResponseDesc = "Validasi tidak terpenuhi"
		} else {
			s.ResponseDesc = e.Error()
		}
	}
}

func WithErrParameter(e error) StdOpt {
	return func(s *Std) {
		s.ResponseCode = stderr.ERROR_CODE_PARAMETER
		if e.Error() == "" {
			s.ResponseDesc = "Parameter tidak sesuai"
		} else {
			s.ResponseDesc = e.Error()
		}
	}
}

func WithErrRuntime(e error) StdOpt {
	return func(s *Std) {
		s.ResponseCode = stderr.ERROR_CODE_SYSTEM
		if e.Error() == "" {
			s.ResponseDesc = "Runtime error happens"
		} else {
			s.ResponseDesc = e.Error()
		}
	}
}

func WithErrUnsupport(e error) StdOpt {
	return func(s *Std) {
		s.ResponseCode = stderr.ERROR_CODE_UNSUPPORTED
		if e.Error() == "" {
			s.ResponseDesc = "Tidak disupport"
		} else {
			s.ResponseDesc = e.Error()
		}
	}
}

func WithErrWaiting(e error) StdOpt {
	return func(s *Std) {
		s.ResponseCode = stderr.ERROR_CODE_WAITING_STATUS
		if e.Error() == "" {
			s.ResponseDesc = "Masih proses harap tunggu"
		} else {
			s.ResponseDesc = e.Error()
		}
	}
}

func maskData(keysToMask []string, data any) any {
	if data == nil || len(keysToMask) == 0 {
		return data
	}

	// Convert to JSON and back to get map structure
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return data
	}

	var result any
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return data
	}

	// Compile patterns from keys
	patterns := make([]*regexp.Regexp, 0, len(keysToMask))
	for _, key := range keysToMask {
		// Escape special regex chars and make case-insensitive
		escaped := regexp.QuoteMeta(key)
		pattern := regexp.MustCompile(`(?i)` + escaped)
		patterns = append(patterns, pattern)
	}

	// Mask fields
	if m, ok := result.(map[string]any); ok {
		maskFieldsRecursive(m, patterns)
		return m
	}

	return result
}

func maskFieldsRecursive(data map[string]any, patterns []*regexp.Regexp) {
	for key, value := range data {
		// Check if key matches any pattern
		shouldMask := false
		for _, pattern := range patterns {
			if pattern.MatchString(key) {
				shouldMask = true
				break
			}
		}

		if shouldMask {
			// Encrypt string values
			if str, ok := value.(string); ok && str != "" {
				encrypted, err := security.Encrypt(str)
				if err != nil {
					// Fallback to masking if encryption fails
					data[key] = "***"
				} else {
					data[key] = encrypted
				}
			}
		} else if nestedMap, ok := value.(map[string]any); ok {
			// Recursively mask nested maps
			maskFieldsRecursive(nestedMap, patterns)
		} else if arr, ok := value.([]any); ok {
			// Handle arrays
			for _, item := range arr {
				if nestedMap, ok := item.(map[string]any); ok {
					maskFieldsRecursive(nestedMap, patterns)
				}
			}
		}
	}
}
