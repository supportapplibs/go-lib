package currency_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/supportapplibs/go-lib/currency"
)

func TestCurrency(t *testing.T) {
	testCases := []struct {
		name   string
		sample int
		expect string
	}{
		{
			name:   "Given number 101 should return seratus satu",
			sample: 101,
			expect: "seratus satu",
		},
		{
			name:   "Given number 5921 should return lima ribu sembilan ratus dua puluh satu",
			sample: 5921,
			expect: "lima ribu sembilan ratus dua puluh satu",
		},
		{
			name:   "Given number 7289123 should return tujuh juta dua ratus delapan puluh sembilan ribu seratus dua puluh tiga",
			sample: 7289123,
			expect: "tujuh juta dua ratus delapan puluh sembilan ribu seratus dua puluh tiga",
		},
		{
			name:   "Given number 774289833 should return tujuh ratus tujuh puluh empat juta dua ratus delapan puluh sembilan ribu delapan ratus tiga puluh tiga",
			sample: 774289833,
			expect: "tujuh ratus tujuh puluh empat juta dua ratus delapan puluh sembilan ribu delapan ratus tiga puluh tiga",
		},
		{
			name:   "Given number 1000000000 should return satu miliar",
			sample: 1000000000,
			expect: "satu miliar",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expect, currency.IntToWordsIndonesian(tc.sample))
		})
	}
}
