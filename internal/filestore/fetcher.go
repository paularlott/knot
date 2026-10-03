package filestore

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"os"
	"sync"
	"time"
)

const (
	fetchWorkers = 4
	fetchQueue   = 4096

	// fetchAttempts is how many times in a row a fetch may fail against a
	// server without making progress; each attempt resumes where the last
	// one broke off.
	fetchAttempts = 3

	// fetchMaxResumes bounds how often one fetch may resume, so a peer that
	// keeps breaking off after a few bytes cannot hold a worker for ever.
	fetchMaxResumes = 1000
)

type fetchJob struct {
	sha    string
	size   int64
	source string
	done   chan struct{}
	err    error
}

// fetcher pulls content this server is missing from other servers, each
// blob streamed directly from one peer.
type fetcher struct {
	s     *Store
	queue chan *fetchJob

	mu       sync.Mutex
	inflight map[string]*fetchJob

	ctx    context.Context // cancelled when the store closes
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newFetcher(s *Store) *fetcher {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fetcher{
		s:        s,
		queue:    make(chan *fetchJob, fetchQueue),
		inflight: make(map[string]*fetchJob),
		ctx:      ctx,
		cancel:   cancel,
	}
	for i := 0; i < fetchWorkers; i++ {
		f.wg.Add(1)
		go f.worker()
	}
	return f
}

func (f *fetcher) close() {
	f.cancel()
	f.wg.Wait()
}

// job returns the in-flight job for sha, starting one if needed.
func (f *fetcher) job(sha string, size int64, source string, block bool) *fetchJob {
	f.mu.Lock()
	if j := f.inflight[sha]; j != nil {
		f.mu.Unlock()
		return j
	}
	j := &fetchJob{sha: sha, size: size, source: source, done: make(chan struct{})}
	f.inflight[sha] = j
	f.mu.Unlock()

	if block {
		// A reader is waiting; don't queue behind background work.
		go f.run(j)
		return j
	}

	select {
	case f.queue <- j:
	default:
		// Queue full: drop it, the maintenance sweep queues it again.
		f.finish(j, errors.New("fetch queue full"))
	}
	return j
}

// enqueue schedules a background fetch; it never blocks.
func (f *fetcher) enqueue(sha string, size int64, source string) {
	if f.s.replicator() == nil {
		return
	}
	f.job(sha, size, source, false)
}

// wait fetches sha, returning once it is stored locally.
func (f *fetcher) wait(ctx context.Context, sha string, size int64, source string) error {
	if f.s.replicator() == nil {
		return ErrUnavailable
	}
	j := f.job(sha, size, source, true)
	select {
	case <-j.done:
		return j.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *fetcher) worker() {
	defer f.wg.Done()
	for {
		select {
		case <-f.ctx.Done():
			return
		case j := <-f.queue:
			f.run(j)
		}
	}
}

func (f *fetcher) finish(j *fetchJob, err error) {
	f.mu.Lock()
	delete(f.inflight, j.sha)
	f.mu.Unlock()
	j.err = err
	close(j.done)
}

func (f *fetcher) run(j *fetchJob) {
	if f.s.blobs.has(j.sha) {
		f.finish(j, nil)
		return
	}

	repl := f.s.replicator()
	if repl == nil {
		f.finish(j, ErrUnavailable)
		return
	}

	// Try the server that wrote the content first, then the rest in random order.
	nodes := repl.FileNodes()
	rand.Shuffle(len(nodes), func(a, b int) { nodes[a], nodes[b] = nodes[b], nodes[a] })
	if j.source != "" && j.source != f.s.nodeId {
		for i, n := range nodes {
			if n == j.source {
				nodes[0], nodes[i] = nodes[i], nodes[0]
				break
			}
		}
	}

	tw, err := f.s.blobs.newTemp()
	if err != nil {
		f.finish(j, err)
		return
	}
	err = ErrUnavailable
	for _, node := range nodes {
		if err = f.fetchFrom(repl, node, j, tw); err == nil || f.ctx.Err() != nil {
			break
		}
		f.s.logger.Debug("fetch from peer failed", "sha", j.sha, "node", node, "error", err)
	}
	if err == nil {
		err = f.install(j, tw)
	} else {
		tw.discard()
	}
	if err != nil && f.ctx.Err() == nil {
		f.s.logger.Warn("unable to fetch object content", "sha", j.sha, "error", err)
	}
	f.finish(j, err)
}

// fetchFrom streams the rest of a blob from node into tw: content is
// identical on every server, so an attempt resumes from what any earlier
// attempt, on this server or another, already wrote.
func (f *fetcher) fetchFrom(repl Replicator, node string, j *fetchJob, tw *tempWriter) error {
	var err error
	for failures, opens := 0, 0; failures < fetchAttempts; opens++ {
		if opens == fetchMaxResumes {
			return errors.New("too many interrupted transfers")
		}
		if failures > 0 {
			select {
			case <-f.ctx.Done():
				return f.ctx.Err()
			case <-time.After(time.Duration(failures) * 200 * time.Millisecond):
			}
		}
		var r io.ReadCloser
		if r, err = repl.OpenContent(f.ctx, node, j.sha, tw.size); err != nil {
			failures++
			continue
		}
		var n int64
		n, err = io.Copy(tw, r)
		r.Close()
		if err == nil {
			return nil
		}
		if n > 0 {
			failures = 0 // progress: keep resuming
		} else {
			failures++
		}
	}
	return err
}

// install verifies and stores a fetched blob.
func (f *fetcher) install(j *fetchJob, tw *tempWriter) error {
	if tw.SHA256() != j.sha {
		tw.discard()
		return ErrContentMismatch
	}
	if err := tw.finish(); err != nil {
		tw.discard()
		return err
	}

	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	if f.s.blobRefs[j.sha] == 0 {
		// Deleted while we were fetching.
		os.Remove(tw.Path())
		return nil
	}
	if err := f.s.blobs.install(tw.Path(), j.sha); err != nil {
		return err
	}
	f.s.contentHeldLocked(j.sha)
	return nil
}

// WriteContent streams a blob to another server, from offset to its end.
func (s *Store) WriteContent(w io.Writer, sha string, offset int64) error {
	f, err := s.blobs.open(sha)
	if err != nil {
		return ErrUnavailable
	}
	defer f.Close()
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return err
		}
	}
	_, err = io.Copy(w, f)
	return err
}
