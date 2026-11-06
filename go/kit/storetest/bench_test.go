package storetest

import (
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/kit/store"
)

func BenchmarkStoreOpen(b *testing.B) {
	for i := 0; i < b.N; i++ {
		d, err := store.Open(filepath.Join(b.TempDir(), "db.sqlite"))
		if err != nil {
			b.Fatal(err)
		}
		d.Close()
	}
}

func BenchmarkTemplateCopy(b *testing.B) {
	for i := 0; i < b.N; i++ {
		d, err := OpenAt(b, filepath.Join(b.TempDir(), "db.sqlite"))
		if err != nil {
			b.Fatal(err)
		}
		d.Close()
	}
}
