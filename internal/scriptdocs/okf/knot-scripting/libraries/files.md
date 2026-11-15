---
type: "API Reference"
title: "knot.files"
description: "File storage from scripts — read, write, list, copy, move and delete files in buckets, snapshot folders, manage the buckets, and run sync projects in a space."
resource: "https://getknot.dev/reference/libraries/files/"
tags:
  - "files"
  - "storage"
  - "api"
status: stable
generated:
  by: knot/scripts/generate-scripting-docs.mjs
sources:
  - resource: "https://getknot.dev/reference/libraries/files/"
---
# knot.files

The `knot.files` library works with [file storage](https://getknot.dev/docs/file-storage/): buckets of files replicated across the cluster. Scripts can read and write files, list and copy them, and create, share and delete buckets, as the user the script runs as, with the same permissions, sharing rules and quotas as the web interface and the `knot file` commands. In a space, [`knot.files.sync`](#knotfilessync) runs [sync projects](https://getknot.dev/docs/file-storage/using-files/#sync-projects).

Buckets are named `<owner>--<name>`. Give your own buckets by their short name; buckets shared with you need the full name. File keys use `/` to organise files into folders, and may contain spaces and any other characters — the library encodes them.

---

## Functions

| Function | Description |
|----------|-------------|
| `list_files(bucket, prefix="", recursive=False)` | List the files in a bucket |
| `list_changes(bucket, prefix="", cursor="")` | List what changed in a bucket since a cursor |
| `read_file(bucket, key)` | Read a file's content as bytes |
| `read_text(bucket, key)` | Read a file's content as UTF-8 text |
| `write_file(bucket, key, data, content_type="")` | Write a file, replacing any file with that key |
| `delete_file(bucket, key, if_match="")` | Delete a file, optionally only if unchanged |
| `copy_file(source_bucket, source_key, dest_bucket, dest_key)` | Copy a file on the server, within or between buckets |
| `move(bucket, source, dest, overwrite=False)` | Rename a file or folder on the server |
| `snapshot(source_bucket, source, dest_bucket, dest)` | Copy a folder as it was at one moment |
| `stat(bucket, key)` | Say what a path is: a file, a folder or nothing |
| `set_attrs(bucket, key, mode=None, mtime=None)` | Change a file's permission bits or modification time |
| `file_exists(bucket, key)` | Check whether a file exists |
| `list_buckets(all=False)` | List the buckets you own or that are shared with you |
| `get_bucket(name)` | Get one bucket |
| `create_bucket(name)` | Create a bucket you own |
| `delete_bucket(name, force=False)` | Delete a bucket |
| `share_bucket(name, user=None, group=None, everyone=False, access="read")` | Share a bucket |
| `unshare_bucket(name, user=None, group=None, everyone=False)` | Stop sharing a bucket |
| `transfer_bucket(name, user, force=False)` | Give a bucket to another user |
| `usage()` | Your file storage usage and limits |
| `share_targets()` | The users and groups your buckets can be shared with or transferred to |
| `fsck(repair=False, deep=False, timeout=3600)` | Check file storage, and with `repair` fix what can be fixed (administrators) |
| `start_upload(bucket, key, content_type="application/octet-stream")` | Start an upload in parts; returns its ID |
| `upload_part(bucket, key, upload_id, number, data)` | Send one part of an upload |
| `list_upload_parts(bucket, key, upload_id)` | The parts of an upload the server holds |
| `complete_upload(bucket, key, upload_id, parts=None, timeout=3600)` | Make the parts the file |
| `abort_upload(bucket, key, upload_id)` | Discard an upload and its parts |

---

### list_files(bucket, prefix="", recursive=False)

List the files in a bucket.

**Parameters:**
- `bucket` (string): Bucket name
- `prefix` (string, optional): Only keys starting with this. End it with `/` for a folder.
- `recursive` (bool, optional): If `False`, files below the next `/` after the prefix are rolled up into folders; if `True`, every file below the prefix is listed

**Returns:** `dict` containing:
- `files` (list): [File dicts](#stat)
- `folders` (list): Folder prefixes, each ending in `/` (empty when `recursive`)

Every page of a large bucket is fetched.

---

### list_changes(bucket, prefix="", cursor="")

List what changed in a bucket since a cursor, to follow a bucket without listing it again.

**Parameters:**
- `bucket` (string): Bucket name
- `prefix` (string, optional): Only keys starting with this. End it with `/` for a folder.
- `cursor` (string, optional): The cursor from the last call. Without one every file is returned; `"now"` returns no files, only a cursor to follow the bucket from now on.

**Returns:** `dict` containing:
- `changes` (list): [File dicts](#stat), each with `deleted` too. Each file changed since the cursor appears once, as it is now; a deleted file has `deleted` set and only its `key` and `modified_at`.
- `cursor` (string): Pass it next time
- `reset` (bool): `True` when the cursor couldn't be followed — the server's index was rebuilt, or the cursor came from another server. `changes` is then empty and `cursor` is `""`: start again without a cursor.

```python
import knot.files as files

state = files.list_changes("configs", cursor="now")
# ... later ...
step = files.list_changes("configs", cursor=state["cursor"])
for f in step["changes"]:
    print(("deleted " if f["deleted"] else "changed ") + f["key"])
state = step
```

---

### read_file(bucket, key)

**Returns:** `bytes` - The file's content. Raises if the file does not exist.

### read_text(bucket, key)

**Returns:** `string` - The file's content decoded as UTF-8. Raises if the file does not exist or is not UTF-8.

---

### write_file(bucket, key, data, content_type="")

Write a file, replacing any file with that key. Folders need no creating.

**Parameters:**
- `bucket` (string): Bucket name
- `key` (string): The file's key, e.g. `app/settings.toml`
- `data` (string or bytes): The content; a string is written as UTF-8
- `content_type` (string, optional): Defaults to `text/plain; charset=utf-8` for a string and `application/octet-stream` for bytes

**Returns:** `dict` - The [file dict](#stat)

Raises if you may not write to the bucket, the owner's quota would be exceeded, or the key is invalid (empty, or with empty, `.` or `..` segments, or not valid UTF-8).

---

### delete_file(bucket, key, if_match="")

Delete a file.

**Parameters:**
- `bucket` (string): Bucket name
- `key` (string): The file's key
- `if_match` (string, optional): Only delete the file if it is still this version — the `etag` from `list_files`, `list_changes`, `write_file` or `copy_file`. A file changed since is left alone and an exception raised (HTTP 412), so a script never deletes a change it didn't see.

**Returns:** `bool` - `True`. Raises if the file does not exist, has changed since `if_match`, or you may not delete it.

```python
info = files.write_file("work", "job.lock", "mine")
# ... later, release the lock only if nobody replaced it meanwhile
files.delete_file("work", "job.lock", if_match=info["etag"])
```

---

### copy_file(source_bucket, source_key, dest_bucket, dest_key)

Copy a file within a bucket or to another, on the server: its content is not transferred, so the size makes no difference. Its content type, metadata and modification time are kept, and any file at the destination is replaced. You need read access to the source and write access to the destination, and the destination bucket's owner is charged for the new file.

**Returns:** `dict` - The copy's file dict. Raises if the file does not exist, access is lacking, the quota would be exceeded, or source and destination are the same file.

---

### move(bucket, source, dest, overwrite=False)

Rename a file, or a folder and everything in it, within a bucket, on the server: no content is transferred. Every file is copied before any is deleted, so a failure part way leaves files in both places, never in neither.

**Parameters:**
- `bucket` (string): Bucket name
- `source` (string): The file's or folder's key
- `dest` (string): The new key
- `overwrite` (bool, optional): Replace files already at the destination; otherwise a destination holding a file refuses the move

**Returns:** `int` - The number of files moved. Raises if there's nothing at `source`, the destination holds a file, a folder would move into itself, or you may not write to the bucket.

---

### snapshot(source_bucket, source, dest_bucket, dest)

Copy a folder, as it was at one moment, to a new folder, on the server, as [`knot file snapshot`](https://getknot.dev/docs/file-storage/using-files/#working-with-files) does. No content is transferred, and the copy takes no more storage until it changes. Files written into the folder while the snapshot is made aren't in it. The snapshot ends with a manifest, `.knot-snapshot.json`, written last, so a folder holding it is a complete snapshot.

**Parameters:**
- `source_bucket` (string): Bucket holding the folder (read access)
- `source` (string): The folder; `""` for the whole bucket
- `dest_bucket` (string): Bucket to make the snapshot in (write access)
- `dest` (string): The new folder, which must not exist

**Returns:** `dict` containing `files`, `folders` (empty folders kept) and `bytes`, `taken_at` (the moment it shows), `source_bucket` (its full name), `source` and `manifest` (the manifest's key). Raises if the source doesn't exist, the destination does, access is lacking, or the owner's quota would be exceeded.

```python
import knot.files as files

# A dated copy of the build output, before it is replaced
snap = files.snapshot("builds", "latest", "builds", "archive/2026-11-06")
print(snap["files"], "files kept")
```

---

### stat(bucket, key)

Say what a path is, without reading any content.

**Parameters:**
- `bucket` (string): Bucket name
- `key` (string): The file's or folder's key; `""` for the top of the bucket

**Returns:** `None` when there's nothing there, or a `dict` with `type` `"file"` or `"folder"`. A file's is the file dict the other functions return too:
- `key`, `size`, `etag`, `sha256`, `content_type`
- `modified_at` (string): The modification time its writer gave it
- `changed_at` (string): When this version was saved
- `mode` (int or None): Permission bits, such as `0o755`, or `None` when none were kept
- `link` (bool): `True` for a symbolic link, whose content is its target
- `meta` (dict): Its metadata, as strings

A folder's has `key`, ending in `/`, and `marker`: `True` when an empty-folder marker keeps it. Raises if the bucket doesn't exist or isn't visible to you.

---

### set_attrs(bucket, key, mode=None, mtime=None)

Change a file's permission bits or modification time, keeping its content and other metadata. These are what `knot file sync`, NFS mounts and rclone give the file when they make it locally.

**Parameters:**
- `bucket` (string): Bucket name
- `key` (string): The file's key
- `mode` (int, optional): Permission bits, `0` to `0o777`, such as `0o755`; a link stays a link
- `mtime` (number or string, optional): Unix seconds, or an RFC 3339 time

**Returns:** `dict` - The [file dict](#stat). Raises if the file doesn't exist, neither `mode` nor `mtime` is given, or the file changed while it was being changed (HTTP 412: try again).

```python
files.write_file("tools", "bin/deploy.sh", script)
files.set_attrs("tools", "bin/deploy.sh", mode=0o755)
```

---

### file_exists(bucket, key)

**Returns:** `bool` - `True` if there is a file with exactly this key. A folder name is not a file.

---

### list_buckets(all=False)

List the buckets you own or that are shared with you; `all=True` lists every bucket (file storage managers only).

**Returns:** `list` of dicts, each containing:
- `name` (string): The full name, `<owner>--<name>`
- `display_name` (string): The short name for your own buckets, else the full name
- `owner`, `owner_id` (string): The owner's username and id
- `access` (string): Your access: `owner`, `write` or `read`
- `size` (int), `count` (int): Bytes and number of files
- `shared` (list): Dicts with `type` (`user`, `group` or `all`), `name` and `access` (`read` or `write`); filled in only for the owner and file storage managers
- `created_at` (string): Creation timestamp

### get_bucket(name)

**Returns:** `dict` - The bucket, as in `list_buckets`. Raises if it does not exist or is not visible to you.

### create_bucket(name)

Create a bucket named `<your username>--<name>`.

**Parameters:**
- `name` (string): The short name: 3–30 lowercase letters, digits and hyphens, with no `--`

**Returns:** `dict` - The bucket. Raises if you may not own buckets, are at your bucket limit, or the name is invalid or taken.

### delete_bucket(name, force=False)

Delete a bucket. A bucket that holds files is refused unless `force=True`, which deletes the files with it.

**Returns:** `bool` - `True`

---

### share_bucket(name, user=None, group=None, everyone=False, access="read")

Share a bucket with exactly one of a user (username or email), a group or everyone. `access` is `"read"` or `"write"`. Sharing again with the same user or group changes their access. Needs the Share Buckets permission.

**Returns:** `dict` - The bucket, with its `shared` list updated.

### unshare_bucket(name, user=None, group=None, everyone=False)

Stop sharing a bucket with exactly one of a user, a group or everyone.

**Returns:** `dict` - The bucket.

### transfer_bucket(name, user, force=False)

Give a bucket to another user, who must be allowed to own buckets. It is renamed into their namespace and their quota is charged; the move is refused when it would take them over their limits, unless a file storage manager gives `force=True`. Needs the Transfer Buckets permission.

**Returns:** `dict` - The bucket, with its new name.

### usage()

**Returns:** `dict` containing `used_bytes`, `files` and `buckets` (what you own), and `quota_bytes` and `max_buckets` (your limits, `0` for none).

### share_targets()

The users and groups your buckets can be shared with or transferred to — the choices of the web UI's share and transfer dialogs, without the user management permission listing users needs. Needs the Share Buckets or Transfer Buckets permission.

**Returns:** `dict` containing `users` (dicts with `id`, `username`, `can_own` — may own buckets, so may receive a transfer — and `self`, `True` for you) and `groups` (dicts with `id` and `name`).

### fsck(repair=False, deep=False, timeout=3600)

Check file storage, as `knot admin file fsck` does. Only file storage administrators may run it.

**Parameters:**
- `repair` (bool, optional): Fix what can be fixed: fetch missing content from the cluster, rebuild counts and indexes, remove buckets of deleted users, rewrite modes in octal
- `deep` (bool, optional): Also read every stored content and verify its checksum (slow)
- `timeout` (number, optional): Seconds to wait for the check (default 3600)

**Returns:** `dict` containing `buckets` and `files` (how many were checked), `found` and `repaired` (dicts of issue kind to count), `issues` (dicts with `kind`, `bucket`, `key` and `detail`; capped) and `notes` (strings).

---

## Uploads in parts

A file larger than one write (64 MB) is sent a part at a time: `start_upload`, then `upload_part` for each part in order from 1, then `complete_upload`. The parts are held on the server until the upload completes, so an upload stopped part way carries on from the parts already sent — `list_upload_parts` says which — and `abort_upload` discards them.

### start_upload(bucket, key, content_type="application/octet-stream")

**Returns:** `string` - The upload ID. Raises if you may not write there.

### upload_part(bucket, key, upload_id, number, data)

Send part `number` (from 1, in file order) of an upload; `data` is a string (sent as UTF-8) or bytes, at most 64 MB. Sending a number again replaces that part.

**Returns:** `dict` with `number`, `etag` and `size` of the part held.

### list_upload_parts(bucket, key, upload_id)

**Returns:** `list` of dicts with `number`, `etag` and `size`, in part order. Raises if the upload is not found.

### complete_upload(bucket, key, upload_id, parts=None, timeout=3600)

Make the parts the file, replacing any file with that key. `parts` lists the parts in order as `upload_part` returned them; by default every part the server holds is used.

**Returns:** `dict` - The [file](#stat).

### abort_upload(bucket, key, upload_id)

Discard an upload and the parts held.

**Returns:** `bool` - `True`

```python
import knot.files as files

upload_id = files.start_upload("backups", "db/dump.sql.gz", "application/gzip")
with open("/tmp/dump.sql.gz", "rb") as f:
    number = 1
    while True:
        chunk = f.read(32 << 20)
        if not chunk:
            break
        files.upload_part("backups", "db/dump.sql.gz", upload_id, number, chunk)
        number += 1
files.complete_upload("backups", "db/dump.sql.gz", upload_id)
```

---

## knot.files.sync

`knot.files.sync` runs [sync projects](https://getknot.dev/docs/file-storage/using-files/#sync-projects) in the space's agent, as `knot file sync-project start` does, so a script can keep folders in step with file storage. It works in scripts that run in a space: startup and shutdown scripts, `knot space run-script` and `knot run-script`. Elsewhere every function raises. A startup script that runs as the space starts waits for the agent's sync projects to be ready.

Projects started this way are kept by the agent and started again when the space restarts. Starting a project that's already running unchanged leaves it running, so a startup script can start it on every boot.

| Function | Description |
|----------|-------------|
| `start(project, name="", wait=False, timeout=600)` | Start a sync project, or replace one of the same name; `name`, `wait` and `timeout` may be positional or keywords |
| `stop(name)` | Stop a sync project after sending its last local changes (`name` positional or `name=`) |
| `list()` | The sync projects running, the template's included |
| `status(name)` | How one sync project is doing, or `None` (`name` positional or `name=`) |

### start(project, name="", wait=False, timeout=600)

**Parameters:**
- `project` (dict or string): A dict like a project file, or a project file's TOML. The dict has `name`, optionally `exclude` and `no_default_excludes`, and `sync`, a list of jobs with the keys a project file's `[[sync]]` tables take: `local`, `remote` (`bucket:folder/`), and optionally `name`, `snapshot`, `mode`, `watch`, `delete`, `exclude`, `exclude_from`, `no_default_excludes` and `allow_mass_delete`.
- `name` (string, optional): The name of a project that gives none
- `wait` (bool, optional): Wait for every job's first pass, raising if a job fails
- `timeout` (number, optional): How many seconds to wait, 600 by default. The project carries on after a timeout.

**Returns:** `dict` - The project's status, as `status` gives it. Raises if the project isn't valid, a job's folder overlaps another project's, or the name is `template`.

### stop(name)

Stop a sync project, sending its last local changes first, and forget it. The template's project can't be stopped.

**Returns:** `bool` - `True`. Raises if no project of that name runs.

### list()

**Returns:** `list` of dicts, each as `status` gives it.

### status(name)

**Returns:** `None` when no project of that name runs, else a `dict` containing `name`, `template` (`True` for the template's) and `jobs`, a list of dicts with `name`, `local`, `remote`, `mode`, `state` (`starting`, `snapshotting`, `syncing`, `watching`, `done`, `failed` or `stopped`), `passes`, `last_pass`, `error`, `uploaded`, `downloaded`, `deleted` and `conflicts`.

```python
import knot.files.sync as sync

# In a startup script: bring the project's code down, then keep it in step
sync.start({
    "name": "code",
    "sync": [
        {"local": "~/src", "remote": "code:projects/api/", "snapshot": "code:templates/go/", "exclude": ["node_modules/"]},
        {"local": "~/repo", "remote": "code:repos/api/", "no_default_excludes": True},
    ],
}, wait=True)

for job in sync.status("code")["jobs"]:
    print(job["name"], job["state"], job["uploaded"], "up")
```

---

## Examples

```python
import knot.files as files

# Keep a small config in a bucket
files.create_bucket("configs")
files.write_file("configs", "app/settings.toml", "debug = true\n")
print(files.read_text("configs", "app/settings.toml"))

# Binary content is bytes
files.write_file("configs", "icons/logo.png", open_logo_bytes())
data = files.read_file("configs", "icons/logo.png")

# Walk a bucket
listing = files.list_files("configs", recursive=True)
for f in listing["files"]:
    print(f["key"], f["size"])

# Copy between buckets on the server, then share the result
files.copy_file("configs", "app/settings.toml", "backups", "2026-10-05/settings.toml")
files.share_bucket("backups", group="ops", access="read")

# Only write what is missing
if not files.file_exists("configs", "app/defaults.toml"):
    files.write_file("configs", "app/defaults.toml", "retries = 3\n")
```
