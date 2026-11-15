# knot.files - File storage library for the Knot server
#
# Buckets of files replicated across the cluster: list, read, write, copy,
# move and delete files, snapshot folders, and manage the buckets, their
# sharing and your usage. Requires knot.apiclient to be configured first.
# In a space, knot.files.sync runs sync projects in the space's agent.
#
# Buckets are named <owner>--<name>. Your own buckets can be given by their
# short name; buckets shared with you need the full name.
#
# Usage:
#   import knot.apiclient
#   import knot.files
#
#   knot.apiclient.configure("https://knot.example.com", "your-token")
#   knot.files.write_file("configs", "app/settings.toml", "debug = true\n")
#   text = knot.files.read_text("configs", "app/settings.toml")

import knot.apiclient as api
import urllib.parse

# knot.files.sync, where knot provides it, is knot.files.sync here too.
try:
    import knot.files.sync as sync
except Exception:
    pass


def _enc(s):
    """URL-encode a path segment for safe interpolation into a URL."""
    return urllib.parse.quote(str(s), safe='')


def _enc_key(key):
    """URL-encode a file key, keeping its folder separators."""
    return urllib.parse.quote(str(key), safe='/')


def _check_key(key):
    """Refuse keys a URL would rewrite to another key: empty, . or ..
    segments. The server refuses them too; checking here keeps the error
    the same whatever transport carries the request, and stops a path
    with .. being resolved away before it reaches the server."""
    if key == "":
        return
    parts = key.split("/")
    for i, part in enumerate(parts):
        if part == "." or part == ".." or (part == "" and i != len(parts) - 1):
            raise Exception("invalid key " + key + ": no empty, . or .. segments")


def _bucket(info):
    grants = []
    for g in info.get("grants") or []:
        grants.append({"type": g.get("type"), "name": g.get("name", ""), "access": g.get("access")})
    return {
        "name": info.get("name"),
        "display_name": info.get("display_name"),
        "owner": info.get("owner_name"),
        "owner_id": info.get("owner_id"),
        "access": info.get("access"),
        "size": info.get("size", 0),
        "count": info.get("count", 0),
        "shared": grants,
        "created_at": info.get("created_at"),
    }


# POSIX file types, as a file's mode keeps them.
_S_IFMT = 0o170000
_S_IFREG = 0o100000
_S_IFDIR = 0o040000
_S_IFLNK = 0o120000


def _mode(meta):
    """A file's permission bits and whether it is a symbolic link, from the
    mode its metadata keeps in octal; None for the bits when it keeps none."""
    v = meta.get("mode")
    if v is None:
        return None, False
    try:
        m = int(str(v).strip(), 8)
    except Exception:
        return None, False
    typ = m & _S_IFMT
    if typ == _S_IFLNK:
        return m & 0o777, True
    if typ == 0 or typ == _S_IFREG or typ == _S_IFDIR:
        return m & 0o777, False
    return None, False


def _file(info):
    meta = info.get("meta") or {}
    mode, link = _mode(meta)
    return {
        "key": info.get("key"),
        "size": info.get("size", 0),
        "etag": info.get("etag"),
        "sha256": info.get("sha256"),
        "content_type": info.get("content_type"),
        "modified_at": info.get("modified_at"),
        "changed_at": info.get("changed_at"),
        "mode": mode,
        "link": link,
        "meta": meta,
    }


def _principal(user, group, everyone):
    """The share target from the one of user, group or everyone that was given."""
    given = 0
    if user:
        given += 1
    if group:
        given += 1
    if everyone:
        given += 1
    if given != 1:
        raise Exception("give exactly one of user, group or everyone")
    if user:
        return {"type": "user", "name": user}
    if group:
        return {"type": "group", "name": group}
    return {"type": "all", "name": ""}


# ---------------------------------------------------------------------------
# Buckets
# ---------------------------------------------------------------------------

def list_buckets(all=False):
    """List the buckets you own or that are shared with you.

    Args:
        all: If True, list every bucket (file storage managers only)

    Returns:
        A list of bucket dicts, each containing:
        - name: The full name, <owner>--<name>
        - display_name: The short name for your own buckets, else the full name
        - owner: The owner's username
        - owner_id: The owner's user id
        - access: Your access: "owner", "write" or "read"
        - size: Total bytes of the files in the bucket
        - count: Number of files
        - shared: A list of dicts with type ("user", "group" or "all"), name and
          access ("read" or "write"); only filled in for the owner and file
          storage managers
        - created_at: Creation timestamp

    Raises:
        Exception if not configured or on API error
    """
    path = "/api/files/buckets"
    if all:
        path += "?all=true"
    response = api.get(path)

    result = []
    for info in response.get("buckets") or []:
        result.append(_bucket(info))
    return result


def get_bucket(name):
    """Get one bucket.

    Args:
        name: Bucket name, short for your own buckets, else the full name

    Returns:
        A bucket dict, as returned by list_buckets

    Raises:
        Exception if the bucket does not exist or is not visible to you
    """
    return _bucket(api.get("/api/files/buckets/" + _enc(name)))


def create_bucket(name):
    """Create a bucket you own, named <your username>--<name>.

    Args:
        name: The short name: 3 to 30 lowercase letters, digits and hyphens,
            with no "--"

    Returns:
        A bucket dict, as returned by list_buckets

    Raises:
        Exception if you may not own buckets, are at your bucket limit, or the
        name is invalid or taken
    """
    return _bucket(api.post("/api/files/buckets", {"name": name}))


def delete_bucket(name, force=False):
    """Delete a bucket.

    Args:
        name: Bucket name
        force: If True, delete the bucket's files with it; otherwise a bucket
            that holds files is refused

    Returns:
        True on success

    Raises:
        Exception on API error
    """
    path = "/api/files/buckets/" + _enc(name)
    if force:
        path += "?force=true"
    api.delete(path)
    return True


def share_bucket(name, user=None, group=None, everyone=False, access="read"):
    """Share a bucket with a user, a group or all users. Sharing again changes
    the access.

    Args:
        name: Bucket name
        user: Username or email of the user to share with
        group: Name of the group to share with
        everyone: If True, share with all users
        access: "read" or "write"

    Returns:
        The bucket dict, as returned by list_buckets

    Raises:
        Exception if you may not share it, or on API error
    """
    body = _principal(user, group, everyone)
    body["access"] = access
    return _bucket(api.post("/api/files/buckets/" + _enc(name) + "/share", body))


def unshare_bucket(name, user=None, group=None, everyone=False):
    """Stop sharing a bucket with a user, a group or all users.

    Args:
        name: Bucket name
        user: Username or email of the user
        group: Name of the group
        everyone: If True, remove the share with all users

    Returns:
        The bucket dict, as returned by list_buckets

    Raises:
        Exception if you may not share it, or on API error
    """
    return _bucket(api.post("/api/files/buckets/" + _enc(name) + "/unshare", _principal(user, group, everyone)))


def transfer_bucket(name, user, force=False):
    """Give a bucket to another user, who must be allowed to own buckets. It is
    renamed into their namespace and their quota is charged.

    Args:
        name: Bucket name
        user: Username or email of the new owner
        force: Allow the move even when it takes the new owner over their quota
            (file storage managers only)

    Returns:
        The bucket dict, as returned by list_buckets, with its new name

    Raises:
        Exception if you may not transfer it or on API error
    """
    return _bucket(api.post("/api/files/buckets/" + _enc(name) + "/transfer", {"user": user, "force": force}))


def usage():
    """Get your file storage usage and limits.

    Returns:
        A dict containing:
        - used_bytes: Bytes in the files of the buckets you own
        - files: Number of those files
        - buckets: Number of buckets you own
        - quota_bytes: Your storage limit in bytes, 0 for none
        - max_buckets: Your bucket limit, 0 for none

    Raises:
        Exception on API error
    """
    info = api.get("/api/files/usage")
    return {
        "used_bytes": info.get("used_bytes", 0),
        "files": info.get("objects", 0),
        "buckets": info.get("buckets", 0),
        "quota_bytes": info.get("quota_bytes", 0),
        "max_buckets": info.get("max_buckets", 0),
    }


def share_targets():
    """List the users and groups your buckets can be shared with or
    transferred to. Needs the share buckets or transfer buckets permission.

    Returns:
        A dict containing:
        - users: List of dicts with id, username, can_own (may own buckets,
          so may receive a transfer) and self (True for you)
        - groups: List of dicts with id and name

    Raises:
        Exception if you may neither share nor transfer buckets, or on API error
    """
    info = api.get("/api/files/share-targets")
    return {
        "users": [{
            "id": u.get("user_id"),
            "username": u.get("username", ""),
            "can_own": u.get("can_own", False),
            "self": u.get("self", False),
        } for u in info.get("users") or []],
        "groups": [{"id": g.get("group_id"), "name": g.get("name", "")} for g in info.get("groups") or []],
    }


def fsck(repair=False, deep=False, timeout=3600):
    """Check file storage for problems, and with repair fix what can be
    fixed. Only file storage administrators may run it.

    Args:
        repair: Fix what can be fixed: fetch missing content from the
            cluster, rebuild counts and indexes, remove buckets of deleted
            users, rewrite modes in octal (default False: check only)
        deep: Also read every stored content and verify its checksum (slow)
        timeout: Seconds to wait for the check to finish (default 3600)

    Returns:
        A dict containing:
        - buckets: Number of buckets checked
        - files: Number of files checked
        - found: Dict of issue kind to count found
        - repaired: Dict of issue kind to count repaired
        - issues: List of dicts with kind, bucket, key and detail (capped)
        - notes: List of notes, as strings

    Raises:
        Exception if you are not a file storage administrator, or on API error
    """
    report = api.post("/api/files/fsck", {"repair": repair, "deep": deep}, timeout=timeout)
    return {
        "buckets": report.get("buckets", 0),
        "files": report.get("files", 0),
        "found": report.get("found") or {},
        "repaired": report.get("repaired") or {},
        "issues": report.get("issues") or [],
        "notes": report.get("notes") or [],
    }


# ---------------------------------------------------------------------------
# Files
# ---------------------------------------------------------------------------

def list_files(bucket, prefix="", recursive=False):
    """List the files in a bucket.

    Args:
        bucket: Bucket name
        prefix: Only keys starting with this; end it with "/" for a folder
        recursive: If False, files below the next "/" after the prefix are
            rolled up into folders; if True, every file below the prefix is
            listed

    Returns:
        A dict containing:
        - files: A list of file dicts, as stat returns for a file
        - folders: A list of folder prefixes, each ending in "/" (empty when
          recursive)

    Raises:
        Exception if the bucket does not exist or is not visible to you
    """
    files = []
    folders = []
    after = ""
    while True:
        path = "/api/files/list/" + _enc(bucket) + "?limit=1000"
        if prefix:
            path += "&prefix=" + _enc(prefix)
        if not recursive:
            path += "&delimiter=%2F"
        if after:
            path += "&after=" + _enc(after)
        response = api.get(path)

        for info in response.get("objects") or []:
            files.append(_file(info))
        for p in response.get("prefixes") or []:
            folders.append(p)

        if not response.get("is_truncated"):
            break
        nxt = response.get("next")
        if not nxt or nxt <= after:
            raise Exception("listing " + bucket + " did not advance")
        after = nxt

    return {"files": files, "folders": folders}


def list_changes(bucket, prefix="", cursor=""):
    """List what changed in a bucket since a cursor, to follow a bucket
    without listing it again.

    Without a cursor every file is returned, with a cursor to follow on from;
    pass that cursor next time for just the files changed since, each once,
    as it is now. Keep the returned cursor for the time after. The cursor
    "now" returns no files, only a cursor to follow the bucket from now on.

    Args:
        bucket: Bucket name
        prefix: Only keys starting with this; end it with "/" for a folder
        cursor: The cursor from the last call, or "" to start

    Returns:
        A dict containing:
        - changes: A list of file dicts, as stat returns for a file, each
          with deleted too (True for a file that was deleted, which has
          only key and modified_at)
        - cursor: The cursor to pass next time
        - reset: True when the cursor could not be followed (the server's
          index was rebuilt, or the cursor came from another server): changes
          is empty, and you start again without a cursor

    Raises:
        Exception if the bucket does not exist or is not visible to you, or
        the cursor is not valid
    """
    changes = []
    while True:
        path = "/api/files/changes/" + _enc(bucket) + "?limit=1000"
        if prefix:
            path += "&prefix=" + _enc(prefix)
        if cursor:
            path += "&cursor=" + _enc(cursor)
        response = api.get(path)
        if response.get("reset"):
            return {"changes": [], "cursor": "", "reset": True}
        for info in response.get("changes") or []:
            f = _file(info)
            f["deleted"] = bool(info.get("deleted"))
            changes.append(f)
        cursor = response.get("cursor") or cursor
        if not response.get("more"):
            break
    return {"changes": changes, "cursor": cursor, "reset": False}


def read_file(bucket, key):
    """Read a file's content.

    Args:
        bucket: Bucket name
        key: The file's key, e.g. "app/settings.toml"

    Returns:
        The content as bytes (at most 64 MB in a script)

    Raises:
        Exception if the file does not exist or on API error
    """
    return api.get_bytes("/api/files/objects/" + _enc(bucket) + "/" + _enc_key(key))


def read_text(bucket, key):
    """Read a file's content as UTF-8 text.

    Args:
        bucket: Bucket name
        key: The file's key

    Returns:
        The content as a string

    Raises:
        Exception if the file does not exist, is not UTF-8, or on API error
    """
    return read_file(bucket, key).decode()


def write_file(bucket, key, data, content_type=""):
    """Write a file, replacing any file with that key.

    Args:
        bucket: Bucket name
        key: The file's key; folders need no creating
        data: The content, a string (written as UTF-8) or bytes (at most 64 MB
            at a time in a script)
        content_type: Content type; text/plain for a string, else
            application/octet-stream, when not given

    Returns:
        The file dict, as stat returns for a file

    Raises:
        Exception if you may not write there, the quota is exceeded, or on API error
    """
    _check_key(key)
    if not content_type:
        if isinstance(data, str):
            content_type = "text/plain; charset=utf-8"
        else:
            content_type = "application/octet-stream"
    return _file(api.put_bytes("/api/files/objects/" + _enc(bucket) + "/" + _enc_key(key), data, content_type))


def delete_file(bucket, key, if_match=""):
    """Delete a file.

    Args:
        bucket: Bucket name
        key: The file's key
        if_match: Only delete the file if it is still this version: the etag
            from list_files, list_changes, write_file or copy_file. A file
            changed since is left alone and an exception raised (HTTP 412)

    Returns:
        True on success

    Raises:
        Exception if the file does not exist, has changed since if_match,
        you may not delete it, or on API error
    """
    headers = None
    if if_match:
        headers = {"If-Match": '"' + str(if_match).strip('"') + '"'}
    api.delete("/api/files/objects/" + _enc(bucket) + "/" + _enc_key(key), headers)
    return True


def copy_file(source_bucket, source_key, dest_bucket, dest_key):
    """Copy a file, within a bucket or to another, on the server: its content
    is not transferred. Its content type, metadata and modification time are
    kept, and any file at the destination is replaced.

    Args:
        source_bucket: Bucket holding the file (you need read access)
        source_key: The file's key
        dest_bucket: Bucket to copy to (you need write access)
        dest_key: The copy's key

    Returns:
        The copy's file dict

    Raises:
        Exception if the file does not exist, access is lacking, the owner's
        quota would be exceeded, or source and destination are the same file
    """
    _check_key(dest_key)
    return _file(api.post("/api/files/copy", {
        "source_bucket": source_bucket,
        "source_key": source_key,
        "dest_bucket": dest_bucket,
        "dest_key": dest_key,
    }))


def file_exists(bucket, key):
    """Check whether a file exists.

    Args:
        bucket: Bucket name
        key: The file's key

    Returns:
        True if there is a file with exactly this key

    Raises:
        Exception if the bucket does not exist or is not visible to you
    """
    # The key sorts before any other key that starts with it.
    response = api.get("/api/files/list/" + _enc(bucket) + "?limit=1&prefix=" + _enc(key))
    found = response.get("objects") or []
    return len(found) > 0 and found[0].get("key") == key


def stat(bucket, key):
    """Say what a path in a bucket is, without reading any content.

    Args:
        bucket: Bucket name
        key: The file's or folder's key; "" for the top of the bucket

    Returns:
        None when there is nothing there, or a dict with type "file" or
        "folder". A file's also has:
        - key, size, etag, sha256, content_type
        - modified_at: The file's modification time, as its writer gave it
        - changed_at: When this version was saved
        - mode: Its permission bits, such as 0o755, or None when none were
          kept
        - link: True for a symbolic link, whose content is its target
        - meta: Its metadata, a dict of strings
        A folder's also has key, ending in "/", and marker: True when an
        empty-folder marker keeps it

    Raises:
        Exception if the bucket does not exist or is not visible to you
    """
    _check_key(key)
    try:
        info = api.get("/api/files/stat/" + _enc(bucket) + "/" + _enc_key(key))
    except Exception as e:
        if "object not found" in str(e):
            return None
        raise
    if info.get("file"):
        f = _file(info["file"])
        f["type"] = "file"
        return f
    folder = key.rstrip("/")
    if folder:
        folder += "/"
    return {"type": "folder", "key": folder, "marker": info.get("marker") is not None}


def move(bucket, source, dest, overwrite=False):
    """Rename a file, or a folder and everything in it, within a bucket, on
    the server: no content is transferred. Every file is copied before any
    is deleted, so a failure part way leaves files in both places, never in
    neither.

    Args:
        bucket: Bucket name
        source: The file's or folder's key
        dest: The new key
        overwrite: Replace files already at the destination; otherwise a
            destination holding a file refuses the move

    Returns:
        The number of files moved

    Raises:
        Exception if there is nothing at source, the destination holds a file
        and overwrite is False, a folder would move into itself, or you may
        not write to the bucket
    """
    _check_key(source)
    _check_key(dest)
    response = api.post("/api/files/move", {
        "bucket": bucket,
        "from": source,
        "to": dest,
        "overwrite": overwrite,
    }, timeout=600)
    return response.get("moved", 0)


def snapshot(source_bucket, source, dest_bucket, dest):
    """Copy a folder, as it was at one moment, to a new folder, on the
    server: no content is transferred and the copy takes no more storage
    until it changes. Files written into the folder while the snapshot is
    made are not in it. The snapshot records itself in a manifest,
    .knot-snapshot.json, written last into the new folder, so a folder
    holding it is a complete snapshot.

    Args:
        source_bucket: Bucket holding the folder (you need read access)
        source: The folder; "" for the whole bucket
        dest_bucket: Bucket to make the snapshot in (you need write access)
        dest: The new folder, which must not exist

    Returns:
        A dict containing:
        - files: Number of files in the snapshot
        - folders: Number of empty folders kept
        - bytes: Their total size
        - taken_at: The moment the snapshot shows
        - source_bucket: The source bucket's full name
        - source: The source folder
        - manifest: The key of the snapshot's manifest

    Raises:
        Exception if the source does not exist, the destination does,
        access is lacking, or the owner's quota would be exceeded
    """
    _check_key(source)
    _check_key(dest)
    info = api.post("/api/files/snapshot", {
        "source_bucket": source_bucket,
        "source": source,
        "dest_bucket": dest_bucket,
        "dest": dest,
    }, timeout=3600)
    return {
        "files": info.get("files", 0),
        "folders": info.get("folders", 0),
        "bytes": info.get("bytes", 0),
        "taken_at": info.get("taken_at"),
        "source_bucket": info.get("source_bucket"),
        "source": info.get("source_path"),
        "manifest": info.get("manifest"),
    }


def set_attrs(bucket, key, mode=None, mtime=None):
    """Change a file's permission bits or modification time, keeping its
    content and other metadata. These are what knot file sync, NFS mounts
    and rclone give the file when they make it locally.

    Args:
        bucket: Bucket name
        key: The file's key
        mode: Permission bits, such as 0o755; a link stays a link
        mtime: Modification time, as Unix seconds (int or float) or an RFC
            3339 string

    Returns:
        The file dict, as stat returns for a file

    Raises:
        Exception if the file does not exist, neither mode nor mtime is
        given, mode is not between 0 and 0o777, or the file changed while
        it was being changed (HTTP 412: try again)
    """
    if mode is None and mtime is None:
        raise Exception("give mode, mtime or both")
    if mode is not None and (not isinstance(mode, int) or mode < 0 or mode > 0o777):
        raise Exception("mode must be permission bits, 0 to 0o777, such as 0o644")
    f = stat(bucket, key)
    if f is None or f["type"] != "file":
        raise Exception("object not found: " + key)
    meta = {}
    for k in f["meta"]:
        meta[k] = f["meta"][k]
    if mode is not None:
        typ = _S_IFLNK if f["link"] else _S_IFREG
        meta["mode"] = "%o" % (typ | mode)
    if mtime is not None:
        if isinstance(mtime, str):
            meta["mtime"] = mtime
        else:
            meta["mtime"] = "%.9f" % float(mtime)
    return _file(api.post("/api/files/copy", {
        "source_bucket": bucket,
        "source_key": key,
        "dest_bucket": bucket,
        "dest_key": key,
        "meta": meta,
        "if_match": f["etag"],
    }))



# ---------------------------------------------------------------------------
# Uploads in parts
# ---------------------------------------------------------------------------
#
# A file larger than a script can write in one go (64 MB) is sent a part at a
# time: start_upload, upload_part for each part in order from 1, then
# complete_upload. The parts are held on the server until the upload
# completes, so an upload stopped part way carries on from the parts already
# sent (list_upload_parts); abort_upload discards them.


def _upload_path(bucket, key):
    return "/api/files/uploads/" + _enc(bucket) + "/" + _enc_key(key)


def start_upload(bucket, key, content_type="application/octet-stream"):
    """Start an upload in parts of a file.

    Args:
        bucket: Bucket name
        key: The file's key
        content_type: Content type the file will have
            (default application/octet-stream)

    Returns:
        The upload ID, for upload_part, list_upload_parts, complete_upload
        and abort_upload

    Raises:
        Exception if you may not write there, or on API error
    """
    _check_key(key)
    response = api.post(_upload_path(bucket, key), None,
                        headers={"Content-Type": content_type or "application/octet-stream"})
    return response.get("upload_id")


def upload_part(bucket, key, upload_id, number, data):
    """Send one part of an upload. Sending a part number again replaces it.

    Args:
        bucket: Bucket name
        key: The file's key
        upload_id: From start_upload
        number: Part number, from 1, in file order
        data: The part's content, str (sent as UTF-8) or bytes, at most 64 MB

    Returns:
        A dict with number, etag and size of the part held

    Raises:
        Exception if the upload is not found, the quota is exceeded, or on API error
    """
    path = _upload_path(bucket, key) + "?upload_id=" + _enc(upload_id) + "&part=" + str(int(number))
    part = api.put_bytes(path, data, "application/octet-stream")
    return {"number": part.get("number", number), "etag": part.get("etag", ""), "size": part.get("size", 0)}


def list_upload_parts(bucket, key, upload_id):
    """List the parts of an upload the server holds.

    Args:
        bucket: Bucket name
        key: The file's key
        upload_id: From start_upload

    Returns:
        A list of dicts with number, etag and size, in part order

    Raises:
        Exception if the upload is not found, or on API error
    """
    response = api.get(_upload_path(bucket, key) + "?upload_id=" + _enc(upload_id))
    return [{"number": p.get("number", 0), "etag": p.get("etag", ""), "size": p.get("size", 0)}
            for p in response.get("parts") or []]


def complete_upload(bucket, key, upload_id, parts=None, timeout=3600):
    """Make the parts of an upload the file, replacing any file with that key.

    Args:
        bucket: Bucket name
        key: The file's key
        upload_id: From start_upload
        parts: The parts in order, as upload_part returned them (dicts with
            number and etag); default: every part the server holds
        timeout: Seconds to wait while the server assembles the file
            (default 3600)

    Returns:
        The file dict, as stat returns for a file

    Raises:
        Exception if the upload is not found, a part is missing or does not
        match, the quota is exceeded, or on API error
    """
    if parts is None:
        parts = list_upload_parts(bucket, key, upload_id)
    body = {"parts": [{"number": p.get("number"), "etag": p.get("etag", "")} for p in parts]}
    path = _upload_path(bucket, key) + "?upload_id=" + _enc(upload_id) + "&complete=1"
    return _file(api.post(path, body, timeout=timeout))


def abort_upload(bucket, key, upload_id):
    """Discard an upload and the parts held.

    Args:
        bucket: Bucket name
        key: The file's key
        upload_id: From start_upload

    Returns:
        True if successful

    Raises:
        Exception if the upload is not found, or on API error
    """
    api.delete(_upload_path(bucket, key) + "?upload_id=" + _enc(upload_id))
    return True
