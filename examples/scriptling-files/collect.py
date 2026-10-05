# collect.py - gather a folder of files from a bucket into one report.
#
# Lists every file below a prefix and reads each one, printing a line per
# file with its size and first line of text. Shows how a script consumes
# what people and tools (knot file copy, rclone) have uploaded.

import os

import knot.files as files

BUCKET = os.environ.get("KNOT_FILES_BUCKET", "scriptfiles")
PREFIX = os.environ.get("KNOT_FILES_PREFIX", "reports/")

name = None
for b in files.list_buckets():
    if b["name"].endswith("--" + BUCKET) or b["display_name"] == BUCKET:
        name = b["name"]
        break
if name is None:
    raise SystemExit("bucket not found, run write-read.py first")

listing = files.list_files(name, prefix=PREFIX, recursive=True)
if not listing["files"]:
    raise SystemExit("no files below " + PREFIX)

for f in sorted(listing["files"], key=lambda f: f["key"]):
    content = files.read_file(name, f["key"])
    try:
        first = content.split(b"\n")[0].decode()
    except Exception:
        first = "<binary>"
    print("%6d  %-30s  %s" % (f["size"], f["key"], first))
