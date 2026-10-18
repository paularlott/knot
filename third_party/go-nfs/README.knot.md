# go-nfs (patched for knot)

A copy of [github.com/willscott/go-nfs](https://github.com/willscott/go-nfs)
at master `6274d051a57f` (26 September 2026, after v0.0.4; Apache-2.0, see
LICENSE), used through a `replace` in knot's go.mod by `internal/nfsserver`.
Only the library sources are kept: no upstream tests, examples or helpers.

knot serves NFSv3 and NFSv4.0 (`Server.EnabledNFSVersions`), and uses
upstream's concurrent request handling (`Server.ConcurrentHandlers`). The
changes below are each marked `knot:` in the code.

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
- Errors map as NFSv3's do (`statusFromError`), with NFS4ERR_XDEV,
  NFS4ERR_NAMETOOLONG and NFS4ERR_DQUOT added.
- READ and WRITE sizes offered are 1MB (`maxTransfer`), not 16MB, which with
  the request around it is more than a request may be.

To move to a newer upstream: copy its library sources over these, keep
`xdr.go`, `knot_test.go` and this file, check whether each change above is
still needed, and re-apply those that are.
