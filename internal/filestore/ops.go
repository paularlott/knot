package filestore

import (
	"context"
	"encoding/hex"
	"io"
	"os"
	"sort"
	"strings"
	"time"

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
	if live(s.buckets[name]) {
		s.mu.Unlock()
		return nil, ErrBucketExists
	}
	if limit > 0 && len(s.owned[p.UserId]) >= limit {
		s.mu.Unlock()
		return nil, ErrBucketLimit
	}
	now := hlc.Now()
	b := &Bucket{
		Name:       name,
		OwnerId:    p.UserId,
		Grants:     []Grant{},
		Generation: now,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  now,
	}
	s.applyBucketLocked(b)
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
		if st := s.stats[name]; st != nil && st.count > 0 {
			s.mu.Unlock()
			return ErrBucketNotEmpty
		}
	}
	nb := b.clone()
	nb.IsDeleted = true
	nb.UpdatedAt = hlc.Now()
	s.applyBucketLocked(nb)
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
	if st := s.stats[b.Name]; st != nil {
		info.Size = st.size
		info.Count = st.count
	}
	return info
}

// Describe returns a bucket as p would see it without checking p may see
// it, for reporting the result of an action p was allowed to take, such as
// a transfer that leaves them without access.
func (s *Store) Describe(p *Principal, name string) (*BucketInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b := s.buckets[name]
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
	s.applyBucketLocked(nb)
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
// from "<old>--<short>" to "<new>--<short>"; content is shared, so only the
// records move. With checkLimits set the new owner must be under their
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
	if err == nil && live(s.buckets[newName]) {
		err = ErrTransferNameTaken
	}
	if err == nil && maxBuckets > 0 && len(s.owned[newOwnerId]) >= maxBuckets {
		err = ErrBucketLimit
	}
	if err == nil {
		var size int64
		if st := s.stats[name]; st != nil {
			size = st.size
		}
		err = s.roomLocked(newOwnerId, quota, size)
	}
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}

	// Create the new bucket and its records before deleting the old one, so
	// shared content always holds a reference. The new owner holds full
	// access, so a grant to them is redundant.
	now := hlc.Now()
	nb := b.clone()
	nb.Name = newName
	nb.OwnerId = newOwnerId
	nb.UpdatedAt = now
	nb.Generation = now
	nb.Grants = withoutGrant(nb.Grants, GrantUser, newOwnerId)
	s.applyBucketLocked(nb)

	var moved []*Object
	for _, o := range s.objects[name] {
		if !s.liveLocked(o) {
			continue
		}
		no := o.clone()
		no.Bucket = newName
		no.Generation = nb.Generation
		no.UpdatedAt = hlc.Now()
		s.applyObjectLocked(no)
		moved = append(moved, no)
	}
	old := b.clone()
	old.IsDeleted = true
	old.UpdatedAt = hlc.Now()
	s.applyBucketLocked(old)
	s.mu.Unlock()

	s.commit([]*Bucket{nb, old}, moved)
	return nb.clone(), nil
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
		s.applyBucketLocked(nb)
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
		if st := s.stats[b.Name]; st != nil {
			ba.Size, ba.Count = st.size, st.count
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

// commit journals and gossips local changes, then removes content they
// left unreferenced.
func (s *Store) commit(buckets []*Bucket, objects []*Object) {
	s.journalAppend(buckets, objects, true)
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

	if _, err := s.bucketForLocked(p, bucket, AccessRead); err != nil {
		return nil, err
	}
	o := s.liveObjectLocked(bucket, key)
	if o == nil {
		return nil, ErrNoSuchKey
	}
	return o.clone(), nil
}

// OpenObject returns an object's record and its content, fetching the
// content from another server first if this one does not hold it yet.
func (s *Store) OpenObject(ctx context.Context, p *Principal, bucket, key string) (*Object, *os.File, error) {
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

func (s *Store) openBlob(ctx context.Context, o *Object) (*os.File, error) {
	if f, err := s.blobs.open(o.SHA256); err == nil {
		return f, nil
	}
	if err := s.fetcher.wait(ctx, o.SHA256, o.Size, o.SourceNode); err != nil {
		return nil, err
	}
	f, err := s.blobs.open(o.SHA256)
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
	remaining := s.remainingLocked(owner, limit, s.existingSizeLocked(bucket, key))
	s.mu.RUnlock()
	if remaining >= 0 && opts.Size > remaining {
		return nil, ErrQuotaExceeded
	}

	tw, err := s.blobs.writeTemp(r, opts.Size, remaining, opts.SHA256, opts.MD5)
	if err != nil {
		return nil, err
	}
	return s.commitContent(p, bucket, key, tw, hex.EncodeToString(tw.MD5()), limit, opts)
}

func (s *Store) existingSizeLocked(bucket, key string) int64 {
	if o := s.liveObjectLocked(bucket, key); o != nil {
		return o.Size
	}
	return 0
}

// commitContent installs a written temporary file and records the object.
// limit is the bucket owner's storage limit, looked up by the caller.
func (s *Store) commitContent(p *Principal, bucket, key string, tw *tempWriter, etag string, limit int64, opts PutOptions) (*Object, error) {
	sha := tw.SHA256()
	s.mu.Lock()
	b, err := s.bucketForLocked(p, bucket, AccessWrite)
	if err == nil {
		err = checkPreconditions(s.liveObjectLocked(bucket, key), opts)
	}
	if err == nil {
		err = s.roomLocked(b.OwnerId, limit, tw.size-s.existingSizeLocked(bucket, key))
	}
	// Installing under the lock keeps the reference count and the file in
	// step with any concurrent delete of the same content.
	if err == nil {
		err = s.blobs.install(tw.Path(), sha)
	}
	if err != nil {
		s.mu.Unlock()
		tw.discard()
		return nil, err
	}
	s.contentHeldLocked(sha)

	o := s.newObjectLocked(b, key, sha, etag, tw.size, opts)
	s.applyObjectLocked(o)
	s.mu.Unlock()

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
		Bucket:      b.Name,
		Key:         key,
		Generation:  b.Generation,
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

	s.mu.Lock()
	if _, err := s.bucketForLocked(p, srcBucket, AccessRead); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	src := s.liveObjectLocked(srcBucket, srcKey)
	if src == nil {
		s.mu.Unlock()
		return nil, ErrNoSuchKey
	}
	b, err := s.bucketForLocked(p, dstBucket, AccessWrite)
	if err == nil {
		err = s.roomLocked(b.OwnerId, limit, src.Size-s.existingSizeLocked(dstBucket, dstKey))
	}
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}

	o := src.clone()
	o.Bucket = b.Name
	o.Key = dstKey
	o.Generation = b.Generation
	o.UpdatedAt = hlc.Now()
	if _, missing := s.missing[src.SHA256]; !missing {
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
	s.applyObjectLocked(o)
	s.mu.Unlock()

	s.commit(nil, []*Object{o})
	return o.clone(), nil
}

// DeleteObject deletes an object.
func (s *Store) DeleteObject(p *Principal, bucket, key string) error {
	s.mu.Lock()
	b, err := s.bucketForLocked(p, bucket, AccessWrite)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if s.liveObjectLocked(bucket, key) == nil {
		s.mu.Unlock()
		return ErrNoSuchKey
	}
	o := &Object{
		Bucket:     bucket,
		Key:        key,
		Generation: b.Generation,
		ModifiedAt: time.Now().UTC(),
		SourceNode: s.nodeId,
		UpdatedAt:  hlc.Now(),
		IsDeleted:  true,
	}
	s.applyObjectLocked(o)
	s.mu.Unlock()

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
// prefix up to the delimiter are rolled up into Prefixes. It seeks through
// the bucket's sorted keys, skipping each rolled-up prefix in one step.
func (s *Store) ListObjects(p *Principal, bucket, prefix, delimiter, after string, max int) (*ListResult, error) {
	if max <= 0 || max > 1000 {
		max = 1000
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, err := s.bucketForLocked(p, bucket, AccessRead); err != nil {
		return nil, err
	}
	res := &ListResult{}
	st := s.stats[bucket]
	if st == nil {
		return res, nil
	}
	objs := s.objects[bucket]
	keys := st.keys

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
	i := sort.SearchStrings(keys, start)
	if i < len(keys) && keys[i] == after {
		i++
	}

	for i < len(keys) {
		k := keys[i]
		if !strings.HasPrefix(k, prefix) {
			break
		}
		o := objs[k]
		if !s.liveLocked(o) {
			i++
			continue
		}

		if delimiter != "" {
			if d := strings.Index(k[len(prefix):], delimiter); d >= 0 {
				entry := k[:len(prefix)+d+len(delimiter)]
				if len(res.Objects)+len(res.Prefixes) == max {
					res.IsTruncated = true
					break
				}
				res.Prefixes = append(res.Prefixes, entry)
				res.Next = entry
				end := prefixEnd(entry)
				if end == "" {
					break
				}
				i = sort.SearchStrings(keys, end)
				continue
			}
		}

		if len(res.Objects)+len(res.Prefixes) == max {
			res.IsTruncated = true
			break
		}
		res.Objects = append(res.Objects, o.clone())
		res.Next = k
		i++
	}
	if !res.IsTruncated {
		res.Next = ""
	}
	return res, nil
}
