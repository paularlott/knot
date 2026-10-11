# write-read.py - write files to a bucket and read them back.
#
# Writes text (including non-ASCII), binary and a file in nested folders,
# reads them back and reports what the bucket holds. Run it twice: the
# second run rewrites everything, so it also exercises replacement.

import os

import knot.files as files

BUCKET = os.environ.get("KNOT_FILES_BUCKET", "scriptfiles")


def bucket():
    """The script's bucket, created when it does not exist."""
    for b in files.list_buckets():
        if b["name"].endswith("--" + BUCKET) or b["display_name"] == BUCKET:
            return b["name"]
    return files.create_bucket(BUCKET)["name"]


name = bucket()
print("bucket:", name)

# Text is written as UTF-8; the write replaces any file with that key.
info = files.write_file(name, "reports/today.txt", "héllo ü 日本語\nline two\n")
print("wrote:", info["key"], info["size"], "bytes,", info["content_type"])

text = files.read_text(name, "reports/today.txt")
print("read back ok:", text == "héllo ü 日本語\nline two\n")

# Bytes go through unchanged, and keys may sit in folders.
blob = bytes([0, 1, 2, 250, 251, 255]) + b"\x00binary"
info = files.write_file(name, "data/blob.bin", blob)
print("wrote:", info["key"], info["size"], "bytes,", info["content_type"])
print("read back ok:", files.read_file(name, "data/blob.bin") == blob)

# Copy a file on the server, without transferring its content.
copy = files.copy_file(name, "reports/today.txt", name, "reports/yesterday.txt")
print("copied to:", copy["key"])

# List what the bucket holds now.
listing = files.list_files(name, prefix="reports/")
print("reports folder:", sorted(f["key"] for f in listing["files"]))

usage = files.usage()
print("usage:", usage["files"], "files,", usage["used_bytes"], "bytes")
