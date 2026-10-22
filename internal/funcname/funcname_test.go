package funcname

import (
	"sync"
	"testing"
)

func TestFormat(t *testing.T) {
	cases := map[string]string{
		"mks/app/usecases.(*orderNotarisUsecase).CreateOrderNotaris":         "OrderNotarisUsecase.CreateOrderNotaris",
		"mks/app/usecases.(*orderNotarisUsecase).CreateOrderNotaris.func1":   "OrderNotarisUsecase.CreateOrderNotaris",
		"mks/app/usecases.(*orderNotarisUsecase).CreateOrderNotaris.func1.2": "OrderNotarisUsecase.CreateOrderNotaris",
		"mks/app/usecases.(*orderNotarisUsecase).CreateOrderNotaris.gowrap1": "OrderNotarisUsecase.CreateOrderNotaris",
		"mks/app/usecases.(*orderNotarisUsecase).CreateOrderNotaris-fm":      "OrderNotarisUsecase.CreateOrderNotaris",
		"mks/app/usecases/transfer.(*glDebitUsecase).executeTransfer":        "GlDebitUsecase.executeTransfer",
		"mks/app/jobs.(*PencairanNotarisJob).Handle":                         "PencairanNotarisJob.Handle",
		"mks/app/repositories.orderBiayaRepository.GetById":                  "OrderBiayaRepository.GetById",
		"mks/app/repositories.(*Repo[...]).Get":                              "Repo.Get",
		"mks/app/util.FormatDate":                                            "util.FormatDate",
		"main.main":                                                          "main.main",
		"github.com/x/y/v2/pkg.(*svc).Do":                                    "Svc.Do",
		"":                                                                   "unknown",
	}
	for in, want := range cases {
		if got := Format(in); got != want {
			t.Errorf("Format(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRegisterTypeName(t *testing.T) {
	RegisterTypeName("glDebitUsecase", "GLDebitUsecase")
	defer RegisterTypeName("glDebitUsecase", "GlDebitUsecase")
	if got := Format("mks/app/usecases/transfer.(*glDebitUsecase).executeTransfer"); got != "GLDebitUsecase.executeTransfer" {
		t.Fatalf("got %q", got)
	}
}

type sampleUsecase struct{}

func (s *sampleUsecase) Direct() string { return Caller(0) }

//go:noinline
func (s *sampleUsecase) NoInline() string { return Caller(0) }

func (s sampleUsecase) ValueRecv() string { return Caller(0) }

func (s *sampleUsecase) InClosure() string {
	var out string
	func() { out = Caller(1) }() // skip=1: lewati closure → method
	return out
}

func (s *sampleUsecase) viaHelper() string { return helper() }

// helper meniru wrapper seperti startSpan di controller: skip=1 → pemanggil helper.
func helper() string { return Caller(1) }

func TestCaller(t *testing.T) {
	s := &sampleUsecase{}
	checks := map[string]string{
		"Direct":    s.Direct(),
		"NoInline":  s.NoInline(),
		"ValueRecv": sampleUsecase{}.ValueRecv(),
		"InClosure": s.InClosure(),
		"viaHelper": s.viaHelper(),
	}
	for fn, got := range checks {
		if want := "SampleUsecase." + fn; got != want {
			t.Errorf("%s: got %q, want %q", fn, got, want)
		}
	}
	// call kedua memakai cache dan harus identik
	if a, b := s.Direct(), s.Direct(); a != b {
		t.Fatalf("cached mismatch %q vs %q", a, b)
	}
}

func TestCaller_Concurrent(t *testing.T) {
	s := &sampleUsecase{}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := s.NoInline(); got != "SampleUsecase.NoInline" {
				t.Errorf("got %q", got)
			}
		}()
	}
	wg.Wait()
}

func BenchmarkCaller_Cached(b *testing.B) {
	s := &sampleUsecase{}
	for i := 0; i < b.N; i++ {
		_ = s.NoInline()
	}
}
