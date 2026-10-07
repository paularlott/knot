package filestore

import (
	"context"
	"encoding/hex"
	"io"
	"sort"
	"strings"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/gossip/hlc"
)

// BucketInfo is a bucket with its usage.
type BucketInfo struct {
	Bucket
	Size   int64
	Count  int
	Access int // effective access, owner for a file administrator
	// Granted and Via are the access the principal holds in their own right
	// and how (ViaOwner, ViaUser, ViaGroup, ViaAll), ignoring a file
	// administrator's blanket access; Via is empty when that is all they have.
	Granted int
	Via     string
}

// ---------------------------------------------------------------------------
// Buckets
// ---------------------------------------------------------------------------

// CreateBucket creates a bucket owned by the principal.
func (s *Store) CreateBucket(p *Principal, name string) (*Bucket, error) {
	if !p.mayOwn() {
		return nil, ErrCannotOwn
	}
	if !ValidBucketName(name) {
		return nil, ErrInvalidName
	}
	limit, err := s.bucketLimitFor(p.UserId)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	if s.byName[name] != "" {
		s.mu.Unlock()
		return nil, ErrBucketExists
	}
	if limit > 0 && len(s.owned[p.UserId]) >= limit {
		s.mu.Unlock()
		return nil, ErrBucketLimit
	}
	b := &Bucket{
		Id:        NewBucketId(),
		Name:      name,
		OwnerId:   p.UserId,
		Grants:    []Grant{},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: hlc.Now(),
	}
	if _, _, err := s.applyBucketLocked(b); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.mu.Unlock()

	s.commit([]*Bucket{b}, nil)
	return b.clone(), nil
}

// DeleteBucket deletes a bucket; with force its objects go with it,
// otherwise it must be empty.
func (s *Store) DeleteBucket(p *Principal, name string, force bool) error {
	s.mu.Lock()
	b, err := s.bucketForLocked(p, name, AccessOwner)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if !force {
		if st := s.statsIfAny(b.Id); st != nil {
			if _, count := st.usage(); count > 0 {
				s.mu.Unlock()
				return ErrBucketNotEmpty
			}
		}
	}
	nb := b.clone()
	nb.IsDeleted = true
	nb.UpdatedAt = hlc.Now()
	if _, _, err := s.applyBucketLocked(nb); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()

	s.commit([]*Bucket{nb}, nil)
	return nil
}

// GetBucket returns a bucket the principal can see.
func (s *Store) GetBucket(p *Principal, name string) (*BucketInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, err := s.bucketForLocked(p, name, AccessRead)
	if err != nil {
		return nil, err
	}
	return s.infoLocked(b, p), nil
}

func (s *Store) infoLocked(b *Bucket, p *Principal) *BucketInfo {
	info := &BucketInfo{Bucket: *b.clone(), Access: b.AccessFor(p)}
	info.Granted, info.Via, _ = b.AccessVia(p)
	if st := s.statsIfAny(b.Id); st != nil {
		info.Size, info.Count = st.usage()
	}
	return info
}

// Describe returns a bucket as p would see it without checking p may see
// it, for reporting the result of an action p was allowed to take, such as
// a transfer that leaves them without access.
func (s *Store) Describe(p *Principal, name string) (*BucketInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b := s.bucketByNameLocked(name)
	if !live(b) {
		return nil, ErrNoSuchBucket
	}
	return s.infoLocked(b, p), nil
}

// ListBuckets lists the buckets the principal owns or that are shared with
// them. With all set, a file administrator sees every bucket.
func (s *Store) ListBuckets(p *Principal, all bool) []*BucketInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*BucketInfo
	for _, b := range s.buckets {
		if !live(b) {
			continue
		}
		info := s.infoLocked(b, p)
		if info.Granted == AccessNone && !(all && p.IsAdmin) {
			continue
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Store) updateBucket(p *Principal, name string, need int, fn func(b *Bucket) error) (*Bucket, error) {
	s.mu.Lock()
	b, err := s.bucketForLocked(p, name, need)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	nb := b.clone()
	if err := fn(nb); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	nb.UpdatedAt = hlc.Now()
	if _, _, err := s.applyBucketLocked(nb); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.mu.Unlock()

	s.commit([]*Bucket{nb}, nil)
	return nb.clone(), nil
}

// SetGrant shares a bucket, replacing any existing grant to the same principal.
func (s *Store) SetGrant(p *Principal, name string, g Grant) (*Bucket, error) {
	if g.Access != GrantRead && g.Access != GrantWrite {
		return nil, ErrInvalidGrant
	}
	if g.Type == GrantAll {
		g.Id = ""
	}
	return s.updateBucket(p, name, AccessOwner, func(b *Bucket) error {
		// Checked after access so others' buckets stay invisible.
		if !p.mayShare() {
			return ErrCannotShare
		}
		// Update an existing grant in place so the list keeps its order.
		for i, eg := range b.Grants {
			if eg.Type == g.Type && eg.Id == g.Id {
				b.Grants[i] = g
				return nil
			}
		}
		if len(b.Grants) >= MaxGrants {
			return ErrTooManyGrants
		}
		b.Grants = append(b.Grants, g)
		return nil
	})
}

// RemoveGrant stops sharing a bucket with a principal.
func (s *Store) RemoveGrant(p *Principal, name, grantType, id string) (*Bucket, error) {
	if grantType == GrantAll {
		id = ""
	}
	return s.updateBucket(p, name, AccessOwner, func(b *Bucket) error {
		// Checked after access so others' buckets stay invisible.
		if !p.mayShare() {
			return ErrCannotShare
		}
		b.Grants = withoutGrant(b.Grants, grantType, id)
		return nil
	})
}

// TransferBucket gives a bucket to a new owner. The owner may transfer their
// own bucket with the transfer permission; a file administrator may transfer
// any. The bucket moves into the new owner's namespace, so its name changes
// from "<old>--<short>" to "<new>--<short>"; its id, and so its files, stay
// where they are. With checkLimits set the new owner must be under their
// bucket limit and have room for the content.
func (s *Store) TransferBucket(p *Principal, name, newOwnerId, newOwnerUsername string, checkLimits bool) (*Bucket, error) {
	newName, err := FullName(newOwnerUsername, ShortName(name))
	if err != nil {
		return nil, err
	}
	var quota int64
	var maxBuckets int
	if checkLimits {
		if quota, err = s.quotaLimit(newOwnerId); err != nil {
			return nil, err
		}
		if maxBuckets, err = s.bucketLimitFor(newOwnerId); err != nil {
			return nil, err
		}
	}

	s.mu.Lock()
	b, err := s.bucketForLocked(p, name, AccessOwner)
	if err == nil && !p.mayTransfer() {
		err = ErrCannotTransfer
	}
	if err == nil && b.OwnerId == newOwnerId {
		s.mu.Unlock()
		return b.clone(), nil
	}
	if err == nil && s.byName[newName] != "" {
		err = ErrTransferNameTaken
	}
	if err == nil && maxBuckets > 0 && len(s.owned[newOwnerId]) >= maxBuckets {
		err = ErrBucketLimit
	}
	if err == nil {
		var size int64
		if st := s.statsIfAny(b.Id); st != nil {
			size, _ = st.usage()
		}
		err = s.roomLocked(newOwnerId, quota, size)
	}
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}

	// The new owner holds full access, so a grant to them is redundant.
	nb := b.clone()
	nb.Name = newName
	nb.OwnerId = newOwnerId
	nb.UpdatedAt = hlc.Now()
	nb.Grants = withoutGrant(nb.Grants, GrantUser, newOwnerId)
	if _, _, err := s.applyBucketLocked(nb); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.mu.Unlock()

	s.commit([]*Bucket{nb}, nil)
	return nb.clone(), nil
}

// BucketRename is a bucket whose name changed with its owner's username.
type BucketRename struct {
	Id      string
	OldName string
	NewName string
}

// RenameOwnerBuckets renames the buckets a user owns to follow a change of
// their username, "<new username>--<name>". A name already taken gets a
// suffix. It returns what it renamed, for the caller to log.
func (s *Store) RenameOwnerBuckets(userId, username string) ([]BucketRename, error) {
	s.mu.Lock()
	ids := make([]string, 0, len(s.owned[userId]))
	for id := range s.owned[userId] {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var out []BucketRename
	var changed []*Bucket
	var firstErr error
	for _, id := range ids {
		b := s.buckets[id]
		want, err := FullName(username, ShortName(b.Name))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if want == b.Name {
			continue
		}
		if holder := s.byName[want]; holder != "" && holder != id {
			want = s.freeNameLocked(want)
		}
		nb := b.clone()
		nb.Name = want
		nb.UpdatedAt = hlc.Now()
		stored, _, err := s.applyBucketLocked(nb)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if stored != nil {
			out = append(out, BucketRename{Id: id, OldName: b.Name, NewName: stored.Name})
			changed = append(changed, stored)
		}
	}
	s.mu.Unlock()

	s.commit(changed, nil)
	return out, firstErr
}

// DeleteUser removes a deleted user from file storage: the buckets they own
// are deleted with their content and their grants on other buckets dropped.
// It returns the number of buckets deleted.
func (s *Store) DeleteUser(userId string) int {
	var changed []*Bucket
	deleted := 0

	s.mu.Lock()
	for _, b := range s.buckets {
		if !live(b) {
			continue
		}
		nb := b.clone()
		if b.OwnerId == userId {
			nb.IsDeleted = true
			deleted++
		} else {
			nb.Grants = withoutGrant(nb.Grants, GrantUser, userId)
			if len(nb.Grants) == len(b.Grants) {
				continue
			}
		}
		nb.UpdatedAt = hlc.Now()
		if _, _, err := s.applyBucketLocked(nb); err != nil {
			s.fail("failed to update bucket", err)
			continue
		}
		changed = append(changed, nb)
	}
	s.mu.Unlock()

	s.commit(changed, nil)
	return deleted
}

// BucketAccess is one bucket a principal reaches and how.
type BucketAccess struct {
	Name    string
	OwnerId string
	Access  int
	Via     string // ViaOwner, ViaUser, ViaGroup or ViaAll
	GroupId string // the group, for ViaGroup
	Size    int64
	Count   int
}

// AccessibleBuckets lists the buckets p reaches as owner or through grants,
// by name. A file administrator's blanket access to every bucket is not
// listed; callers report that separately.
func (s *Store) AccessibleBuckets(p *Principal) []BucketAccess {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []BucketAccess
	for _, b := range s.buckets {
		if !live(b) {
			continue
		}
		level, via, gid := b.AccessVia(p)
		if level == AccessNone {
			continue
		}
		ba := BucketAccess{Name: b.Name, OwnerId: b.OwnerId, Access: level, Via: via, GroupId: gid}
		if st := s.statsIfAny(b.Id); st != nil {
			ba.Size, ba.Count = st.usage()
		}
		out = append(out, ba)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// HasAccessibleBucket reports whether p holds access to any bucket in their
// own right: as its owner or through a share.
func (s *Store) HasAccessibleBucket(p *Principal) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.owned[p.UserId]) > 0 && p.CanOwn {
		return true
	}
	for _, b := range s.buckets {
		if live(b) {
			if level, _, _ := b.AccessVia(p); level != AccessNone {
				return true
			}
		}
	}
	return false
}

// commit gossips local changes, then removes content they
// left unreferenced.
func (s *Store) commit(buckets []*Bucket, objects []*Object) {
	s.broadcast(buckets, objects)
	s.flushUnlinks()
}

// ---------------------------------------------------------------------------
// Objects
// ---------------------------------------------------------------------------

// PutOptions describes an object being written.
type PutOptions struct {
	ContentType string
	Meta        map[string]string
	ModifiedBy  string
	ModifiedAt  time.Time
	Size        int64  // expected size, -1 when unknown
	SHA256      string // expected sha256 hex, optional
	MD5         []byte // expected md5, optional
	IfMatch     string // only replace the object if its ETag is this
	IfNoneMatch bool   // only write if the object does not exist
}

// HeadObject returns an object's record.
func (s *Store) HeadObject(p *Principal, bucket, key string) (*Object, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, err := s.bucketForLocked(p, bucket, AccessRead)
	if err != nil {
		return nil, err
	}
	o := s.liveObjectLocked(b.Id, key)
	if o == nil {
		return nil, ErrNoSuchKey
	}
	return o.clone(), nil
}

// OpenObject returns an object's record and its content, fetching the
// content from another server first if this one does not hold it yet.
func (s *Store) OpenObject(ctx context.Context, p *Principal, bucket, key string) (*Object, Content, error) {
	o, err := s.HeadObject(p, bucket, key)
	if err != nil {
		return nil, nil, err
	}
	f, err := s.openBlob(ctx, o)
	if err != nil {
		return nil, nil, err
	}
	return o, f, nil
}

func (s *Store) openBlob(ctx context.Context, o *Object) (Content, error) {
	if f, err := s.openContent(o.SHA256); err == nil {
		return f, nil
	}
	if err := s.fetcher.wait(ctx, o.SHA256, o.Size, o.SourceNode); err != nil {
		return nil, err
	}
	f, err := s.openContent(o.SHA256)
	if err != nil {
		return nil, ErrUnavailable
	}
	return f, nil
}

// PutObject stores an object from r.
func (s *Store) PutObject(p *Principal, bucket, key string, r io.Reader, opts PutOptions) (*Object, error) {
	if !ValidKey(key) {
		return nil, ErrInvalidKey
	}
	if err := checkMeta(opts.ContentType, opts.Meta); err != nil {
		return nil, err
	}
	owner, limit, err := s.quotaFor(p, bucket, AccessWrite)
	if err != nil {
		return nil, err
	}

	// Fail early when the declared size cannot fit, and stop a body of
	// unknown size once it passes what the owner has left.
	s.mu.RLock()
	var existing int64
	if b := s.bucketByNameLocked(bucket); b != nil {
		existing = s.existingSizeLocked(b.Id, key)
	}
	remaining := s.remainingLocked(owner, limit, existing)
	s.mu.RUnlock()
	if remaining >= 0 && opts.Size > remaining {
		return nil, ErrQuotaExceeded
	}

	st, err := s.blobs.stage(r, opts.Size, remaining, opts.SHA256, opts.MD5)
	if err != nil {
		return nil, err
	}
	return s.commitContent(p, bucket, key, st, hex.EncodeToString(st.md5), limit, opts)
}

func (s *Store) existingSizeLocked(bucketId, key string) int64 {
	if o := s.liveObjectLocked(bucketId, key); o != nil {
		return o.Size
	}
	return 0
}

// commitContent stores staged content and records the object. limit is the
// bucket owner's storage limit, looked up by the caller.
func (s *Store) commitContent(p *Principal, bucket, key string, st *staged, etag string, limit int64, opts PutOptions) (*Object, error) {
	sha := st.sha
	kind := contentInline
	var data []byte
	if st.size <= inlineMax {
		var err error
		if data, err = st.bytes(); err != nil {
			return nil, err
		}
	} else {
		kind = contentFile
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	b, err := s.bucketForLocked(p, bucket, AccessWrite)
	var release func()
	if err == nil {
		release, err = s.reserveLocked(b.OwnerId, limit, st.size-s.existingSizeLocked(b.Id, key))
	}
	if err != nil {
		st.discard()
		return nil, err
	}
	defer release()

	// A content file is installed and referenced under the lock that stops
	// content whose last reference went from being removed in between.
	if kind == contentFile {
		s.blobMu.RLock()
		defer s.blobMu.RUnlock()
		if err := s.blobs.install(st.tw.Path(), sha); err != nil {
			st.discard()
			return nil, err
		}
	}

	o := s.newObjectLocked(b, key, sha, etag, st.size, opts)
	changed, err := s.applyObjects([]objectOp{{
		obj: o,
		pre: func(txn *badger.Txn, cur *Object) error {
			if !s.liveLocked(cur) {
				cur = nil
			}
			return checkPreconditions(cur, opts)
		},
		content: kind,
		data:    data,
	}})
	if err != nil {
		return nil, err
	}
	if len(changed) == 0 {
		// A newer record, from a write on another server at the same moment,
		// is already held: this write happened and was replaced at once, which
		// is what last writer wins means. Nothing is stored or gossiped.
		return o.clone(), nil
	}

	s.commit(nil, []*Object{o})
	return o.clone(), nil
}

// checkPreconditions applies conditional-write options against the current
// object, nil when there is none.
func checkPreconditions(cur *Object, opts PutOptions) error {
	if opts.IfNoneMatch && cur != nil {
		return ErrPrecondition
	}
	if opts.IfMatch != "" && (cur == nil || strings.Trim(opts.IfMatch, `"`) != cur.ETag) {
		return ErrPrecondition
	}
	return nil
}

func (s *Store) newObjectLocked(b *Bucket, key, sha, etag string, size int64, opts PutOptions) *Object {
	modified := opts.ModifiedAt
	if modified.IsZero() {
		modified = time.Now()
	}
	contentType := opts.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return &Object{
		BucketId:    b.Id,
		Key:         key,
		Size:        size,
		SHA256:      sha,
		ETag:        etag,
		ContentType: contentType,
		Meta:        normalizeMeta(opts.Meta),
		ModifiedAt:  modified.UTC(),
		ModifiedBy:  opts.ModifiedBy,
		SourceNode:  s.nodeId,
		UpdatedAt:   hlc.Now(),
	}
}

// CopyObject copies an object. Content is shared, so only metadata is
// written. With opts nil the source metadata is kept.
func (s *Store) CopyObject(p *Principal, srcBucket, srcKey, dstBucket, dstKey string, opts *PutOptions) (*Object, error) {
	if !ValidKey(dstKey) {
		return nil, ErrInvalidKey
	}
	if opts != nil {
		if err := checkMeta(opts.ContentType, opts.Meta); err != nil {
			return nil, err
		}
	}
	_, limit, err := s.quotaFor(p, dstBucket, AccessWrite)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	sb, err := s.bucketForLocked(p, srcBucket, AccessRead)
	if err != nil {
		return nil, err
	}
	src := s.liveObjectLocked(sb.Id, srcKey)
	if src == nil {
		return nil, ErrNoSuchKey
	}
	b, err := s.bucketForLocked(p, dstBucket, AccessWrite)
	var release func()
	if err == nil {
		release, err = s.reserveLocked(b.OwnerId, limit, src.Size-s.existingSizeLocked(b.Id, dstKey))
	}
	if err != nil {
		return nil, err
	}
	defer release()

	o := src.clone()
	o.BucketId = b.Id
	o.Key = dstKey
	o.UpdatedAt = hlc.Now()
	if !s.isMissing(src.SHA256) {
		o.SourceNode = s.nodeId
	}
	if opts != nil {
		o.Meta = normalizeMeta(opts.Meta)
		if opts.ContentType != "" {
			o.ContentType = opts.ContentType
		}
		o.ModifiedBy = opts.ModifiedBy
		o.ModifiedAt = time.Now().UTC()
	}
	changed, err := s.applyObjects([]objectOp{{
		obj: o,
		// The source must still be the file that was read, or its content
		// may have been removed with it.
		pre: func(txn *badger.Txn, cur *Object) error {
			now, err := getObject(txn, sb.Id, srcKey)
			if err != nil {
				return err
			}
			if now == nil || now.IsDeleted || now.SHA256 != src.SHA256 {
				return ErrNoSuchKey
			}
			return nil
		},
	}})
	if err != nil {
		return nil, err
	}
	if len(changed) == 0 {
		// Replaced at once by a newer write elsewhere: last writer wins.
		return o.clone(), nil
	}

	s.commit(nil, []*Object{o})
	return o.clone(), nil
}

// DeleteObject deletes an object.
func (s *Store) DeleteObject(p *Principal, bucket, key string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, err := s.bucketForLocked(p, bucket, AccessWrite)
	if err != nil {
		return err
	}
	o := &Object{
		BucketId:   b.Id,
		Key:        key,
		ModifiedAt: time.Now().UTC(),
		SourceNode: s.nodeId,
		UpdatedAt:  hlc.Now(),
		IsDeleted:  true,
	}
	changed, err := s.applyObjects([]objectOp{{
		obj: o,
		pre: func(txn *badger.Txn, cur *Object) error {
			if !s.liveLocked(cur) {
				return ErrNoSuchKey
			}
			return nil
		},
	}})
	if err != nil {
		return err
	}
	if len(changed) == 0 {
		// A newer write elsewhere wins over this delete.
		return nil
	}

	s.commit(nil, []*Object{o})
	return nil
}

// ListResult is one page of a listing.
type ListResult struct {
	Objects     []*Object
	Prefixes    []string
	IsTruncated bool
	Next        string // pass as after to continue
}

// prefixEnd returns the smallest string greater than every string starting
// with prefix, "" when there is none.
func prefixEnd(prefix string) string {
	for i := len(prefix) - 1; i >= 0; i-- {
		if prefix[i] < 0xff {
			return prefix[:i] + string(prefix[i]+1)
		}
	}
	return ""
}

// ListObjects lists a bucket in key order. With a delimiter, keys sharing a
// prefix up to the delimiter are rolled up into Prefixes. Keys are stored in
// order, so a folder is a scan of the keys with its prefix, and each
// rolled-up prefix is skipped in one seek.
func (s *Store) ListObjects(p *Principal, bucket, prefix, delimiter, after string, max int) (*ListResult, error) {
	if max <= 0 || max > 1000 {
		max = 1000
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	b, err := s.bucketForLocked(p, bucket, AccessRead)
	if err != nil {
		return nil, err
	}
	res := &ListResult{}

	// Start after the continuation point; a rolled-up prefix is skipped whole.
	start := prefix
	if after > start {
		start = after
		if delimiter != "" && strings.HasSuffix(after, delimiter) {
			if end := prefixEnd(after); end != "" {
				start = end
			}
		}
	}

	base := objectPrefix(b.Id)
	at := func(k string) []byte { return append(append([]byte(nil), base...), k...) }
	err = s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{PrefetchValues: true, PrefetchSize: 100, Prefix: at(prefix)})
		defer it.Close()
		it.Seek(at(start))
		for it.Valid() {
			item := it.Item()
			k := string(item.Key()[len(base):])
			if k == after {
				it.Next()
				continue
			}
			var o *Object
			if err := item.Value(func(v []byte) (err error) { o, err = decodeObject(v); return }); err != nil {
				return err
			}
			if !s.liveLocked(o) {
				it.Next()
				continue
			}

			if delimiter != "" {
				if d := strings.Index(k[len(prefix):], delimiter); d >= 0 {
					entry := k[:len(prefix)+d+len(delimiter)]
					if len(res.Objects)+len(res.Prefixes) == max {
						res.IsTruncated = true
						return nil
					}
					res.Prefixes = append(res.Prefixes, entry)
					res.Next = entry
					end := prefixEnd(entry)
					if end == "" {
						return nil
					}
					it.Seek(at(end))
					continue
				}
			}

			if len(res.Objects)+len(res.Prefixes) == max {
				res.IsTruncated = true
				return nil
			}
			res.Objects = append(res.Objects, o)
			res.Next = k
			it.Next()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !res.IsTruncated {
		res.Next = ""
	}
	return res, nil
}
