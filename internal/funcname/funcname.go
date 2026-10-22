// Package funcname menurunkan nama span/metric "<Struct>.<Func>" dari fungsi
// pemanggil saat runtime, supaya nama tidak perlu ditulis hardcode.
package funcname

import (
	"regexp"
	"runtime"
	"strings"
	"sync"
	"unicode"
)

var (
	cache sync.Map // uintptr (pc call site) → string

	overrideMu sync.RWMutex
	overrides  = map[string]string{}

	reClosure = regexp.MustCompile(`(\.func\d+|\.gowrap\d+)(\.\d+)*$`)
	reGeneric = regexp.MustCompile(`\[[^\]]*\]`)
)

// RegisterTypeName mengatur nama tampil untuk tipe receiver, mis. untuk akronim
// yang tidak bisa ditebak: RegisterTypeName("glDebitUsecase", "GLDebitUsecase").
// Panggil saat init/boot, sebelum span pertama dibuat.
func RegisterTypeName(goName, display string) {
	overrideMu.Lock()
	overrides[goName] = display
	overrideMu.Unlock()
	// nama yang sudah ter-cache mungkin memakai nama lama
	cache.Range(func(k, _ any) bool { cache.Delete(k); return true })
}

// Caller mengembalikan nama fungsi pada kedalaman stack skip:
// skip=0 → fungsi yang memanggil Caller, skip=1 → pemanggilnya, dst.
// Hasil di-cache per call site, sehingga parsing hanya terjadi sekali per lokasi kode.
func Caller(skip int) string {
	var pcs [1]uintptr
	// +2: lewati runtime.Callers dan Caller sendiri
	if runtime.Callers(skip+2, pcs[:]) == 0 {
		return "unknown"
	}
	pc := pcs[0]
	if v, ok := cache.Load(pc); ok {
		return v.(string)
	}
	// CallersFrames menangani fungsi yang di-inline compiler dengan benar.
	frame, _ := runtime.CallersFrames(pcs[:]).Next()
	name := Format(frame.Function)
	cache.Store(pc, name)
	return name
}

// Format mengubah nama fungsi Go lengkap menjadi "<Struct>.<Func>" atau
// "<pkg>.<Func>" untuk fungsi tanpa receiver.
//
//	mks/app/usecases.(*orderNotarisUsecase).CreateOrderNotaris → OrderNotarisUsecase.CreateOrderNotaris
//	mks/app/usecases.(*orderNotarisUsecase).CreateOrderNotaris.func1 → OrderNotarisUsecase.CreateOrderNotaris
//	mks/app/repo.(*Repo[...]).Get → Repo.Get
//	mks/app/jobs.(*PencairanNotarisJob).Handle → PencairanNotarisJob.Handle
//	mks/app/util.FormatDate → util.FormatDate
func Format(full string) string {
	if full == "" {
		return "unknown"
	}
	s := full
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(s, "-fm") // method value
	s = reClosure.ReplaceAllString(s, "")
	s = reGeneric.ReplaceAllString(s, "")

	// s sekarang "pkg.(*T).M", "pkg.T.M", atau "pkg.F"
	dot := strings.Index(s, ".")
	if dot < 0 {
		return s
	}
	pkg, rest := s[:dot], s[dot+1:]
	rest = strings.ReplaceAll(rest, "(*", "")
	rest = strings.ReplaceAll(rest, ")", "")

	parts := strings.SplitN(rest, ".", 2)
	if len(parts) == 1 {
		// fungsi tanpa receiver
		return pkg + "." + parts[0]
	}
	return typeName(parts[0]) + "." + parts[1]
}

func typeName(goName string) string {
	overrideMu.RLock()
	v, ok := overrides[goName]
	overrideMu.RUnlock()
	if ok {
		return v
	}
	r := []rune(goName)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}
