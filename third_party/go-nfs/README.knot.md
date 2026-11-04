# go-nfs (patched for knot)

A copy of [github.com/willscott/go-nfs](https://github.com/willscott/go-nfs)
at master `6274d051a57f` (26 September 2026, after v0.0.4; Apache-2.0, see
LICENSE), used through a `replace` in knot's go.mod by `internal/nfsserver`.
Only the library sources are kept: no upstream tests, examples or helpers.

knot serves NFSv3 and NFSv4.0 (`Server.EnabledNFSVersions`), and uses
upstream's concurrent request handling (`Server.ConcurrentHandlers`). The
changes below are each marked `knot:` in the code. Upstream master was
checked again on 31 October 2026 and had not moved.

Both versions:

- A panic in a request's handler is recovered and answered with a system
  error, logged with its stack (`conn.callHandler`), as upstream's open
  PR #151 proposes; upstream let it end the process.

NFSv3:

- READDIR/READDIRPLUS cookies are a hash of the entry name, and a listing is
  in cookie order, so a client resuming after entries were removed (Linux
  `rm -rf`) carries on where it was instead of skipping entries. Listings are
  always read afresh; no cookie verifier is used. (`getDirListing`,
  `cookieFor`; NFSv4 keeps upstream's `getDirListingWithVerifier`.)
- An UNCHECKED CREATE of a file that exists leaves it as it is, applying only
  a size given, as Linux's server does. Upstream creates the file again,
  emptying it, so an open with O_CREAT by a client without the file cached
  (`dd conv=notrunc`, `>>` in a shell) loses its content.
- Variable-length opaques skip their XDR padding (`readOpaque`), so arguments
  after an unaligned file handle are read correctly; reads are bounded.
  go-nfs-client's `xdr.ReadOpaque`, used upstream, ignores the padding.
- A filesystem implementing `Committer` gets UNSTABLE writes and a call on
  COMMIT, rather than every write being answered FILE_SYNC and COMMIT doing
  nothing.
- Filesystem errors from create, mkdir, rename, symlink and remove map to
  their NFS status (`statusFromError`): cross-device, is/not a directory,
  quota, invalid name and so on, where upstream reports IO or ACCESS.
  (Upstream's RMDIR now finds a directory not empty itself.)
- SETATTR of a file that can't be looked at (its server unreachable, say)
  reports the error (`SetFileAttributes.Apply`); upstream answered OK
  though nothing was set.
- READ and WRITE sizes offered are 1MB (`maxTransfer`), within the 16MB a
  request may be; upstream offers 1GB.

NFSv4.0:

- READDIR uses the same name-hash cookies, so a listing that changes between
  calls is resumed correctly instead of refused (BAD_COOKIE) or skipped.
- WRITE, COMMIT and CLOSE use `Committer` as NFSv3 does; CLOSE commits too,
  so a file is saved when it is closed.
- SETATTR, and the attributes given to OPEN and CREATE, accept owner,
  owner_group (by number), hidden and system, which are listed as supported:
  macOS sets hidden on everything it creates, which upstream refused, so no
  file or folder could be created from macOS.
- CREATE makes symbolic links (NF4LNK) where the filesystem can; upstream
  refused them.
- CREATE of a folder sets the attributes given (its mode, from
  `mkdir -m`), as NFSv3's MKDIR and NFSv4's OPEN do; upstream dropped them,
  so every folder got the default mode.
- CREATE of a name that exists answers NFS4ERR_EXIST (RFC 7530 18.4.4);
  upstream answered every CREATE of a folder that existed as if it had made
  it. NFSv3's MKDIR answers EXIST for a file of that name too, not only a
  folder. Both check and make under a lock on the name (`createlock.go`),
  so of clients racing to make a folder one does and the others are told
  it exists.
- Open and lock state follows its file. On a RENAME, NFSv4's or NFSv3's,
  it moves to the new path (`nfs4StateManager.rename`), and what the rename
  replaced lets go of its path; on a REMOVE it lets go of the path
  (`removed`), its locks dropped. The lock table follows through
  `LockPaths`, which the in-memory locker implements. Upstream kept state
  under the path it was taken on: the lock stateid of a file renamed while
  locked was NFS4ERR_BAD_STATEID, and the locks of a file removed while
  locked passed to the next file made with its name, where no one would
  release them. The Mac's client, many processes making folders at once,
  renames, removes and remakes the AppleDouble `._` file beside a folder
  while others hold locks on it, and both hung every process using the
  mount: the first sent it reclaiming its opens forever, the second left
  processes waiting forever for a lock.
- A handle that no longer resolves, its file removed, is a ghost: PUTFH
  succeeds, the operations that need the file answer NFS4ERR_STALE as PUTFH
  did, but CLOSE and LOCKU act by stateid, answering NFS4ERR_NOENT for state
  already gone, and the GETATTR after such a CLOSE answers NFS4ERR_NOENT
  (`nfs4OnPutFH`). The Mac's client, letting go of a file it held open
  when it was removed, sends PUTFH CLOSE GETATTR and on any error but
  NOENT sends it again at once, forever (NFS.kext `nfs_vnop_inactive`),
  every process using that folder waiting behind it, and unmounting too.
- An operation on the current filehandle (READ, WRITE, COMMIT, GETATTR,
  SETATTR, READDIR, READLINK) whose file is gone answers NFS4ERR_STALE, not
  NFS4ERR_NOENT, which RFC 7530 keeps for operations that look a name up
  (`nfs4FileStatusFromErr`); CLOSE of a file removed meanwhile closes its
  open without saving.
- A special stateid (all zeros or all ones) where state is needed is
  NFS4ERR_BAD_STATEID (RFC 7530 9.1.4.3), not NFS4ERR_STALE_STATEID, which
  tells the client the server restarted, with one exception: a LOCK for a
  new lock owner under the anonymous open stateid is granted, a lock with
  no open behind it. The Mac's client sends one for a file it has lost its
  open of (one process closing it as another locks it), and any refusal
  starts the recovery below.
- Reclaims are granted, at any time: an OPEN with CLAIM_PREVIOUS opens the
  current file afresh (`nfs4Reclaim`), and a LOCK with reclaim set is a
  LOCK, DENIED if another owner took the range meanwhile. Upstream answered
  NFS4ERR_NO_GRACE, keeping no state across a restart, and the Mac's client,
  told so, starts its recovery again a second later, forever, every process
  using the mount hung: after this server restarts, and after any of its
  stateids is refused. There are no share reservations or delegations that
  a reclaim could contradict.
- Errors map as NFSv3's do (`statusFromError`), with NFS4ERR_XDEV,
  NFS4ERR_NAMETOOLONG and NFS4ERR_DQUOT added.
- READ and WRITE sizes offered are 1MB (`maxTransfer`), not 16MB, which with
  the request around it is more than a request may be.

To move to a newer upstream: copy its library sources over these, keep
`xdr.go`, `knot_test.go` and this file, check whether each change above is
still needed, and re-apply those that are.
