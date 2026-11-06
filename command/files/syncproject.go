package command_files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/syncproject"
	"github.com/paularlott/knot/internal/util/excludes"
	"github.com/paularlott/logger"
)

// snapshotManifest is the file a snapshot writes last into the folder it
// makes (filestore.SnapshotManifest).
const snapshotManifest = ".knot-snapshot.json"

// Retries of a watching job that stopped, and how long a snapshot is waited
// for. Variables for tests.
var (
	jobRetryMin      = 2 * time.Second
	jobRetryMax      = time.Minute
	snapshotWaitFor  = 30 * time.Second
	snapshotWaitPoll = time.Second
)

// clientFunc gives the client a job syncs with, asked for on each attempt:
// in the agent it is not there until the agent has registered.
type clientFunc func() (*apiclient.ApiClient, error)

// projectRun is a sync project running, each job on its own.
type projectRun struct {
	project  *syncproject.Project
	template bool
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	jobs     []*jobRun
}

// jobRun is one job of a running project.
type jobRun struct {
	job     syncproject.Job
	project *syncproject.Project
	log     logger.Logger

	mu        sync.Mutex
	st        syncproject.JobStatus
	first     chan struct{}   // closed when the first attempt has passed once, or failed
	flush     chan chan error // asks the watching sync to send its local changes now
	firstErr  error
	firstDone bool
}

// checkProject checks what Validate cannot: that the bucket folders are
// bucket paths knot reads.
func checkProject(p *syncproject.Project) error {
	for _, j := range p.Sync {
		if _, err := requireRemote(j.Remote); err != nil {
			return fmt.Errorf("sync job %s: remote: %w", j.Name, err)
		}
		if j.Snapshot != "" {
			src, err := requireRemote(j.Snapshot)
			if err != nil {
				return fmt.Errorf("sync job %s: snapshot: %w", j.Name, err)
			}
			if hasGlob(src.key) {
				return fmt.Errorf("sync job %s: snapshot: give a folder, not a pattern", j.Name)
			}
			dst, _ := requireRemote(j.Remote)
			if strings.Trim(dst.key, "/") == "" {
				return fmt.Errorf("sync job %s: a job with a snapshot syncs a folder made from it, not a whole bucket: give remote as %s:folder", j.Name, dst.bucket)
			}
		}
	}
	return nil
}

// startProject starts every job of a project, each on its own: one that
// fails leaves the rest running. A watching job that stops is started again.
func startProject(ctx context.Context, client clientFunc, p *syncproject.Project, template bool, l logger.Logger, follow eventFollower) *projectRun {
	ctx, cancel := context.WithCancel(ctx)
	r := &projectRun{project: p, template: template, cancel: cancel}
	for _, j := range p.Sync {
		jr := &jobRun{
			job:     j,
			project: p,
			log:     l.With("project", p.Name).With("job", j.Name),
			first:   make(chan struct{}),
			flush:   make(chan chan error),
			st: syncproject.JobStatus{
				Name: j.Name, Local: j.Local, Remote: j.Remote, Mode: j.SyncMode(), State: "starting",
			},
		}
		r.jobs = append(r.jobs, jr)
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			jr.run(ctx, client, follow)
		}()
	}
	return r
}

// stop stops every job and waits for them to end.
func (r *projectRun) stop() {
	r.cancel()
	r.wg.Wait()
}

// wait waits for every job's first pass, or its failure: the error says
// which failed.
func (r *projectRun) waitFirst(ctx context.Context) error {
	var errs []error
	for _, j := range r.jobs {
		select {
		case <-j.first:
		case <-ctx.Done():
			return ctx.Err()
		}
		j.mu.Lock()
		if j.firstErr != nil {
			errs = append(errs, fmt.Errorf("%s: %w", j.job.Name, j.firstErr))
		}
		j.mu.Unlock()
	}
	return errors.Join(errs...)
}

// flush has every watching job that sends local changes send them now,
// waiting until they have, or ctx ends.
func (r *projectRun) flush(ctx context.Context) error {
	var wg sync.WaitGroup
	errs := make([]error, len(r.jobs))
	for i, j := range r.jobs {
		if !j.job.Watching() || j.job.SyncMode() == syncproject.ModeDownload {
			continue
		}
		j.mu.Lock()
		watching := j.st.State == "watching"
		j.mu.Unlock()
		if !watching {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			reply := make(chan error, 1)
			select {
			case j.flush <- reply:
			case <-ctx.Done():
				errs[i] = fmt.Errorf("%s: %w", j.job.Name, ctx.Err())
				return
			}
			select {
			case err := <-reply:
				if err != nil {
					errs[i] = fmt.Errorf("%s: %w", j.job.Name, err)
				}
			case <-ctx.Done():
				errs[i] = fmt.Errorf("%s: %w", j.job.Name, ctx.Err())
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

// done waits for every job to end, as single passes do.
func (r *projectRun) done() {
	r.wg.Wait()
}

func (r *projectRun) status() syncproject.ProjectStatus {
	ps := syncproject.ProjectStatus{Name: r.project.Name, Template: r.template}
	for _, j := range r.jobs {
		j.mu.Lock()
		ps.Jobs = append(ps.Jobs, j.st)
		j.mu.Unlock()
	}
	return ps
}

// failed reports whether any job ended in failure.
func (r *projectRun) failed() bool {
	for _, j := range r.jobs {
		j.mu.Lock()
		f := j.st.State == "failed"
		j.mu.Unlock()
		if f {
			return true
		}
	}
	return false
}

func (j *jobRun) set(f func(st *syncproject.JobStatus)) {
	j.mu.Lock()
	f(&j.st)
	j.mu.Unlock()
}

// firstOver records the first pass's outcome, once.
func (j *jobRun) firstOver(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.firstDone {
		return
	}
	j.firstDone, j.firstErr = true, err
	close(j.first)
}

// run runs the job until it is done or stopped; a watching job that stops
// with an error waits a while and starts again.
func (j *jobRun) run(ctx context.Context, client clientFunc, follow eventFollower) {
	defer j.firstOver(context.Canceled) // stopped before a first pass
	wait := jobRetryMin
	for {
		err := j.attempt(ctx, client, follow)
		if ctx.Err() != nil {
			j.set(func(st *syncproject.JobStatus) { st.State = "stopped" })
			return
		}
		if err == nil {
			j.set(func(st *syncproject.JobStatus) { st.State, st.Error = "done", "" })
			return
		}
		j.firstOver(err)
		j.set(func(st *syncproject.JobStatus) { st.State, st.Error = "failed", err.Error() })
		if !j.job.Watching() {
			j.log.WithError(err).Error("sync failed")
			return
		}
		j.log.WithError(err).Warn("sync stopped: starting it again", "in", wait)
		select {
		case <-ctx.Done():
			j.set(func(st *syncproject.JobStatus) { st.State = "stopped" })
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, jobRetryMax)
	}
}

// attempt makes the job's folder from its snapshot when it does not exist
// yet, then syncs: once, or watching until stopped.
func (j *jobRun) attempt(ctx context.Context, client clientFunc, follow eventFollower) error {
	c, err := client()
	if err != nil {
		return err
	}
	remote, err := requireRemote(j.job.Remote)
	if err != nil {
		return err
	}
	if j.job.Snapshot != "" {
		j.set(func(st *syncproject.JobStatus) { st.State = "snapshotting" })
		if err := ensureSnapshot(ctx, c, j.job.Snapshot, remote, j.log); err != nil {
			return err
		}
	}

	dir, err := filepath.Abs(expandHome(j.job.Local))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	opt := syncOptions{
		dir:             dir,
		bucket:          remote.bucket,
		prefix:          dirPrefix(remote.key),
		del:             j.job.Deleting(),
		allowMassDelete: j.job.AllowMassDelete,
		watching:        j.job.Watching(),
		log:             j.log,
		flush:           j.flush,
	}
	switch j.job.SyncMode() {
	case syncproject.ModeUpload:
		opt.mode = modeUp
	case syncproject.ModeDownload:
		opt.mode = modeDown
	default:
		opt.mode = modeTwoWay
	}
	patterns := []string{snapshotManifest}
	// As knot file sync: watched and two-way syncs leave out version
	// control folders and editor scratch files, unless the job or the
	// project says to sync everything its patterns do not leave out.
	if (opt.watching || opt.mode == modeTwoWay) && !j.job.NoDefaultExcludes && !j.project.NoDefaultExcludes {
		patterns = append(patterns, excludes.Defaults...)
	}
	patterns = append(patterns, j.project.Exclude...)
	patterns = append(patterns, j.job.Exclude...)
	if j.job.ExcludeFrom != "" {
		more, err := j.excludeFrom(ctx, c, dir, remote)
		if err != nil {
			return err
		}
		patterns = append(patterns, more...)
	}
	opt.excludes = excludes.Compile(patterns)

	j.set(func(st *syncproject.JobStatus) { st.State = "syncing" })
	opt.onPass = func(pc passCounts, err error) {
		j.set(func(st *syncproject.JobStatus) {
			st.Passes++
			st.LastPass = time.Now().UTC().Format(time.RFC3339)
			st.Uploaded += pc.uploaded
			st.Downloaded += pc.downloaded
			st.Deleted += pc.deleted
			st.Conflicts += pc.conflicts
			st.Error = ""
			if err != nil {
				st.Error = err.Error()
			}
			if opt.watching {
				st.State = "watching"
			}
		})
		j.firstOver(err)
	}
	return runEngine(ctx, c, opt, follow)
}

// excludeFrom reads the job's exclude_from file: from the directory, or,
// when it is not there yet (the folder still to come down), from the
// bucket folder.
func (j *jobRun) excludeFrom(ctx context.Context, c *apiclient.ApiClient, dir string, remote location) ([]string, error) {
	rel := filepath.ToSlash(filepath.Clean(j.job.ExcludeFrom))
	if filepath.IsAbs(j.job.ExcludeFrom) || rel == ".." || strings.HasPrefix(rel, "../") {
		return nil, fmt.Errorf("exclude_from %s: give a file inside the directory", j.job.ExcludeFrom)
	}
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if errors.Is(err, os.ErrNotExist) {
		data, err = readRemote(ctx, c, remote.bucket, dirPrefix(remote.key)+rel)
		if apiclient.StatusCode(err) == 404 {
			j.log.Debug("exclude_from file not found: nothing more left out", "file", rel)
			return nil, nil
		}
	}
	if err != nil {
		return nil, fmt.Errorf("exclude_from %s: %w", rel, apiError(err))
	}
	patterns, skipped := syncproject.GitignorePatterns(data)
	if len(skipped) > 0 {
		j.log.Warn("exclude_from: patterns that put files back (!) are not supported and were skipped", "file", rel, "patterns", strings.Join(skipped, " "))
	}
	return patterns, nil
}

// readRemote reads a whole bucket file.
func readRemote(ctx context.Context, c *apiclient.ApiClient, bucket, key string) ([]byte, error) {
	resp, err := c.GetFileObject(ctx, bucket, key)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// ensureSnapshot makes the job's bucket folder from its snapshot source
// when the folder does not exist: every later start finds it and syncs it
// as it is. A folder being made by a snapshot (another space starting at
// the same moment) is waited for.
func ensureSnapshot(ctx context.Context, c *apiclient.ApiClient, from string, remote location, l logger.Logger) error {
	src, err := requireRemote(from)
	if err != nil {
		return err
	}
	prefix := dirPrefix(remote.key)
	exists := func() (bool, error) {
		list, err := c.ListFileObjectsMarkers(ctx, remote.bucket, prefix, "", "", 1)
		if err != nil {
			return false, apiError(err)
		}
		return len(list.Objects) > 0 || len(list.Prefixes) > 0, nil
	}
	ok, err := exists()
	if err != nil {
		return err
	}
	if !ok {
		snap, err := c.SnapshotFileFolder(ctx, apiclient.FileSnapshotRequest{
			SourceBucket: src.bucket, Source: src.key,
			DestBucket: remote.bucket, Dest: remote.key,
		})
		if err == nil {
			l.Info("made the folder from its snapshot", "folder", remoteName(remote.bucket, prefix), "from", from, "files", snap.Files)
			return nil
		}
		// Made meanwhile, by another space starting with it?
		if ok, _ := exists(); !ok {
			return fmt.Errorf("snapshot of %s to %s: %w", from, remoteName(remote.bucket, prefix), apiError(err))
		}
	}

	// The folder is there: whole, once its manifest is. One without, being
	// made, is waited for a while; one that vanishes, its snapshot having
	// failed, is made again.
	deadline := time.Now().Add(snapshotWaitFor)
	for {
		if _, err := c.HeadFileObject(ctx, remote.bucket, prefix+snapshotManifest); err == nil {
			return nil
		} else if apiclient.StatusCode(err) != 404 {
			return apiError(err)
		}
		if ok, err := exists(); err != nil {
			return err
		} else if !ok {
			return ensureSnapshot(ctx, c, from, remote, l)
		}
		if time.Now().After(deadline) {
			l.Warn("the folder has no snapshot manifest, so was not made by a snapshot, or one that did not finish: syncing it as it is", "folder", remoteName(remote.bucket, prefix))
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(snapshotWaitPoll):
		}
	}
}
