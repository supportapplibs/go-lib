package otellog

import (
	"encoding/json"
	"strings"
	"sync"
	"unicode/utf8"

	golib "github.com/supportapplibs/go-lib/log"
)

// MaskedValue adalah pengganti nilai untuk field sensitif.
const MaskedValue = "***"

// Aturan pencocokan dilakukan pada key yang sudah dinormalisasi: lowercase dan
// tanpa '_', '-', '.', spasi. Contoh: "access_token", "accessToken", "Access-Token" → "accesstoken".
//
// Default HANYA mencakup kredensial/rahasia (password, token, authorization, apikey,
// secret, signature, pin, otp, cvv, nomor kartu). Data pengguna/nasabah (nama, NIK,
// KTP, rekening, dll.) sengaja TIDAK di-mask; tambahkan key domain tertentu lewat
// RegisterSensitiveKeys bila memang perlu.
var (
	maskMu sync.RWMutex

	// sensitiveExact: key harus sama persis. Untuk kata pendek yang rawan
	// false positive bila dicocokkan sebagai substring (mis. "pin" ⊂ "pinjaman").
	sensitiveExact = map[string]struct{}{
		"pin": {}, "mpin": {}, "cvv": {}, "cvc": {}, "otp": {},
	}

	// sensitivePrefix: key diawali salah satu nilai ini (mis. "otpCode").
	sensitivePrefix = []string{"otp"}

	// sensitiveContains: key mengandung salah satu nilai ini.
	sensitiveContains = []string{
		"password", "passwd", "secret", "token", "apikey", "authorization",
		"signature", "credential", "privatekey", "cardnumber", "pinblock",
	}
)

func normalizeKey(k string) string {
	k = strings.ToLower(k)
	return strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(k)
}

// RegisterSensitiveKeys menambah key yang harus di-mask (pencocokan exact,
// setelah normalisasi). Panggil sekali saat boot service, mis. untuk field
// domain khusus: otellog.RegisterSensitiveKeys("namaIbuKandung", "noHp").
func RegisterSensitiveKeys(keys ...string) {
	maskMu.Lock()
	defer maskMu.Unlock()
	for _, k := range keys {
		sensitiveExact[normalizeKey(k)] = struct{}{}
	}
}

// IsSensitiveKey melaporkan apakah nilai dengan key ini harus di-mask.
func IsSensitiveKey(key string) bool {
	k := normalizeKey(key)
	if k == "" {
		return false
	}
	maskMu.RLock()
	_, ok := sensitiveExact[k]
	maskMu.RUnlock()
	if ok {
		return true
	}
	for _, p := range sensitivePrefix {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	for _, c := range sensitiveContains {
		if strings.Contains(k, c) {
			return true
		}
	}
	return false
}

// MaskValue mengembalikan salinan v dengan semua nilai ber-key sensitif diganti
// MaskedValue, rekursif ke map dan slice. Nilai asli tidak dimodifikasi.
func MaskValue(v any) any {
	switch t := v.(type) {
	case golib.Map:
		return golib.Map(MaskValue(map[string]any(t)).(map[string]any))
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if IsSensitiveKey(k) {
				out[k] = MaskedValue
			} else {
				out[k] = MaskValue(val)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = MaskValue(val)
		}
		return out
	default:
		return v
	}
}

// maxMaskParseBytes: body lebih besar dari ini tidak di-parse (hemat CPU) dan
// tidak dikirim sama sekali — lebih aman daripada mengirim body besar tanpa masking.
const maxMaskParseBytes = 1 << 20 // 1 MiB

// MaskJSON mem-parse body JSON, me-mask field sensitif, lalu memotong hasilnya ke
// maxBytes (0 = tanpa batas). Body non-JSON dikembalikan apa adanya (terpotong),
// karena tidak bisa diidentifikasi field-nya.
func MaskJSON(body []byte, maxBytes int) string {
	if len(body) == 0 {
		return ""
	}
	if len(body) > maxMaskParseBytes {
		return "[omitted: body too large]"
	}
	var parsed any
	if err := json.Unmarshal(body, &parsed); err == nil {
		if b, err := json.Marshal(MaskValue(parsed)); err == nil {
			body = b
		}
	}
	return truncate(string(body), maxBytes)
}

func truncate(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	cut := s[:maxBytes]
	// jangan potong di tengah karakter multi-byte
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "...[truncated]"
}
