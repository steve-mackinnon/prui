package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkBorrowObjects(b *testing.B) {
	for _, count := range []int{2000, 20000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			objects := b.TempDir()
			for i := 0; i < count; i++ {
				path := filepath.Join(objects, fmt.Sprintf("%02x/%038x", i%256, i))
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					b.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("synthetic immutable object payload"), 0600); err != nil {
					b.Fatal(err)
				}
			}
			for _, workers := range []int{1, 2, 4} {
				b.Run(fmt.Sprintf("workers-%d", workers), func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						borrowed := b.TempDir()
						err := borrowObjects(context.Background(), objects, borrowed, workers)
						if err != nil {
							b.Fatal(err)
						}
						if err := os.RemoveAll(borrowed); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
