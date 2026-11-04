package command_files

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/paularlott/knot/apiclient"
)

// A large file is uploaded in parts, each kept on the server until the file
// is complete, so an upload stopped part way (a lost connection, the knot
// command interrupted) carries on from the parts already sent when it is
// run again, rather than sending the whole file again. What was sent is
// remembered in an upload record in the sync folder (syncDir), by server,
// bucket and file, with the local file's size, time and stamp: a file
// changed since is sent afresh. A part held by the server is used only when
// its checksum matches that part of the local file. The server keeps the
// parts of an upload not completed for a week; the record of one is swept
// after as long (sweepSyncDir).

var (
	// partSize is the size of each part sent.
	partSize int64 = 64 << 20
	// partedAt is the size from which a file is sent in parts.
	partedAt int64 = 256 << 20
)

// afterPart, when set, is called after each part is sent: tests stop an
// upload part way with it.
var afterPart func(n int)

const (
	uploadRecordVersion = 1
	// uploadStaleAfter is how long the server keeps the parts of an upload
	// not completed, and so how long its record is worth keeping.
	uploadStaleAfter = 7 * 24 * time.Hour
)

type uploadRecord struct {
	Version  int    `json:"version" msgpack:"version"`
	Server   string `json:"server" msgpack:"server"`
	Bucket   string `json:"bucket" msgpack:"bucket"`
	Key      string `json:"key" msgpack:"key"`
	UploadId string `json:"upload_id" msgpack:"upload_id"`
	PartSize int64  `json:"part_size" msgpack:"part_size"`
	// The local file when the upload started.
	Size  int64  `json:"size" msgpack:"size"`
	Mtime int64  `json:"mtime" msgpack:"mtime"`
	Ino   uint64 `json:"ino,omitempty" msgpack:"ino,omitempty"`
	Ctime int64  `json:"ctime,omitempty" msgpack:"ctime,omitempty"`
}

func uploadRecordPath(server, bucket, key string) (string, error) {
	sd, err := syncDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(server + "\x00" + bucket + "\x00" + key))
	return filepath.Join(sd, "upload-"+hex.EncodeToString(sum[:12])+".json"), nil
}

// uploadInParts sends the local file at path, open as f, to bucket/key in
// parts, carrying on an upload of it stopped part way. ifMatch and ifAbsent
// are the conditions of the write, as PutFileObjectIfMatch and
// PutFileObjectIfAbsent take them. It returns how many bytes of the file
// were already on the server.
func uploadInParts(ctx context.Context, client *apiclient.ApiClient, f *os.File, info os.FileInfo, bucket, key, ctype string, attrs apiclient.FileAttrs, ifMatch string, ifAbsent bool) (*apiclient.FileObjectInfo, int64, error) {
	server := client.GetBaseURL()
	stamp := stampOf(info)
	want := uploadRecord{
		Version: uploadRecordVersion, Server: server, Bucket: bucket, Key: key, PartSize: partSize,
		Size: info.Size(), Mtime: info.ModTime().UnixNano(), Ino: stamp.ino, Ctime: stamp.ctime,
	}
	recPath, err := uploadRecordPath(server, bucket, key)
	if err != nil {
		return nil, 0, err
	}

	// Carry on the upload of this file as it is, if one was stopped.
	held := map[int]apiclient.FileUploadPart{}
	var rec uploadRecord
	if data, err := os.ReadFile(recPath); err == nil && json.Unmarshal(data, &rec) == nil {
		same := rec.UploadId != ""
		resumed := rec
		resumed.UploadId = ""
		same = same && resumed == want
		if same {
			parts, err := client.ListFileUploadParts(ctx, bucket, key, rec.UploadId)
			if err == nil {
				for _, p := range parts {
					held[p.Number] = p
				}
				want.UploadId = rec.UploadId
			}
		}
		if want.UploadId == "" {
			// Of another version of the file, or forgotten by the server.
			client.AbortFileUpload(ctx, rec.Bucket, rec.Key, rec.UploadId)
		}
	}
	if want.UploadId == "" {
		id, err := client.StartFileUpload(ctx, bucket, key, ctype, attrs, nil)
		if err != nil {
			return nil, 0, err
		}
		want.UploadId = id
		if data, err := json.Marshal(&want); err == nil {
			writeAtomic(recPath, data)
		}
	}

	size := info.Size()
	count := int((size + partSize - 1) / partSize)
	if count == 0 {
		count = 1
	}
	parts := make([]apiclient.FileUploadPart, 0, count)
	var reused int64
	for n := 1; n <= count; n++ {
		if err := ctx.Err(); err != nil {
			return nil, reused, err
		}
		off := int64(n-1) * partSize
		length := min(partSize, size-off)
		md5Hex, shaHex, err := sectionSums(f, off, length)
		if err != nil {
			return nil, reused, err
		}
		if p, ok := held[n]; ok && p.Size == length && p.ETag == md5Hex {
			parts = append(parts, p)
			reused += length
			continue
		}
		p, err := client.UploadFilePart(ctx, bucket, key, want.UploadId, n, io.NewSectionReader(f, off, length), length, shaHex)
		if err != nil {
			if apiclient.IsNotFound(err) {
				os.Remove(recPath) // the server forgot the upload: start again next time
			}
			return nil, reused, err
		}
		parts = append(parts, *p)
		if afterPart != nil {
			afterPart(n)
		}
	}

	out, err := client.CompleteFileUpload(ctx, bucket, key, want.UploadId, parts, ifMatch, ifAbsent)
	switch {
	case err == nil:
		os.Remove(recPath)
	case apiclient.IsPreconditionFailed(err):
		// The file there changed: this upload will not be completed.
		client.AbortFileUpload(ctx, bucket, key, want.UploadId)
		os.Remove(recPath)
	case apiclient.IsNotFound(err):
		os.Remove(recPath)
	}
	return out, reused, err
}

// sectionSums returns the MD5 and SHA-256, in hex, of length bytes of f
// from off.
func sectionSums(f *os.File, off, length int64) (string, string, error) {
	m, s := md5.New(), sha256.New()
	n, err := io.Copy(io.MultiWriter(m, s), io.NewSectionReader(f, off, length))
	if err != nil {
		return "", "", err
	}
	if n != length {
		return "", "", errors.New("the file changed while it was read")
	}
	return hex.EncodeToString(m.Sum(nil)), hex.EncodeToString(s.Sum(nil)), nil
}
