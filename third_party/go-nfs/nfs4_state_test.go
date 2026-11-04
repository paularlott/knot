package nfs

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"testing"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/willscott/go-nfs-client/nfs/xdr"
)

// A special stateid, which names no state, is BAD_STATEID where state is
// needed, not STALE_STATEID, which tells the client the server restarted.
func TestSpecialStateIDsAreBad(t *testing.T) {
	sm := newNFS4StateManager(NewMemoryLocker(0, 0))
	for name, id := range map[string]nfs4StateID{"anonymous": nfs4AnonymousStateID, "bypass": nfs4BypassStateID, "zero with a seqid": {Seqid: 1}} {
		if _, status := sm.findLocked(nfs4OpenState, id); status != nfs4ErrBadStateID {
			t.Errorf("the %s stateid as an open stateid: %d, want BAD_STATEID", name, status)
		}
		if id == nfs4AnonymousStateID {
			continue
		}
		if _, _, status := sm.lock(id, nfs4Owner{ClientID: 1, Owner: "o"}, "f", LockRange{Start: 0, End: 1}); status != nfs4ErrBadStateID {
			t.Errorf("LOCK with the %s stateid: %d, want BAD_STATEID", name, status)
		}
	}
	// A stateid of another server instance is still stale.
	var other nfs4StateID
	binary.BigEndian.PutUint32(other.Other[0:4], sm.boot+1)
	if _, status := sm.findLocked(nfs4OpenState, other); status != nfs4ErrStaleStateID {
		t.Errorf("a stateid from another server instance: %d, want STALE_STATEID", status)
	}
}

// newClient returns a confirmed client of sm.
func newClient(t *testing.T, sm *nfs4StateManager, name string) uint64 {
	t.Helper()
	id, confirm := sm.setClientID([]byte(name), [8]byte{1})
	if status := sm.confirmClientID(id, confirm); status != nfs4OK {
		t.Fatalf("confirming client %s: %d", name, status)
	}
	return id
}

// A file renamed while open and locked keeps its open and its locks under
// its new name: the Mac's client renames a file removed while open to
// ".nfs...", then unlocks and closes it there. Refused as BAD_STATEID, it
// took the server to have lost its state and hung the mount.
func TestStateFollowsRename(t *testing.T) {
	sm := newNFS4StateManager(NewMemoryLocker(0, 0))
	a := nfs4Owner{ClientID: newClient(t, sm, "a"), Owner: "o"}
	b := nfs4Owner{ClientID: newClient(t, sm, "b"), Owner: "o"}
	whole := LockRange{Start: 0, End: 10, Exclusive: true}

	open, status := sm.open(a, "d/._f")
	if status != nfs4OK {
		t.Fatal(status)
	}
	lock, _, status := sm.lock(open, a, "d/._f", whole)
	if status != nfs4OK {
		t.Fatal(status)
	}
	sm.rename("d/._f", "d/.nfs.1")

	if denied, _ := sm.test(b, "d/.nfs.1", whole); denied == nil {
		t.Error("the lock did not move with the file")
	}
	if denied, _ := sm.test(b, "d/._f", whole); denied != nil {
		t.Error("the lock is still held under the old name")
	}
	if _, _, status := sm.lockByStateID(lock, "d/.nfs.1", whole); status != nfs4OK {
		t.Errorf("LOCK with the lock stateid under the new name: %d", status)
	}
	if _, status := sm.unlock(lock, "d/.nfs.1", whole); status != nfs4OK {
		t.Errorf("LOCKU under the new name: %d", status)
	}
	if _, status := sm.close(open); status != nfs4OK {
		t.Errorf("CLOSE after the rename: %d", status)
	}

	// A folder's rename moves the state of the files in it.
	open, _ = sm.open(a, "x/f")
	lock, _, _ = sm.lock(open, a, "x/f", whole)
	other, _ := sm.open(a, "xy/f")
	sm.lock(other, a, "xy/f", whole)
	sm.rename("x", "y")
	if _, status := sm.unlock(lock, "y/f", whole); status != nfs4OK {
		t.Errorf("LOCKU of a file whose folder was renamed: %d", status)
	}
	if denied, _ := sm.test(b, "xy/f", whole); denied == nil {
		t.Error("a rename of x moved the locks of xy")
	}

	// A rename over an open file leaves that file's open closable, and the
	// renamed file's state under the name.
	replaced, _ := sm.open(b, "t")
	sm.lock(replaced, b, "t", whole)
	moved, _ := sm.open(b, "s")
	sm.rename("s", "t")
	if denied, _ := sm.test(a, "t", whole); denied != nil {
		t.Error("the replaced file's lock still holds the name")
	}
	if _, status := sm.close(replaced); status != nfs4OK {
		t.Errorf("CLOSE of the replaced file: %d", status)
	}
	if _, _, status := sm.lock(moved, b, "t", whole); status != nfs4OK {
		t.Errorf("LOCK of the renamed file under its new name: %d", status)
	}
}

// A lock under the anonymous stateid, with no open behind it, is granted:
// the Mac's client sends one for a file it lost its open of, and refused,
// it hangs the mount reclaiming its opens.
func TestLockWithoutOpen(t *testing.T) {
	sm := newNFS4StateManager(NewMemoryLocker(0, 0))
	a := nfs4Owner{ClientID: newClient(t, sm, "a"), Owner: "o"}
	b := nfs4Owner{ClientID: newClient(t, sm, "b"), Owner: "o"}
	whole := LockRange{Start: 0, End: 10, Exclusive: true}
	lock, _, status := sm.lock(nfs4AnonymousStateID, a, "f", whole)
	if status != nfs4OK {
		t.Fatalf("LOCK under the anonymous stateid: %d", status)
	}
	if _, _, status := sm.lock(nfs4AnonymousStateID, b, "f", whole); status != nfs4ErrDenied {
		t.Errorf("a second owner's conflicting LOCK: %d, want DENIED", status)
	}
	if _, status := sm.unlock(lock, "f", whole); status != nfs4OK {
		t.Errorf("LOCKU: %d", status)
	}
	if _, _, status := sm.lock(nfs4AnonymousStateID, nfs4Owner{ClientID: 99, Owner: "o"}, "f", whole); status == nfs4OK {
		t.Error("a LOCK from an unknown client was granted")
	}
}

// An OPEN reclaiming an open, as a client recovering from a server restart
// sends, is granted as a fresh open of the current file. Answered
// NO_GRACE, the Mac's client reclaims again every second, the mount hung.
func TestReclaimOpen(t *testing.T) {
	fs := memfs.New()
	f, _ := fs.Create("f")
	f.Close()
	srv := &Server{Handler: pathHandler{}}
	sm := srv.nfs4State()
	owner := nfs4Owner{ClientID: newClient(t, sm, "a"), Owner: "o"}
	open := func(claim, delegate uint32) (nfs4Status, nfs4OpenRes) {
		var args bytes.Buffer
		if err := xdr.Write(&args, nfs4OpenArgs{Seqid: 1, ShareAccess: nfs4ShareAccessRead | nfs4ShareAccessWrite, Owner: owner, Claim: claim, DelegateType: delegate}); err != nil {
			t.Fatal(err)
		}
		c := &nfs4Compound{ctx: context.Background(), w: &response{conn: &conn{Server: srv}}, handler: pathHandler{}, current: &nfs4FileHandle{fs: fs, path: []string{"f"}}}
		var out bytes.Buffer
		status := nfs4OnOpen(c, &args, &out)
		var res nfs4OpenRes
		if status == nfs4OK {
			if err := xdr.Read(&out, &res); err != nil {
				t.Fatal(err)
			}
		}
		return status, res
	}
	status, res := open(nfs4ClaimPrevious, nfs4OpenDelegateNone)
	if status != nfs4OK {
		t.Fatalf("reclaim OPEN: %d", status)
	}
	if _, status := sm.close(res.StateID); status != nfs4OK {
		t.Errorf("CLOSE of the reclaimed open: %d", status)
	}
	if status, _ := open(nfs4ClaimPrevious, 1); status != nfs4ErrReclaimBad {
		t.Errorf("reclaim of a delegation never granted: %d, want RECLAIM_BAD", status)
	}
}

// A file removed while locked leaves its locks with it: a file made later
// with the same name is not held by a lock no one will release. The Mac's
// client removes and remakes the "._" file beside a folder while another
// process holds a lock on it.
func TestStateGoesWithRemove(t *testing.T) {
	sm := newNFS4StateManager(NewMemoryLocker(0, 0))
	a := nfs4Owner{ClientID: newClient(t, sm, "a"), Owner: "o"}
	b := nfs4Owner{ClientID: newClient(t, sm, "b"), Owner: "o"}
	whole := LockRange{Start: 0, End: 10, Exclusive: true}

	open, _ := sm.open(a, "d/._f")
	if _, _, status := sm.lock(open, a, "d/._f", whole); status != nfs4OK {
		t.Fatal(status)
	}
	sm.removed("d/._f")

	again, _ := sm.open(b, "d/._f")
	if _, _, status := sm.lock(again, b, "d/._f", whole); status != nfs4OK {
		t.Errorf("LOCK of a new file with a removed one's name: %d, want granted", status)
	}
	if _, status := sm.close(open); status != nfs4OK {
		t.Errorf("CLOSE of the removed file: %d", status)
	}

	// A folder's removal lets go of what was below it.
	open, _ = sm.open(a, "e/g")
	sm.lock(open, a, "e/g", whole)
	sm.removed("e")
	if denied, _ := sm.test(b, "e/g", whole); denied != nil {
		t.Error("a lock below a removed folder still holds")
	}
}

// ghostHandler resolves no handle, as for a file that was removed.
type ghostHandler struct{ pathHandler }

func (ghostHandler) FromHandle([]byte) (billy.Filesystem, []string, error) {
	return nil, nil, os.ErrNotExist
}

// The open and locks of a file removed meanwhile are closed and released
// by stateid through its stale handle, and the GETATTR after the CLOSE is
// NOENT: the Mac's client, letting go of such a file, sends PUTFH CLOSE
// GETATTR again at once on any other error, forever.
func TestGhostHandleCloses(t *testing.T) {
	srv := &Server{Handler: ghostHandler{}}
	sm := srv.nfs4State()
	owner := nfs4Owner{ClientID: newClient(t, sm, "a"), Owner: "o"}
	open, _ := sm.open(owner, "d/._f")
	lock, _, status := sm.lock(open, owner, "d/._f", LockRange{Start: 0, End: 10, Exclusive: true})
	if status != nfs4OK {
		t.Fatal(status)
	}
	sm.removed("d/._f")

	compound := func(ops ...func(c *nfs4Compound) nfs4Status) []nfs4Status {
		c := &nfs4Compound{ctx: context.Background(), w: &response{conn: &conn{Server: srv}}, handler: ghostHandler{}}
		var out []nfs4Status
		for _, op := range ops {
			status := op(c)
			out = append(out, status)
			if status != nfs4OK {
				break
			}
		}
		return out
	}
	putfh := func(c *nfs4Compound) nfs4Status {
		var args bytes.Buffer
		xdr.Write(&args, nfs4PutFHArgs{Handle: []byte("d/._f")})
		return nfs4OnPutFH(c, &args, &bytes.Buffer{})
	}
	locku := func(c *nfs4Compound) nfs4Status {
		var args bytes.Buffer
		xdr.Write(&args, nfs4LockUArgs{LockType: 2, Seqid: 1, StateID: lock, Offset: 0, Length: 10})
		return nfs4OnLockU(c, &args, &bytes.Buffer{})
	}
	closeOp := func(c *nfs4Compound) nfs4Status {
		var args bytes.Buffer
		xdr.Write(&args, nfs4CloseArgs{Seqid: 1, StateID: open})
		return nfs4OnClose(c, &args, &bytes.Buffer{})
	}
	getattr := func(c *nfs4Compound) nfs4Status {
		var args bytes.Buffer
		xdr.Write(&args, nfs4GetAttrArgs{})
		return nfs4OnGetAttr(c, &args, &bytes.Buffer{})
	}

	if got := compound(putfh, getattr); got[len(got)-1] != nfs4ErrStale {
		t.Errorf("GETATTR of a removed file: %v, want STALE", got)
	}
	if got := compound(putfh, locku); got[len(got)-1] != nfs4OK {
		t.Errorf("LOCKU of a removed file: %v, want OK", got)
	}
	if got := compound(putfh, closeOp, getattr); len(got) != 3 || got[1] != nfs4OK || got[2] != nfs4ErrNoEnt {
		t.Errorf("PUTFH CLOSE GETATTR of a removed file: %v, want OK OK NOENT", got)
	}
	if got := compound(putfh, closeOp); got[len(got)-1] != nfs4ErrNoEnt {
		t.Errorf("CLOSE again: %v, want NOENT", got)
	}
}
