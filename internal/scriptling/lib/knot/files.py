# knot.files - File storage library for the Knot server
#
# Buckets of files replicated across the cluster: list, read, write, copy and
# delete files, and manage the buckets, their sharing and your usage.
# Requires knot.apiclient to be configured first.
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


def _file(info):
    return {
        "key": info.get("key"),
        "size": info.get("size", 0),
        "etag": info.get("etag"),
        "sha256": info.get("sha256"),
        "content_type": info.get("content_type"),
        "modified_at": info.get("modified_at"),
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
        - files: A list of file dicts, each with key, size, etag, sha256,
          content_type and modified_at
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
        - changes: A list of file dicts, each with key, size, etag, sha256,
          content_type, modified_at and deleted (True for a file that was
          deleted, which has only key and modified_at)
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
        The file dict, with key, size, etag, sha256, content_type and modified_at

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
