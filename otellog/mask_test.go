package otellog

import (
	"testing"

	golib "github.com/supportapplibs/go-lib/log"
)

func TestIsSensitiveKey(t *testing.T) {
	sensitive := []string{
		"password", "Password", "newPassword", "user_password", "pin", "PIN", "otp", "otpCode",
		"nik", "NIK", "nikDebitur", "nik_debitur", "norekNotaris", "norek_notaris",
		"Authorization", "X-Api-Key", "x-signature", "accessToken", "refresh_token", "clientSecret",
		"noRekening", "nomor_rekening", "cvv",
	}
	for _, k := range sensitive {
		if !IsSensitiveKey(k) {
			t.Errorf("expected %q to be sensitive", k)
		}
	}
	safe := []string{
		"pinjaman", "jenisPinjaman", "teknik", "unik", "orderId", "trace.id", "span.id",
		"requestID", "deviceID", "identifier", "msg", "message", "responseCode", "nama",
	}
	for _, k := range safe {
		if IsSensitiveKey(k) {
			t.Errorf("expected %q to be safe", k)
		}
	}
}

func TestRegisterSensitiveKeys(t *testing.T) {
	if IsSensitiveKey("namaIbuKandung") {
		t.Fatal("precondition: should not be sensitive yet")
	}
	RegisterSensitiveKeys("nama_ibu_kandung")
	if !IsSensitiveKey("namaIbuKandung") {
		t.Fatal("registered key should be sensitive after normalization")
	}
}

func TestMaskValue_NestedAndImmutable(t *testing.T) {
	in := map[string]any{
		"username": "budi",
		"password": "rahasia",
		"data": map[string]any{
			"nikDebitur": "3201000000000001",
			"items":      []any{map[string]any{"norekNotaris": "0101", "nominal": 1000}},
		},
		"log": golib.Map{"token": "abc", "ok": true},
	}
	out := MaskValue(in).(map[string]any)

	if out["username"] != "budi" || out["password"] != MaskedValue {
		t.Fatalf("top-level mask wrong: %+v", out)
	}
	data := out["data"].(map[string]any)
	if data["nikDebitur"] != MaskedValue {
		t.Fatalf("nested map not masked: %+v", data)
	}
	item := data["items"].([]any)[0].(map[string]any)
	if item["norekNotaris"] != MaskedValue || item["nominal"] != 1000 {
		t.Fatalf("slice item mask wrong: %+v", item)
	}
	if lg := out["log"].(golib.Map); lg["token"] != MaskedValue || lg["ok"] != true {
		t.Fatalf("golib.Map not masked: %+v", lg)
	}
	if in["password"] != "rahasia" || in["data"].(map[string]any)["nikDebitur"] != "3201000000000001" {
		t.Fatal("input was mutated")
	}
}

func TestMaskJSON(t *testing.T) {
	got := MaskJSON([]byte(`{"responseCode":"00","data":{"nik":"123","nama":"Budi"}}`), 0)
	want := `{"data":{"nama":"Budi","nik":"***"},"responseCode":"00"}`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	if got := MaskJSON([]byte("plain text body"), 5); got != "plain...[truncated]" {
		t.Fatalf("non-JSON truncate: %q", got)
	}
	if got := MaskJSON(nil, 10); got != "" {
		t.Fatalf("empty body: %q", got)
	}
}

func TestTruncate_UTF8Safe(t *testing.T) {
	// "é" = 2 byte; potong di byte ke-2 tidak boleh menghasilkan UTF-8 rusak
	if got := truncate("aé", 2); got != "a...[truncated]" {
		t.Fatalf("got %q", got)
	}
}

func TestEnrich_MasksSensitiveFields(t *testing.T) {
	l := &Logger{}
	out := l.enrich(golib.Map{"msg": "login", "password": "x", "payload": map[string]any{"pin": "1234"}})
	if out["password"] != MaskedValue {
		t.Fatalf("password not masked: %+v", out)
	}
	if out["payload"].(map[string]any)["pin"] != MaskedValue {
		t.Fatalf("nested pin not masked: %+v", out)
	}
}
