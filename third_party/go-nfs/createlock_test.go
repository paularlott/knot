package nfs

import (
	"bytes"
	"context"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/willscott/go-nfs-client/nfs/xdr"
)

// slowFS is a memfs safe to use from several goroutines, whose folders
// take a moment to make, as knot's do (a call to its server).
type slowFS struct {
	billy.Filesystem
	mu sync.Mutex
}

func (f *slowFS) Lstat(p string) (os.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Filesystem.Lstat(p)
}

func (f *slowFS) Stat(p string) (os.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Filesystem.Stat(p)
}

func (f *slowFS) MkdirAll(p string, mode os.FileMode) error {
	time.Sleep(5 * time.Millisecond)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Filesystem.MkdirAll(p, mode)
}

type pathHandler struct{}

func (pathHandler) Mount(context.Context, net.Conn, MountRequest) (MountStatus, billy.Filesystem, []AuthFlavor) {
	return MountStatusOk, nil, nil
}
func (pathHandler) Change(billy.Filesystem) billy.Change                    { return nil }
func (pathHandler) FSStat(context.Context, billy.Filesystem, *FSStat) error { return nil }
func (pathHandler) ToHandle(_ billy.Filesystem, p []string) []byte {
	return []byte(strings.Join(p, "/"))
}
func (pathHandler) FromHandle([]byte) (billy.Filesystem, []string, error) {
	return nil, nil, os.ErrNotExist
}
func (pathHandler) InvalidateHandle(billy.Filesystem, []byte) error { return nil }
func (pathHandler) HandleLimit() int                                { return 1000 }

func create(t *testing.T, fs billy.Filesystem, name string) nfs4Status {
	t.Helper()
	var args bytes.Buffer
	if err := xdr.Write(&args, nfs4CreateArgs{Type: FileTypeDirectory, Name: name}); err != nil {
		t.Fatal(err)
	}
	c := &nfs4Compound{ctx: context.Background(), handler: pathHandler{}, current: &nfs4FileHandle{fs: fs}}
	return nfs4OnCreate(c, &args, &bytes.Buffer{})
}

// Of clients making the same folder at once, one makes it and the others
// are told it exists.
func TestCreateFolderRacing(t *testing.T) {
	fs := &slowFS{Filesystem: memfs.New()}
	const clients = 16
	got := make(chan nfs4Status, clients)
	var wg sync.WaitGroup
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got <- create(t, fs, "d")
		}()
	}
	wg.Wait()
	close(got)
	made, exist := 0, 0
	for s := range got {
		switch s {
		case nfs4OK:
			made++
		case nfs4ErrExist:
			exist++
		default:
			t.Errorf("CREATE answered %d", s)
		}
	}
	if made != 1 || exist != clients-1 {
		t.Errorf("%d CREATEs made the folder and %d were told it exists, want 1 and %d", made, exist, clients-1)
	}
	if create(t, fs, "d") != nfs4ErrExist {
		t.Error("CREATE of a folder that exists was not answered EXIST")
	}
}

// A folder is not made over a file of that name.
func TestCreateFolderOverFile(t *testing.T) {
	fs := memfs.New()
	f, err := fs.Create("f")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if s := create(t, fs, "f"); s != nfs4ErrExist {
		t.Errorf("CREATE of a folder over a file answered %d, want EXIST", s)
	}
	if info, err := fs.Lstat("f"); err != nil || info.IsDir() {
		t.Errorf("the file became %v, %v", info, err)
	}
}
