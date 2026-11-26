package filestore

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Writers storing the same content at the same moment all succeed, and the
// blob ends up there once.
func TestBlobInstallConcurrentIdentical(t *testing.T) {
	dir := t.TempDir()
	bs, err := newBlobStore(filepath.Join(dir, "blobs"), filepath.Join(dir, "tmp"), true)
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dir, "tmp"), 0700)
	for round := 0; round < 200; round++ {
		content := []byte(fmt.Sprintf("round %d %s", round, bytes.Repeat([]byte("x"), 1000)))
		const writers = 16
		tws := make([]*tempWriter, writers)
		for i := range tws {
			tw, err := bs.writeTemp(bytes.NewReader(content), -1, -1, "", nil)
			if err != nil {
				t.Fatal(err)
			}
			tws[i] = tw
		}
		start := make(chan struct{})
		errs := make(chan error, writers)
		var wg sync.WaitGroup
		for _, tw := range tws {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errs <- bs.install(tw.Path(), tw.SHA256())
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("round %d: %v", round, err)
			}
		}
		if got, err := os.ReadFile(bs.path(tws[0].SHA256())); err != nil || !bytes.Equal(got, content) {
			t.Fatalf("round %d: blob %d bytes, err %v", round, len(got), err)
		}
	}
	if left, _ := os.ReadDir(filepath.Join(dir, "tmp")); len(left) != 0 {
		t.Errorf("%d temporary files left", len(left))
	}
}

// Installs succeed while the sweep removes empty fan-out folders, as it
// does after deletes.
func TestBlobInstallDuringSweep(t *testing.T) {
	dir := t.TempDir()
	bs, err := newBlobStore(filepath.Join(dir, "blobs"), filepath.Join(dir, "tmp"), true)
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dir, "tmp"), 0700)
	stop := make(chan struct{})
	var sweeps sync.WaitGroup
	for range 4 {
		sweeps.Add(1)
		go func() {
			defer sweeps.Done()
			for {
				select {
				case <-stop:
					return
				default:
					bs.removeEmptyDirs()
				}
			}
		}()
	}
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for w := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 400 {
				tw, err := bs.writeTemp(bytes.NewReader([]byte(fmt.Sprintf("w%d i%d", w, i))), -1, -1, "", nil)
				if err != nil {
					errs <- err
					return
				}
				sha := tw.SHA256()
				if err := bs.install(tw.Path(), sha); err != nil {
					errs <- err
					return
				}
				bs.remove(sha) // leaves its folders empty for the sweep
			}
		}()
	}
	wg.Wait()
	close(stop)
	sweeps.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
