package otellog

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	golib "github.com/supportapplibs/go-lib/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// AddFields menambahkan field bisnis (mis. refno, order_id, pn_maker) ke context:
//
//   - disimpan sebagai request fields, sehingga otomatis ikut ke setiap log
//     otellog.Runtime / RuntimeCtx / Activity yang memakai context ini
//   - dipasang sebagai attribute di span aktif pada ctx, supaya bisa dicari di
//     Jaeger/APM
//
// Nilai di-mask lewat MaskValue (key sensitif jadi "***"). Nilai nil atau string
// kosong dilewati. Field yang sudah ada di context ditimpa bila key-nya sama.
// Pakai context hasil kembalian untuk pemanggilan berikutnya:
//
//	ctx = otellog.AddFields(ctx, log.Map{"refno": req.Refno, "order_id": id})
func AddFields(ctx context.Context, fields ...golib.Map) context.Context {
	merged := golib.Map{}
	for _, f := range fields {
		if len(f) == 0 {
			continue
		}
		masked, _ := MaskValue(f).(golib.Map)
		for k, v := range masked {
			if s, ok := fieldString(v); ok {
				merged[k] = s
			}
		}
	}
	if len(merged) == 0 {
		return ctx
	}
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		span.SetAttributes(fieldAttrs(merged)...)
	}
	return WithRequestFields(ctx, merged)
}

// fieldAttrs mengubah field menjadi attribute span (urut berdasarkan key supaya
// deterministik).
func fieldAttrs(m golib.Map) []attribute.KeyValue {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]attribute.KeyValue, 0, len(keys))
	for _, k := range keys {
		if s, ok := fieldString(m[k]); ok {
			out = append(out, attribute.String(k, s))
		}
	}
	return out
}

// fieldString mengubah nilai skalar (termasuk pointer ke skalar) menjadi string.
// ok=false untuk nil, string kosong, map, slice, atau struct — nilai kompleks
// tidak cocok sebagai field pencarian.
func fieldString(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return "", false
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.String:
		s := strings.TrimSpace(rv.String())
		return s, s != ""
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return fmt.Sprint(rv.Interface()), true
	default:
		if s, ok := rv.Interface().(fmt.Stringer); ok {
			out := s.String()
			return out, out != ""
		}
		return "", false
	}
}

// bodyField memetakan key di request body (src) ke nama field di log/span (dest).
type bodyField struct{ src, dest string }

// parseBodyFields memecah spesifikasi "src" atau "src:dest".
func parseBodyFields(specs []string) []bodyField {
	out := make([]bodyField, 0, len(specs))
	for _, s := range specs {
		src, dest, found := strings.Cut(s, ":")
		src, dest = strings.TrimSpace(src), strings.TrimSpace(dest)
		if src == "" {
			continue
		}
		if !found || dest == "" {
			dest = src
		}
		out = append(out, bodyField{src: src, dest: dest})
	}
	return out
}

// extractBodyFields mengambil field top-level dari request body. Pencocokan key
// tidak peka huruf besar/kecil dan mengabaikan '_', '-', '.' ("refnoOrder" cocok
// dengan "refno_order"). Key sensitif tidak pernah diambil.
func extractBodyFields(body map[string]any, fields []bodyField) golib.Map {
	if len(body) == 0 || len(fields) == 0 {
		return nil
	}
	norm := make(map[string]any, len(body))
	for k, v := range body {
		norm[normalizeKey(k)] = v
	}
	out := golib.Map{}
	for _, f := range fields {
		if IsSensitiveKey(f.src) || IsSensitiveKey(f.dest) {
			continue
		}
		if _, dup := out[f.dest]; dup {
			continue
		}
		if s, ok := fieldString(norm[normalizeKey(f.src)]); ok {
			out[f.dest] = s
		}
	}
	return out
}
