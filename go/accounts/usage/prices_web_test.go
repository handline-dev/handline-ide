package usage

import (
	"os"
	"reflect"
	"regexp"
	"strconv"
	"testing"
)

// The web app estimates with its own copy of the list prices (packages/app-core/src/prices.ts); it must be InputPrices and
// DefaultPattern, so a price change here cannot leave the app's estimate behind.
func TestWebPricesMatchInputPrices(t *testing.T) {
	src, err := os.ReadFile("../../../packages/app-core/src/prices.ts")
	if err != nil {
		t.Fatal(err)
	}
	var got []priceEntry
	for _, m := range regexp.MustCompile(`\['([^']+)', ([0-9.]+)\]`).FindAllStringSubmatch(string(src), -1) {
		usd, _ := strconv.ParseFloat(m[2], 64)
		got = append(got, priceEntry{m[1], usd})
	}
	if !reflect.DeepEqual(got, InputPrices) {
		t.Errorf("prices.ts INPUT_USD = %v, want InputPrices %v", got, InputPrices)
	}
	def := regexp.MustCompile(`DEFAULT_PATTERN = '([^']+)'`).FindStringSubmatch(string(src))
	if def == nil || def[1] != DefaultPattern {
		t.Errorf("prices.ts DEFAULT_PATTERN = %v, want %q", def, DefaultPattern)
	}
}
