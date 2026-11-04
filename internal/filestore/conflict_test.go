package filestore

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// Many writers at once of files with the same content, which all change its
// reference count, all succeed: empty files and folder markers, as a sync of
// a large tree writes them several at a time, and identical files.
func TestConcurrentWritesOfSameContent(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "same")
	const writers, each = 64, 20
	for _, content := range []string{"", "the same licence text"} {
		var wg sync.WaitGroup
		errs := make(chan error, writers*each)
		for w := 0; w < writers; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				for i := 0; i < each; i++ {
					key := fmt.Sprintf("c%d/w%02d/f%02d/", len(content), w, i)
					if content != "" {
						key = strings.TrimSuffix(key, "/") + ".txt"
					}
					if _, err := s.PutObject(alice, "same", key, strings.NewReader(content), PutOptions{Size: int64(len(content))}); err != nil {
						errs <- fmt.Errorf("%s: %w", key, err)
					}
				}
			}(w)
		}
		wg.Wait()
		close(errs)
		failed := 0
		for err := range errs {
			if failed++; failed <= 3 {
				t.Error(err)
			}
		}
		if failed > 0 {
			t.Fatalf("%d of %d writes of %q failed", failed, writers*each, content)
		}
	}
	if u := s.Usage("alice"); u.Objects != 2*writers*each {
		t.Errorf("%d objects, want %d", u.Objects, 2*writers*each)
	}
}
