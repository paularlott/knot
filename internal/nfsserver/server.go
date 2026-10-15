// Package nfsserver serves knot's file storage over NFSv3, so buckets can be
// mounted by the operating system (the macOS Finder or mount_nfs) and by
// Docker and Podman as volumes with the local driver's nfs type, without
// FUSE. It is a client of a knot server's files API, acting as the user whose
// token it is given, so any number can run against one knot server.
//
// NFS itself carries no authentication, so the server only accepts
// connections from the addresses it is told to trust: loopback by default.
package nfsserver

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	billy "github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/helper/chroot"
	"github.com/paularlott/knot/internal/log"
	nfs "github.com/willscott/go-nfs"
)

type Config struct {
	Listen      string        // address to listen on, e.g. 127.0.0.1:2049
	Allow       []string      // client addresses or CIDRs allowed to connect; loopback when empty
	FlushDelay  time.Duration // save a written file once writes stop for this long
	MaxDirtyAge time.Duration // and at the latest this long after it changed
	StagingDir  string        // where written files wait to be saved; a temp dir when empty
	Uid, Gid    uint32        // owner of files with none stored
}

type Server struct {
	listener net.Listener
	staging  *staging
	ownDir   string
	done     chan struct{}
}

// Start serves file storage from backend over NFS until Close.
func Start(backend Backend, cfg Config) (*Server, error) {
	allow, err := parseAllow(cfg.Allow)
	if err != nil {
		return nil, err
	}
	if cfg.FlushDelay <= 0 {
		cfg.FlushDelay = 2 * time.Second
	}
	if cfg.MaxDirtyAge <= 0 {
		cfg.MaxDirtyAge = 30 * time.Second
	}

	srv := &Server{done: make(chan struct{})}
	dir := cfg.StagingDir
	if dir == "" {
		if dir, err = os.MkdirTemp("", "knot-nfs-"); err != nil {
			return nil, err
		}
		srv.ownDir = dir
	} else if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	fsys := newStoreFS(backend)
	fsys.uid, fsys.gid = cfg.Uid, cfg.Gid
	fsys.staging = newStaging(fsys, dir, cfg.FlushDelay, cfg.MaxDirtyAge)
	srv.staging = fsys.staging

	l, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		srv.cleanup()
		return nil, err
	}
	srv.listener = &allowListener{Listener: l, allow: allow}

	h := newHandler(fsys)
	go func() {
		defer close(srv.done)
		err := nfs.Serve(srv.listener, h)
		if err != nil && !errors.Is(err, net.ErrClosed) {
			log.WithGroup("nfs").Error("nfs server stopped", "error", err)
		}
	}()
	return srv, nil
}

// Addr is the address the server listens on.
func (s *Server) Addr() net.Addr { return s.listener.Addr() }

// Close stops accepting connections and saves every file still being
// written. Connections already open are not interrupted.
func (s *Server) Close() error {
	err := s.listener.Close()
	<-s.done
	s.staging.close()
	s.cleanup()
	return err
}

func (s *Server) cleanup() {
	if s.ownDir != "" {
		os.RemoveAll(s.ownDir)
	}
}

// ---------------------------------------------------------------------------
// Connection filter
// ---------------------------------------------------------------------------

func parseAllow(list []string) ([]netip.Prefix, error) {
	if len(list) == 0 {
		list = []string{"127.0.0.0/8", "::1/128"}
	}
	var out []netip.Prefix
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.Contains(s, "/") {
			a, err := netip.ParseAddr(s)
			if err != nil {
				return nil, fmt.Errorf("nfs allow %q: %w", s, err)
			}
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("nfs allow %q: %w", s, err)
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

// allowListener drops connections from addresses not allowed. File handles
// name paths, so a client that can connect can reach every file the user
// can: refusing the connection is the only access control.
type allowListener struct {
	net.Listener
	allow []netip.Prefix
}

func (l *allowListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if l.allowed(c.RemoteAddr()) {
			return c, nil
		}
		log.WithGroup("nfs").Warn("refused connection", "remote", c.RemoteAddr().String())
		c.Close()
	}
}

func (l *allowListener) allowed(a net.Addr) bool {
	ap, err := netip.ParseAddrPort(a.String())
	if err != nil {
		return false
	}
	addr := ap.Addr().Unmap()
	for _, p := range l.allow {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// NFS handler
// ---------------------------------------------------------------------------

// handler maps NFS file handles to paths. A handle is the path itself when
// it fits, so handles survive restarts and are never evicted; longer paths
// get a hash, remembered for the life of the process. A handle also carries
// the inode number of a file or folder that has one (see metaIno).
//
// A handle whose path no longer exists, or now holds a different file, is
// stale: after a rename the client then looks the name up again rather than
// reaching whatever is now at the old path.
type handler struct {
	fs *storeFS

	mu   sync.RWMutex
	long map[[16]byte]string
}

const (
	maxHandle    = 64 // NFSv3 limit
	handlePath   = 1  // kind, inode number (8 bytes), path
	handleHashed = 2  // kind, inode number, hash of the path (16 bytes)
	handleHeader = 9
)

func newHandler(fsys *storeFS) *handler {
	return &handler{fs: fsys, long: map[[16]byte]string{}}
}

var _ nfs.Handler = (*handler)(nil)

// Mount exports the whole of file storage at "/", or a bucket or a folder
// in one by its path.
func (h *handler) Mount(ctx context.Context, conn net.Conn, req nfs.MountRequest) (nfs.MountStatus, billy.Filesystem, []nfs.AuthFlavor) {
	dir := path.Clean("/" + string(req.Dirpath))
	logger := log.WithGroup("nfs")
	if dir == "/" {
		logger.Info("mount", "path", dir, "remote", conn.RemoteAddr().String())
		return nfs.MountStatusOk, h.fs, []nfs.AuthFlavor{nfs.AuthFlavorNull}
	}
	fi, err := h.fs.Stat(dir)
	if err != nil {
		logger.Warn("mount refused", "path", dir, "error", err)
		if errors.Is(err, os.ErrNotExist) {
			return nfs.MountStatusErrNoEnt, nil, nil
		}
		return nfs.MountStatusErrAcces, nil, nil
	}
	if !fi.IsDir() {
		return nfs.MountStatusErrNotDir, nil, nil
	}
	logger.Info("mount", "path", dir, "remote", conn.RemoteAddr().String())
	return nfs.MountStatusOk, chroot.New(h.fs, dir), []nfs.AuthFlavor{nfs.AuthFlavorNull}
}

func (h *handler) Change(billy.Filesystem) billy.Change { return h.fs }

func (h *handler) FSStat(ctx context.Context, f billy.Filesystem, s *nfs.FSStat) error {
	s.TotalSize = 1 << 50
	s.FreeSize = 1 << 49
	s.AvailableSize = 1 << 49
	s.TotalFiles = 1 << 32
	s.FreeFiles = 1 << 31
	s.AvailableFiles = 1 << 31
	return nil
}

func (h *handler) ToHandle(f billy.Filesystem, p []string) []byte {
	if c, ok := f.(*chroot.ChrootHelper); ok {
		root := strings.Trim(c.Root(), "/")
		if root != "" {
			p = append(strings.Split(root, "/"), p...)
		}
	}
	joined := strings.Join(p, "/")

	var ino uint64
	if fi, err := h.fs.Stat(joined); err == nil {
		if info, ok := fi.(*fileInfo); ok {
			ino = info.ino
		}
	}

	if handleHeader+len(joined) <= maxHandle {
		fh := binary.BigEndian.AppendUint64([]byte{handlePath}, ino)
		return append(fh, joined...)
	}
	sum := sha256.Sum256([]byte(joined))
	var id [16]byte
	copy(id[:], sum[:])
	h.mu.Lock()
	h.long[id] = joined
	h.mu.Unlock()
	fh := binary.BigEndian.AppendUint64([]byte{handleHashed}, ino)
	return append(fh, id[:]...)
}

func (h *handler) FromHandle(fh []byte) (billy.Filesystem, []string, error) {
	if len(fh) < handleHeader {
		return nil, nil, &nfs.NFSStatusError{NFSStatus: nfs.NFSStatusBadHandle}
	}
	ino := binary.BigEndian.Uint64(fh[1:handleHeader])
	var joined string
	switch fh[0] {
	case handlePath:
		joined = string(fh[handleHeader:])
	case handleHashed:
		if len(fh) < handleHeader+16 {
			return nil, nil, &nfs.NFSStatusError{NFSStatus: nfs.NFSStatusBadHandle}
		}
		var id [16]byte
		copy(id[:], fh[handleHeader:])
		h.mu.RLock()
		s, ok := h.long[id]
		h.mu.RUnlock()
		if !ok {
			return nil, nil, &nfs.NFSStatusError{NFSStatus: nfs.NFSStatusStale}
		}
		joined = s
	default:
		return nil, nil, &nfs.NFSStatusError{NFSStatus: nfs.NFSStatusBadHandle}
	}

	fi, err := h.fs.Stat(joined)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, &nfs.NFSStatusError{NFSStatus: nfs.NFSStatusStale}
		}
		return nil, nil, err
	}
	// A handle made before its file or folder had an inode number cannot
	// tell it from another at the same path, so is taken as it is: a
	// folder gets one when its attributes are first set.
	if info, ok := fi.(*fileInfo); ok && ino != 0 && info.ino != ino {
		return nil, nil, &nfs.NFSStatusError{NFSStatus: nfs.NFSStatusStale}
	}

	if joined == "" {
		return h.fs, []string{}, nil
	}
	return h.fs, strings.Split(joined, "/"), nil
}

func (h *handler) InvalidateHandle(billy.Filesystem, []byte) error { return nil }

func (h *handler) HandleLimit() int { return 1 << 30 }
