package store

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// TestParallelWrites 是回归测试（第 21 轮）：16 goroutine 并行写同一库，
// 修复前 1561/1600 次报 database is locked (SQLITE_BUSY)。
func TestParallelWrites(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var wg sync.WaitGroup
	errs := make(chan error, 16*100)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				h := VodHistory{Site: fmt.Sprintf("s%d", g), VodID: fmt.Sprintf("v%d", i), VodTitle: "t", Progress: i, Duration: 100}
				if err := st.UpsertVodHistory(h); err != nil {
					errs <- err
				}
				if err := st.RecordVodSearch(fmt.Sprintf("q%d-%d", g, i)); err != nil {
					errs <- err
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	n := 0
	for err := range errs {
		if n < 3 {
			t.Logf("write error: %v", err)
		}
		n++
	}
	if n > 0 {
		t.Fatalf("并行写入共 %d 次失败", n)
	}
}
