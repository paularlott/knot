package filestore

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/paularlott/gossip/hlc"
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

// A large file written while content left unreferenced waits to be removed
// commits: the removal takes the content lock the write held to install its
// file, which once deadlocked the store.
func TestLargeWriteWithRemovalPending(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "big")
	s.unlinkMu.Lock()
	s.unlink = append(s.unlink, contentSHA("content nothing references"))
	s.unlinkMu.Unlock()

	big := strings.Repeat("B", inlineMax+1)
	done := make(chan error, 1)
	go func() {
		_, err := s.PutObject(alice, "big", "f", strings.NewReader(big), PutOptions{Size: int64(len(big))})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a large write with a removal pending did not finish")
	}
	if got := read(t, s, alice, "big", "f"); got != big {
		t.Error("the large file reads back wrong")
	}
}

// A large write that loses to a newer record from another server leaves no
// content file behind.
func TestLosingLargeWriteLeavesNoContent(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "race")
	id := bid(s, "race")
	s.Merge(nil, []*Object{{BucketId: id, Key: "k", Size: 3, SHA256: contentSHA("new"), ETag: "e", UpdatedAt: hlc.Now() + (1 << 40)}})

	big := strings.Repeat("L", inlineMax+1)
	if _, err := s.PutObject(alice, "race", "k", strings.NewReader(big), PutOptions{Size: int64(len(big))}); err != nil {
		t.Fatal(err)
	}
	if s.blobs.has(contentSHA(big)) {
		t.Error("the losing write's content file was kept")
	}
}
