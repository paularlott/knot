// Package filestore implements knot's replicated file storage: buckets owned
// by a user, shareable with users and groups, holding objects whose content
// is replicated to every server in the cluster.
//
// Metadata (bucket and object records) lives in memory, backed by a journal
// in the storage directory, and is replicated by gossip with last-writer-wins
// on HLC timestamps. Object content is stored content-addressed (sha256) and
// pulled directly from a peer by any server that does not yet hold it.
package filestore

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/paularlott/gossip/hlc"
)

// Access levels for a bucket.
const (
	AccessNone  = 0
	AccessRead  = 1
	AccessWrite = 2
	AccessOwner = 3 // owner or file administrator: may also manage the bucket
)

// Grant principal types.
const (
	GrantUser  = "user"
	GrantGroup = "group"
	GrantAll   = "all"
)

// Grant access strings.
const (
	GrantRead  = "read"
	GrantWrite = "write"
)

var (
	ErrDisabled           = errors.New("file storage is not enabled on this server")
	ErrNoSuchBucket       = errors.New("bucket not found")
	ErrNoSuchKey          = errors.New("object not found")
	ErrBucketExists       = errors.New("bucket already exists")
	ErrBucketNotEmpty     = errors.New("bucket is not empty")
	ErrAccessDenied       = errors.New("access denied")
	ErrCannotOwn          = errors.New("no permission to create buckets, only to use buckets shared with you")
	ErrQuotaExceeded      = errors.New("file storage quota exceeded")
	ErrBucketLimit        = errors.New("bucket limit reached")
	ErrCannotShare        = errors.New("no permission to share buckets")
	ErrCannotTransfer     = errors.New("no permission to transfer buckets")
	ErrTransferNameTaken  = errors.New("the new owner already has a bucket with this name")
	ErrRecipientCannotOwn = errors.New("the new owner does not have permission to use file storage")
	ErrInvalidName        = errors.New("invalid bucket name")
	ErrInvalidGrant       = errors.New("invalid share: access must be read or write")
	ErrTooManyGrants      = errors.New("a bucket can be shared with at most 256 users and groups")
	ErrMetadataTooLarge   = errors.New("object metadata is too large")
	ErrInvalidMetadata    = errors.New("object metadata must be valid UTF-8")
	ErrInvalidKey         = errors.New("invalid object key")
	ErrContentMismatch    = errors.New("content does not match the supplied checksum")
	ErrStoreInUse         = errors.New("file storage directory is in use by another process")
	ErrUnavailable        = errors.New("object content is not available on any reachable server")
	ErrPrecondition       = errors.New("the file has changed since it was read")
)

// Limits that keep every record small, so a page of records always fits in
// one gossip message.
const (
	MaxGrants          = 256  // grants per bucket
	MaxMetaBytes       = 2048 // user metadata per object, keys and values, as S3
	MaxContentTypeSize = 256
)

// Grant shares a bucket with a user, a group or all users.
type Grant struct {
	Type   string `json:"type" msgpack:"type"`
	Id     string `json:"id,omitempty" msgpack:"id"`
	Access string `json:"access" msgpack:"access"`
}

// Bucket is the replicated bucket record. Name is the cluster-wide identity.
// Generation changes each time a bucket of the same name is created, so the
// objects of a deleted bucket can never reappear in its successor.
type Bucket struct {
	Name       string        `json:"name" msgpack:"name"`
	OwnerId    string        `json:"owner_id" msgpack:"owner_id"`
	Grants     []Grant       `json:"grants" msgpack:"grants"`
	Generation hlc.Timestamp `json:"generation" msgpack:"generation"`
	CreatedAt  time.Time     `json:"created_at" msgpack:"created_at"`
	UpdatedAt  hlc.Timestamp `json:"updated_at" msgpack:"updated_at"`
	IsDeleted  bool          `json:"is_deleted" msgpack:"is_deleted"`
}

// Object is the replicated object record.
type Object struct {
	Bucket      string            `json:"bucket" msgpack:"bucket"`
	Key         string            `json:"key" msgpack:"key"`
	Generation  hlc.Timestamp     `json:"generation" msgpack:"generation"`
	Size        int64             `json:"size" msgpack:"size"`
	SHA256      string            `json:"sha256" msgpack:"sha256"`
	ETag        string            `json:"etag" msgpack:"etag"`
	ContentType string            `json:"content_type,omitempty" msgpack:"content_type"`
	Meta        map[string]string `json:"meta,omitempty" msgpack:"meta"`
	ModifiedAt  time.Time         `json:"modified_at" msgpack:"modified_at"`
	ModifiedBy  string            `json:"modified_by,omitempty" msgpack:"modified_by"`
	SourceNode  string            `json:"source_node,omitempty" msgpack:"source_node"`
	UpdatedAt   hlc.Timestamp     `json:"updated_at" msgpack:"updated_at"`
	IsDeleted   bool              `json:"is_deleted" msgpack:"is_deleted"`
}

// Principal is the identity a request acts as. Every user can reach the
// buckets shared with them; owning buckets (creating them, and owner rights
// over those they own) takes the use-files permission.
type Principal struct {
	UserId      string
	Username    string // namespaces the buckets they create, see names.go
	Groups      []string
	CanOwn      bool // holds the use-files permission
	CanShare    bool // holds the share-buckets permission
	CanTransfer bool // holds the transfer-buckets permission
	IsAdmin     bool // holds the manage-files permission
}

// A file administrator may do everything an owner may.

func (p *Principal) mayOwn() bool      { return p.CanOwn || p.IsAdmin }
func (p *Principal) mayShare() bool    { return p.CanShare || p.IsAdmin }
func (p *Principal) mayTransfer() bool { return p.CanTransfer || p.IsAdmin }

var bucketNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

// ValidBucketName reports whether name is a valid full bucket name: an S3
// compatible name that holds at most one "--", the separator between the
// owner's username and the short name (see names.go).
func ValidBucketName(name string) bool {
	if !bucketNameRe.MatchString(name) || strings.Count(name, "--") > 1 {
		return false
	}
	for _, bad := range []string{"..", "---", ".-", "-."} {
		if strings.Contains(name, bad) {
			return false
		}
	}
	return true
}

// checkMeta checks an object's content type and user metadata are within
// the record limits and valid UTF-8, which the JSON journal needs.
func checkMeta(contentType string, meta map[string]string) error {
	if len(contentType) > MaxContentTypeSize {
		return ErrMetadataTooLarge
	}
	if !utf8.ValidString(contentType) {
		return ErrInvalidMetadata
	}
	n := 0
	for k, v := range meta {
		if !utf8.ValidString(k) || !utf8.ValidString(v) {
			return ErrInvalidMetadata
		}
		n += len(k) + len(v)
	}
	if n > MaxMetaBytes {
		return ErrMetadataTooLarge
	}
	return nil
}

// withoutGrant returns grants less any to the given principal.
func withoutGrant(grants []Grant, grantType, id string) []Grant {
	out := grants[:0:0]
	for _, g := range grants {
		if g.Type != grantType || g.Id != id {
			out = append(out, g)
		}
	}
	return out
}

// ValidKey reports whether key is a usable object key.
func ValidKey(key string) bool {
	// Records are journalled as JSON, which would replace invalid UTF-8,
	// changing the key on restart and parting this server from the others.
	if key == "" || len(key) > 1024 || strings.ContainsRune(key, 0) || !utf8.ValidString(key) {
		return false
	}
	for _, part := range strings.Split(key, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	return true
}

// AccessFor returns the access level p has on bucket b: a file
// administrator has owner access to every bucket.
func (b *Bucket) AccessFor(p *Principal) int {
	if p != nil && p.IsAdmin {
		return AccessOwner
	}
	level, _, _ := b.AccessVia(p)
	return level
}

// Ways a principal reaches a bucket, reported by AccessVia.
const (
	ViaOwner = "owner"
	ViaUser  = "user"
	ViaGroup = "group"
	ViaAll   = "all"
)

// viaRank orders equally strong routes: a direct grant is preferred.
var viaRank = map[string]int{ViaUser: 3, ViaGroup: 2, ViaAll: 1}

// AccessVia returns p's access to b, ignoring the file administrator's
// blanket access, and how it is granted: as owner, by a grant to the user,
// to one of their groups (with the group id) or to all users. The strongest
// route wins; between equal ones a direct grant is preferred.
func (b *Bucket) AccessVia(p *Principal) (level int, via string, groupId string) {
	if p == nil {
		return AccessNone, "", ""
	}
	if p.CanOwn && b.OwnerId == p.UserId {
		return AccessOwner, ViaOwner, ""
	}
	for _, g := range b.Grants {
		l := AccessRead
		if g.Access == GrantWrite {
			l = AccessWrite
		}
		v, gid := "", ""
		switch g.Type {
		case GrantUser:
			if g.Id == p.UserId {
				v = ViaUser
			}
		case GrantGroup:
			for _, id := range p.Groups {
				if id == g.Id {
					v, gid = ViaGroup, g.Id
					break
				}
			}
		case GrantAll:
			v = ViaAll
		}
		if v == "" {
			continue
		}
		if l > level || (l == level && viaRank[v] > viaRank[via]) {
			level, via, groupId = l, v, gid
		}
	}
	return level, via, groupId
}

func (b *Bucket) clone() *Bucket {
	c := *b
	c.Grants = append([]Grant(nil), b.Grants...)
	return &c
}

func (o *Object) clone() *Object {
	c := *o
	if o.Meta != nil {
		c.Meta = make(map[string]string, len(o.Meta))
		for k, v := range o.Meta {
			c.Meta[k] = v
		}
	}
	return &c
}
