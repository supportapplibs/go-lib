package otellog

import (
	"os"
	"strings"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
)

var dbSystemOverride atomic.Value // string

// SetDBSystem menetapkan nilai attribute db.system untuk semua span TraceDB /
// TraceDBAuto (mis. "postgresql", "mysql"). Tanpa pemanggilan ini, nilai diturunkan
// dari env DB_CONNECTION (lihat DBSystem). String kosong mengembalikan ke perilaku env.
//
// db.system dipakai Kibana APM untuk mengisi span.subtype, sehingga span database
// tampil dengan nama database ("postgresql") di legend dan filter, bukan "db" generik.
func SetDBSystem(system string) {
	dbSystemOverride.Store(strings.TrimSpace(system))
}

// DBSystem mengembalikan nilai db.system yang dipakai span database: override dari
// SetDBSystem bila ada, jika tidak diturunkan dari env DB_CONNECTION (nama koneksi
// Goravel). Kosong bila tidak bisa ditentukan — attribute tidak dipasang.
func DBSystem() string {
	if v, _ := dbSystemOverride.Load().(string); v != "" {
		return v
	}
	return normalizeDBSystem(os.Getenv("DB_CONNECTION"))
}

// normalizeDBSystem memetakan nama driver/koneksi ke nilai db.system standar
// OpenTelemetry. Nama yang tidak dikenal dipakai apa adanya (huruf kecil).
func normalizeDBSystem(conn string) string {
	switch c := strings.ToLower(strings.TrimSpace(conn)); c {
	case "":
		return ""
	case "postgres", "postgresql", "pgsql", "pg":
		return "postgresql"
	case "mysql", "mariadb":
		return c
	case "sqlserver", "mssql":
		return "mssql"
	default:
		return c
	}
}

func dbSystemAttrs() []attribute.KeyValue {
	if s := DBSystem(); s != "" {
		return []attribute.KeyValue{attribute.String("db.system", s)}
	}
	return nil
}
