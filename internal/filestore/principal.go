package filestore

import (
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"strings"
)

// PrincipalFor builds the principal for a user, false for an inactive or
// deleted user. Users without a file storage permission still reach the
// buckets shared with them but cannot own buckets. On a leaf node, whose
// buckets are local to it, every user has full use of their own buckets, as
// with the leaf's spaces.
func PrincipalFor(user *model.User) (*Principal, bool) {
	if user == nil || !user.Active || user.IsDeleted {
		return nil, false
	}
	admin := user.HasPermission(model.PermissionManageFiles)
	leaf := false
	if cfg := config.GetServerConfig(); cfg != nil {
		leaf = cfg.LeafNode
	}
	return &Principal{
		UserId:      user.Id,
		Username:    user.Username,
		Groups:      user.Groups,
		CanOwn:      leaf || admin || user.HasPermission(model.PermissionUseFiles),
		CanShare:    leaf || admin || user.HasPermission(model.PermissionShareBuckets),
		CanTransfer: leaf || admin || user.HasPermission(model.PermissionTransferBuckets),
		IsAdmin:     admin,
	}, true
}

// CanUseFilesPage reports whether the Files page is any use to a user: they
// can own buckets or at least one bucket is shared with them. Users with
// neither still reach file storage through the API.
func CanUseFilesPage(s *Store, user *model.User) bool {
	if s == nil {
		return false
	}
	p, ok := PrincipalFor(user)
	if !ok {
		return false
	}
	return p.mayOwn() || s.HasAccessibleBucket(p)
}

// EffectiveLimits applies the server defaults to a user's summed user and
// group limits, returning the storage limit in bytes and the bucket limit,
// 0 meaning none.
func EffectiveLimits(q *model.Quota) (int64, int) {
	mb, buckets := int64(q.FileStorageMB), int(q.MaxBuckets)
	if cfg := config.GetServerConfig(); cfg != nil {
		if mb == 0 {
			mb = int64(cfg.FilesDefaultQuotaMB)
		}
		if buckets == 0 {
			buckets = cfg.FilesDefaultMaxBuckets
		}
	}
	return mb * 1024 * 1024, buckets
}

// userLimits returns a user's effective limits; ok is false when the user
// no longer exists.
func userLimits(userId string) (quota int64, buckets int, ok bool, err error) {
	user, err := database.GetInstance().GetUser(userId)
	if err != nil || user == nil {
		return 0, 0, false, nil
	}
	q, err := database.GetUserQuota(user)
	if err != nil {
		return 0, 0, true, err
	}
	quota, buckets = EffectiveLimits(q)
	return quota, buckets, true, nil
}

// DatabaseBucketLimit returns how many buckets a user may own.
func DatabaseBucketLimit(userId string) (int, error) {
	_, buckets, _, err := userLimits(userId)
	return buckets, err
}

// DatabaseQuota returns a user's file storage limit in bytes.
func DatabaseQuota(userId string) (int64, error) {
	quota, _, ok, err := userLimits(userId)
	if !ok {
		// The owner is gone; nothing more may be added to their buckets
		// until an administrator transfers them.
		return 1, nil
	}
	return quota, err
}

// OwnerState is what the user database says about the owner of a bucket.
type OwnerState int

const (
	OwnerActive  OwnerState = iota
	OwnerDeleted            // the user was deleted
	OwnerMissing            // there is no such user, which may only mean it has not replicated here yet
	OwnerUnknown            // the database could not say
)

// OwnerFunc looks up the owner of a bucket.
type OwnerFunc func(userId string) OwnerState

// DatabaseOwnerState looks an owner up in the user database.
func DatabaseOwnerState(userId string) OwnerState {
	user, err := database.GetInstance().GetUser(userId)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return OwnerMissing
		}
		return OwnerUnknown
	}
	if user == nil {
		return OwnerMissing
	}
	if user.IsDeleted {
		return OwnerDeleted
	}
	return OwnerActive
}
