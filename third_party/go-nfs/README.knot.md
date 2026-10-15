# go-nfs (patched for knot)

A copy of [github.com/willscott/go-nfs](https://github.com/willscott/go-nfs)
v0.0.4 (Apache-2.0, see LICENSE), used through a `replace` in knot's go.mod by
`internal/nfsserver`. Only the library sources are kept: no tests, examples or
helpers. Changes are marked `knot:` in the code:

- READDIR/READDIRPLUS cookies are a hash of the entry name, and a listing is
  in cookie order, so a client resuming after entries were removed (Linux
  `rm -rf`) carries on where it was instead of skipping entries. Listings are
  always read afresh; no cookie verifier is used.
- Filesystem errors map to their NFS status (`statusFromError`): not empty,
  cross-device, is/not a directory, quota and so on, where most were reported
  as IO or ACCESS.
- Variable-length opaques skip their XDR padding (`readOpaque`), so arguments
  after an unaligned file handle are read correctly; reads are complete and
  bounded.
- A filesystem implementing `Committer` gets UNSTABLE writes and a call on
  COMMIT, rather than every write being answered FILE_SYNC and COMMIT doing
  nothing.
- Requests on a connection are handled concurrently (up to 32), each read
  whole first, instead of one after another, so a client reading ahead is
  not held to one request per round trip of the filesystem behind.
- READ and WRITE sizes offered are 1MB, within the size a request may be
  read into memory, rather than 1GB.
