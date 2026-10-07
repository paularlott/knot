package filestore

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"math/rand"
	"sync"
	"time"
)

const (
	fetchWorkers = 16
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

// busy reports whether the queue is more than half full, so there is no need
// to feed it.
func (f *fetcher) busy() bool { return len(f.queue) > cap(f.queue)/2 }

// enqueueWait schedules a background fetch, waiting for room in the queue
// rather than dropping it; the caller is a background task that can wait.
func (f *fetcher) enqueueWait(sha string, size int64, source string) {
	if f.s.replicator() == nil {
		return
	}
	f.mu.Lock()
	if _, busy := f.inflight[sha]; busy {
		f.mu.Unlock()
		return
	}
	j := &fetchJob{sha: sha, size: size, source: source, done: make(chan struct{})}
	f.inflight[sha] = j
	f.mu.Unlock()

	select {
	case f.queue <- j:
	case <-f.ctx.Done():
		f.finish(j, f.ctx.Err())
	}
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
			if j.size > inlineMax {
				f.run(j)
				continue
			}

			// Small content is fetched many files to a connection: a
			// connection for each would be far slower, and a server catching
			// up on a large backlog would run out of ports.
			batch := []*fetchJob{j}
			var large []*fetchJob
		collect:
			for len(batch) < fetchBatch {
				select {
				case k := <-f.queue:
					if k.size <= inlineMax {
						batch = append(batch, k)
					} else {
						large = append(large, k)
					}
				default:
					break collect
				}
			}
			f.runBatch(batch)
			for _, k := range large {
				f.run(k)
			}
		}
	}
}

// runBatch fetches small content, a server at a time. What a server did not
// send is fetched one at a time, which also tries the other servers.
func (f *fetcher) runBatch(jobs []*fetchJob) {
	repl := f.s.replicator()
	var fallback []*fetchJob
	bySource := make(map[string][]*fetchJob)
	for _, j := range jobs {
		switch {
		case f.s.holds(j.sha):
			f.finish(j, nil)
		case repl == nil:
			f.finish(j, ErrUnavailable)
		default:
			bySource[j.source] = append(bySource[j.source], j)
		}
	}

	nodes := map[string]bool{}
	if repl != nil {
		for _, n := range repl.FileNodes() {
			nodes[n] = true
		}
	}
	for source, group := range bySource {
		node := source
		if !nodes[node] {
			// The writer is not reachable, or is this server: ask any other.
			node = ""
			for n := range nodes {
				node = n
				break
			}
		}
		if node == "" {
			fallback = append(fallback, group...)
			continue
		}
		fallback = append(fallback, f.fetchBatch(repl, node, group)...)
	}
	for _, j := range fallback {
		f.run(j)
	}
}

// fetchBatch fetches the content of jobs from one server, finishing those it
// stores and returning the rest.
func (f *fetcher) fetchBatch(repl Replicator, node string, jobs []*fetchJob) []*fetchJob {
	shas := make([]string, len(jobs))
	for i, j := range jobs {
		shas[i] = j.sha
	}
	r, err := repl.OpenContentBatch(f.ctx, node, shas)
	if err != nil {
		return jobs
	}
	defer r.Close()

	var rest []*fetchJob
	for i, j := range jobs {
		data, err := readBatchFrame(r, j.sha)
		if err != nil {
			return append(rest, jobs[i:]...) // the rest is fetched another way
		}
		if data == nil { // not held there
			rest = append(rest, j)
			continue
		}
		f.finish(j, f.s.storeContent(j.sha, int64(len(data)), data, "", false))
	}
	return rest
}

// fetchBatch is how many small blobs are fetched over one connection.
const fetchBatch = 64

// readBatchFrame reads one blob of a batch, nil if the sender did not have
// it, checked against the checksum it was asked for.
func readBatchFrame(r io.Reader, sha string) ([]byte, error) {
	var head [72]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return nil, err
	}
	if string(head[:64]) != sha {
		return nil, errors.New("content out of order")
	}
	size := int64(binary.BigEndian.Uint64(head[64:]))
	if size < 0 {
		return nil, nil
	}
	if size > inlineMax {
		return nil, errors.New("content larger than a batch carries")
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != sha {
		return nil, ErrContentMismatch
	}
	return data, nil
}

// WriteContentBatch writes the content of each of shas that this server
// holds, in order, each as its checksum, its size as 8 bytes (negative when
// not held) and its bytes. It is how a server catching up fetches small files.
func (s *Store) WriteContentBatch(w io.Writer, shas []string) error {
	for _, sha := range shas {
		if !validSHA(sha) {
			return ErrContentMismatch
		}
		var head [72]byte
		copy(head[:64], sha)
		c, err := s.openContent(sha)
		var data []byte
		if err == nil {
			data, err = io.ReadAll(io.LimitReader(c, inlineMax+1))
			c.Close()
		}
		if err != nil || len(data) > inlineMax {
			binary.BigEndian.PutUint64(head[64:], ^uint64(0)) // -1
			if _, werr := w.Write(head[:]); werr != nil {
				return werr
			}
			continue
		}
		binary.BigEndian.PutUint64(head[64:], uint64(len(data)))
		if _, err := w.Write(append(head[:], data...)); err != nil {
			return err
		}
	}
	return nil
}

func (f *fetcher) finish(j *fetchJob, err error) {
	f.mu.Lock()
	delete(f.inflight, j.sha)
	f.mu.Unlock()
	j.err = err
	close(j.done)
}

func (f *fetcher) run(j *fetchJob) {
	if f.s.holds(j.sha) {
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

	// Small content is fetched into memory, as it is stored in the database.
	var sink fetchSink
	if j.size <= inlineMax {
		sink = &memSink{sha: sha256.New()}
	} else {
		tw, err := f.s.blobs.newTemp()
		if err != nil {
			f.finish(j, err)
			return
		}
		sink = tw
	}
	err := ErrUnavailable
	for _, node := range nodes {
		if err = f.fetchFrom(repl, node, j, sink); err == nil || f.ctx.Err() != nil {
			break
		}
		f.s.logger.Debug("fetch from peer failed", "sha", j.sha, "node", node, "error", err)
	}
	if err == nil {
		err = f.install(j, sink)
	} else {
		sink.discard()
	}
	if err != nil && f.ctx.Err() == nil {
		f.s.logger.Warn("unable to fetch object content", "sha", j.sha, "error", err)
	}
	f.finish(j, err)
}

// fetchFrom streams the rest of a blob from node into tw: content is
// identical on every server, so an attempt resumes from what any earlier
// attempt, on this server or another, already wrote.
func (f *fetcher) fetchFrom(repl Replicator, node string, j *fetchJob, tw fetchSink) error {
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
		if r, err = repl.OpenContent(f.ctx, node, j.sha, tw.Size()); err != nil {
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

// fetchSink receives fetched content: in memory when it is small, else a
// temporary file.
type fetchSink interface {
	io.Writer
	Size() int64
	SHA256() string
	discard()
}

// memSink holds small content in memory while it is fetched.
type memSink struct {
	buf []byte
	sha hash.Hash
}

func (m *memSink) Write(p []byte) (int, error) {
	if len(m.buf)+len(p) > inlineMax {
		return 0, errors.New("content is larger than its record says")
	}
	m.buf = append(m.buf, p...)
	m.sha.Write(p)
	return len(p), nil
}

func (m *memSink) Size() int64    { return int64(len(m.buf)) }
func (m *memSink) SHA256() string { return hex.EncodeToString(m.sha.Sum(nil)) }
func (m *memSink) discard()       {}

// install verifies and stores fetched content.
func (f *fetcher) install(j *fetchJob, sink fetchSink) error {
	if sink.SHA256() != j.sha {
		sink.discard()
		return ErrContentMismatch
	}
	if m, ok := sink.(*memSink); ok {
		return f.s.storeContent(j.sha, m.Size(), m.buf, "", false)
	}
	tw := sink.(*tempWriter)
	if err := tw.finish(); err != nil {
		tw.discard()
		return err
	}
	// Small content goes into the database, larger content into a file.
	return f.s.storeContent(j.sha, tw.size, nil, tw.Path(), false)
}

// WriteContent streams a blob to another server, from offset to its end.
func (s *Store) WriteContent(w io.Writer, sha string, offset int64) error {
	f, err := s.openContent(sha)
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
