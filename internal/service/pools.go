package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/health"
	"github.com/paularlott/knot/internal/methods"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/util/validate"
)

const (
	PoolSweepInterval = 15 * time.Second
	PoolReapInterval  = 1 * time.Hour
	// PoolLeaseMaxWait bounds the server-side long-poll of Acquire.
	PoolLeaseMaxWait = 300 * time.Second
)

// Lease lifecycle errors. The API layer maps these onto status codes.
var (
	ErrPoolLeasesDisabled = errors.New("pool leases are not enabled — set lease_max_time on the pool first (edit the pool, or PUT /api/pools/{pool})")
	ErrPoolNotActive      = errors.New("pool is not active")
	ErrPoolExhausted      = errors.New("no free pool member available")
	ErrLeaseNotFound      = errors.New("lease not found")
	ErrLeaseNotHolder     = errors.New("lease is held by another user")
	ErrLeaseExtendLimit   = errors.New("lease extension limit reached")
	ErrLeaseDuration      = errors.New("lease duration exceeds the pool maximum")
)

// leaseView is the in-memory routing view of an exclusive member lease. It
// lets method routing and the port proxy exclude leased members without a
// database read on the hot path; the space record is the durable truth and
// both are reconciled by the sweep and gossip.
type leaseView struct {
	userId    string
	expiresAt int64 // unix seconds, 0 = never expires
}

type PoolService struct {
	mu               sync.Mutex
	pendingDeletions map[string]time.Time  // space ID -> first seen as excess stopped
	drained          map[string]bool       // space ID -> currently drained by pool sweep
	leases           map[string]*leaseView // space ID -> held (or draining) lease
	rrCounters       map[string]int        // pool ID -> round-robin cursor
	createMu         sync.Mutex            // serializes member ordinal allocation
}

type PoolSessionState struct {
	CPUPercent        float64
	MemoryUsedBytes   uint64
	MemoryLimitBytes  uint64
	MethodRPS         float64
	HTTPRPS           float64
	TCPRPS            float64
	ActiveMethodCalls int64
}

var (
	poolService         *PoolService
	poolSessionProvider func(spaceID string) *PoolSessionState
)

func SetPoolSessionProvider(provider func(spaceID string) *PoolSessionState) {
	poolSessionProvider = provider
}

func getPoolSession(spaceID string) *PoolSessionState {
	if poolSessionProvider == nil {
		return nil
	}
	return poolSessionProvider(spaceID)
}

func GetPoolService() *PoolService {
	if poolService == nil {
		poolService = &PoolService{
			pendingDeletions: make(map[string]time.Time),
			drained:          make(map[string]bool),
			leases:           make(map[string]*leaseView),
			rrCounters:       make(map[string]int),
		}
	}
	return poolService
}

// ---------------------------------------------------------------------------
// Resolve / List / Info
// ---------------------------------------------------------------------------

func (s *PoolService) Resolve(idOrName string) (*model.PoolDefinition, error) {
	db := database.GetInstance()
	if validate.UUID(idOrName) {
		return db.GetPoolDefinition(idOrName)
	}
	return nil, fmt.Errorf("pool not found")
}

func (s *PoolService) ResolveForUser(idOrName string, user *model.User) (*model.PoolDefinition, error) {
	db := database.GetInstance()
	if validate.UUID(idOrName) {
		pool, err := db.GetPoolDefinition(idOrName)
		if err != nil || pool == nil {
			return nil, fmt.Errorf("pool not found")
		}
		if user == nil || pool.CreatedUserId != user.Id {
			return nil, fmt.Errorf("pool not found")
		}
		return pool, nil
	}
	if user == nil {
		return nil, fmt.Errorf("pool not found")
	}
	return db.GetPoolDefinitionByName(user.Id, idOrName)
}

func (s *PoolService) List(user *model.User) ([]apiclient.PoolInfo, error) {
	if user == nil {
		return []apiclient.PoolInfo{}, nil
	}
	db := database.GetInstance()
	// Scoped to the user's pools — the query already excludes deleted pools
	// and pools owned by others.
	pools, err := db.GetPoolDefinitionsByUser(user.Id)
	if err != nil {
		return nil, err
	}
	result := []apiclient.PoolInfo{}
	for _, pool := range pools {
		info, err := s.Info(pool, user)
		if err == nil {
			result = append(result, info)
		}
	}
	return result, nil
}

func (s *PoolService) Info(pool *model.PoolDefinition, user *model.User) (apiclient.PoolInfo, error) {
	// Defensive ownership guard: callers resolve pools user-scoped, but keep
	// this so the public method can't leak another user's pool.
	if pool == nil || pool.IsDeleted || user == nil || pool.CreatedUserId != user.Id {
		return apiclient.PoolInfo{}, fmt.Errorf("pool not found")
	}

	db := database.GetInstance()
	spaces, err := db.GetSpaces()
	if err != nil {
		return apiclient.PoolInfo{}, err
	}

	info := apiclient.PoolInfo{
		Id:                 pool.Id,
		Name:               pool.Name,
		TemplateId:         pool.TemplateId,
		StartupScriptId:    pool.StartupScriptId,
		DesiredCount:       pool.DesiredCount,
		Active:             pool.Active,
		LeaseMaxTime:       pool.LeaseMaxTime,
		LeaseMaxExtensions: pool.LeaseMaxExtensions,
		Members:            []apiclient.PoolMemberInfo{},
	}

	var cpuTotal, memTotal float64
	var resourceCount int
	for _, space := range spaces {
		if space.PoolId != pool.Id || space.IsDeleted {
			continue
		}
		member := s.memberInfo(space)
		info.Members = append(info.Members, member)
		if member.State == "alive" {
			info.AliveMembers++
			info.Utilization.MethodRPS += member.MethodRPS
			info.Utilization.HTTPRPS += member.HTTPRPS
			info.Utilization.TCPRPS += member.TCPRPS
			info.Utilization.MethodInflight += member.MethodInflight
			cpuTotal += member.CPUPercent
			memTotal += member.MemoryPercent
			resourceCount++
		}
	}
	info.Utilization.CombinedRPS = info.Utilization.MethodRPS + info.Utilization.HTTPRPS + info.Utilization.TCPRPS
	if resourceCount > 0 {
		info.Utilization.AvgCPUPercent = cpuTotal / float64(resourceCount)
		info.Utilization.AvgMemoryPercent = memTotal / float64(resourceCount)
	}
	return info, nil
}

func (s *PoolService) memberInfo(space *model.Space) apiclient.PoolMemberInfo {
	member := apiclient.PoolMemberInfo{
		Id:         space.Id,
		Name:       space.Name,
		State:      "dead",
		Healthy:    true,
		IsPending:  space.IsPending,
		IsDeleting: space.IsDeleting,
		IsDeployed: space.IsDeployed,
	}
	if space.LeaseId != "" {
		member.LeaseState = "draining"
		if space.LeaseActive() {
			member.LeaseState = "active"
		}
		member.LeaseExpiresAt = space.LeaseExpiresAt
		if holder, err := database.GetInstance().GetUser(space.LeaseUserId); err == nil && holder != nil {
			member.LeaseHolder = holder.Username
		}
	}
	session := getPoolSession(space.Id)
	if session != nil {
		member.MethodRPS = session.MethodRPS
		member.HTTPRPS = session.HTTPRPS
		member.TCPRPS = session.TCPRPS
		member.CombinedRPS = member.MethodRPS + member.HTTPRPS + member.TCPRPS
		member.MethodInflight = methods.DefaultRegistry().InFlightForSpace(space.Id)
		member.CPUPercent = session.CPUPercent
		if session.MemoryLimitBytes > 0 {
			member.MemoryPercent = float64(session.MemoryUsedBytes) / float64(session.MemoryLimitBytes) * 100
		}
	}
	if h := health.Get(space.Id); h != nil {
		member.Healthy = h.Healthy
	}
	if session != nil && member.Healthy {
		member.State = "alive"
	} else if space.IsPending {
		member.State = "starting"
	} else if space.IsDeleting || space.IsDeleted {
		member.State = "stopping"
	}
	return member
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

func (s *PoolService) validate(pool *model.PoolDefinition) error {
	if pool == nil {
		return fmt.Errorf("pool is required")
	}
	if !validate.Name(pool.Name) {
		return fmt.Errorf("invalid pool name")
	}
	if pool.DesiredCount < 1 {
		return fmt.Errorf("desired_count must be at least 1")
	}
	db := database.GetInstance()
	template, err := db.GetTemplate(pool.TemplateId)
	if err != nil || template == nil || template.IsDeleted || !template.Active {
		return fmt.Errorf("template not found")
	}
	// Bridged KVM spaces need an IP address chosen at creation, which pools
	// can't provide; NAT KVM templates work (no addresses to assign).
	if template.IsKvmBridged() {
		return fmt.Errorf("bridged KVM templates cannot back pools — their spaces need an IP address chosen at creation; use a NAT mode KVM template instead")
	}
	if pool.StartupScriptId != "" {
		if _, err := db.GetScript(pool.StartupScriptId); err != nil {
			return fmt.Errorf("startup script not found")
		}
	}
	if pool.LeaseMaxTime < -1 {
		return fmt.Errorf("lease_max_time must be -1 (no timeout), 0 (disabled) or a positive number of seconds")
	}
	if pool.LeaseMaxExtensions < -1 {
		return fmt.Errorf("lease_max_extensions must be -1 (unlimited), 0 (forbidden) or a positive count")
	}
	if pool.LeaseMaxExtensions != 0 && pool.LeaseMaxTime == 0 {
		return fmt.Errorf("lease_max_extensions requires leases to be enabled (lease_max_time)")
	}
	return nil
}

func (s *PoolService) Create(pool *model.PoolDefinition, user *model.User) error {
	if err := s.validate(pool); err != nil {
		return err
	}
	db := database.GetInstance()
	if _, err := db.GetPoolDefinitionByName(user.Id, pool.Name); err == nil {
		return fmt.Errorf("pool name already exists")
	}
	if _, err := db.GetSpaceByName(user.Id, pool.Name); err == nil {
		return fmt.Errorf("pool name conflicts with an existing space")
	}

	// Stamp the owning zone. Only this zone's leader manages the pool's spaces.
	pool.Zone = config.GetServerConfig().Zone

	requested := pool.DesiredCount
	if requested < 1 {
		requested = 1
	}
	pool.DesiredCount = 0

	if err := db.SavePoolDefinition(pool, nil); err != nil {
		return err
	}
	if transport := GetTransport(); transport != nil {
		transport.GossipPoolDefinition(pool)
	}

	created := 0
	for i := 0; i < requested; i++ {
		if err := s.createPoolSpace(pool, user); err != nil {
			break
		}
		created++
	}

	pool.DesiredCount = created
	pool.UpdatedUserId = user.Id
	pool.UpdatedAt = hlc.Now()
	if err := db.SavePoolDefinition(pool, nil); err != nil {
		return err
	}
	if transport := GetTransport(); transport != nil {
		transport.GossipPoolDefinition(pool)
	}

	if created < requested {
		if created == 0 {
			pool.IsDeleted = true
			pool.Name = pool.Id
			_ = db.SavePoolDefinition(pool, []string{"IsDeleted", "Name", "UpdatedAt"})
			if transport := GetTransport(); transport != nil {
				transport.GossipPoolDefinition(pool)
			}
			return fmt.Errorf("quota exceeded: no spaces could be created")
		}
		return fmt.Errorf("quota exceeded: only %d of %d spaces were created", created, requested)
	}

	return nil
}

func (s *PoolService) savePool(pool *model.PoolDefinition) error {
	if err := database.GetInstance().SavePoolDefinition(pool, nil); err != nil {
		return err
	}
	if transport := GetTransport(); transport != nil {
		transport.GossipPoolDefinition(pool)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Exclusive member leases
// ---------------------------------------------------------------------------

// SaveLeaseConfig validates and persists a change to the pool's lease
// options without touching membership.
func (s *PoolService) SaveLeaseConfig(pool *model.PoolDefinition) error {
	if err := s.validate(pool); err != nil {
		return err
	}
	return s.savePool(pool)
}

// InitLeases populates the in-memory lease view from the database. Called
// once at server start so routing exclusion survives restarts; afterwards
// the view is maintained by lease operations, their gossip, and space
// merges (SyncSpaceLease).
func (s *PoolService) InitLeases() {
	spaces, err := database.GetInstance().GetSpaces()
	if err != nil {
		return
	}
	for _, space := range spaces {
		if space.LeaseId != "" {
			s.SyncSpaceLease(space)
		}
	}
}

// SyncSpaceLease updates the in-memory lease view from a space record.
// Called at startup, on lease operations, and when a gossiped space merge
// changes a member's lease fields — that keeps servers that joined late or
// missed a lease gossip push converged with the durable truth.
func (s *PoolService) SyncSpaceLease(space *model.Space) {
	if space == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if space.LeaseId == "" {
		delete(s.leases, space.Id)
		return
	}
	var expiresAt int64
	if space.LeaseExpiresAt != nil {
		expiresAt = space.LeaseExpiresAt.Unix()
	}
	s.leases[space.Id] = &leaseView{userId: space.LeaseUserId, expiresAt: expiresAt}
}

// IsLeased reports whether the space is currently held by an exclusive
// lease — including one past its expiry that is still draining in-flight
// work: the member only returns to shared routing once the sweep reclaims
// the lease. Wired into the methods registry (SetLeaseChecker) so leased
// pool members are excluded from shared method routing.
func (s *PoolService) IsLeased(spaceID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.leases[spaceID] != nil
}

// MarkLeased sets the local lease view without gossiping. Called by the
// cluster handler on peer nodes receiving a lease gossip push.
func (s *PoolService) MarkLeased(spaceID, userId string, expiresAtUnix int64) {
	s.mu.Lock()
	s.leases[spaceID] = &leaseView{userId: userId, expiresAt: expiresAtUnix}
	s.mu.Unlock()
}

// MarkLeaseCleared removes the local lease view without gossiping. Called
// by the cluster handler when the leader reclaims an ended lease.
func (s *PoolService) MarkLeaseCleared(spaceID string) {
	s.mu.Lock()
	delete(s.leases, spaceID)
	s.mu.Unlock()
}

// resolveLeaseDuration turns a requested duration into the effective one,
// honouring the pool's configured maximum. requested: 0 = use the pool
// default, -1 = never-expiring (unlimited pools only), >0 = a concrete
// duration bounded by the pool maximum. Returns never=true for an
// unexpiring lease.
func resolveLeaseDuration(poolMax, requested int) (seconds int, never bool, err error) {
	if requested < -1 {
		return 0, false, ErrLeaseDuration
	}
	if requested == -1 {
		if poolMax != -1 {
			return 0, false, ErrLeaseDuration
		}
		return 0, true, nil
	}
	if requested == 0 {
		if poolMax == -1 {
			return 0, true, nil
		}
		return poolMax, false, nil
	}
	if poolMax > 0 && requested > poolMax {
		return 0, false, ErrLeaseDuration
	}
	return requested, false, nil
}

// Acquire grants the caller an exclusive lease on one free member of the
// pool. durationSeconds follows resolveLeaseDuration; waitSeconds
// optionally long-polls (bounded by PoolLeaseMaxWait) for a member to free
// up before returning ErrPoolExhausted.
func (s *PoolService) Acquire(ctx context.Context, pool *model.PoolDefinition, user *model.User, durationSeconds, waitSeconds int) (*apiclient.LeaseInfo, error) {
	if pool.LeaseMaxTime == 0 {
		return nil, ErrPoolLeasesDisabled
	}
	if !pool.Active {
		return nil, ErrPoolNotActive
	}
	if _, _, err := resolveLeaseDuration(pool.LeaseMaxTime, durationSeconds); err != nil {
		return nil, err
	}
	if waitSeconds < 0 {
		waitSeconds = 0
	}
	if waitSeconds > int(PoolLeaseMaxWait.Seconds()) {
		waitSeconds = int(PoolLeaseMaxWait.Seconds())
	}

	deadline := time.Now().Add(time.Duration(waitSeconds) * time.Second)
	for {
		info, err := s.tryAcquire(pool, user, durationSeconds)
		if err == nil {
			return info, nil
		}
		if !errors.Is(err, ErrPoolExhausted) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, ErrPoolExhausted
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// tryAcquire is one Acquire attempt: pick a free healthy member
// (round-robin, sharing the routing cursor), take the space's mutation
// lock, re-validate under the lock, then persist and gossip the lease.
func (s *PoolService) tryAcquire(pool *model.PoolDefinition, user *model.User, durationSeconds int) (*apiclient.LeaseInfo, error) {
	db := database.GetInstance()
	spaces, err := db.GetSpaces()
	if err != nil {
		return nil, err
	}

	var candidates []*model.Space
	for _, sp := range poolMembers(pool, spaces) {
		if sp.IsDeleting || sp.IsDeleted || sp.LeaseId != "" {
			continue
		}
		if !sp.IsDeployed || sp.IsPending {
			continue
		}
		if h := health.Get(sp.Id); h != nil && !h.Healthy {
			continue
		}
		candidates = append(candidates, sp)
	}
	if len(candidates) == 0 {
		return nil, ErrPoolExhausted
	}

	// Rotate the candidate order round-robin so repeated acquires spread
	// over the free members like routing does.
	s.mu.Lock()
	idx := s.rrCounters[pool.Id] % len(candidates)
	s.rrCounters[pool.Id] = (idx + 1) % len(candidates)
	s.mu.Unlock()
	ordered := append(candidates[idx:], candidates[:idx]...)

	for _, sp := range ordered {
		transport := GetTransport()
		token := ""
		if transport != nil {
			if token = transport.LockResource(sp.Id); token == "" {
				continue // contended; try the next member
			}
		}

		// Re-fetch under the lock and re-validate: a concurrent acquire or
		// a sweep decision may have claimed the member meanwhile.
		fresh, err := db.GetSpace(sp.Id)
		if err != nil || fresh == nil || fresh.IsDeleted || fresh.IsDeleting || fresh.LeaseId != "" {
			if transport != nil {
				transport.UnlockResource(sp.Id, token)
			}
			continue
		}

		seconds, never, err := resolveLeaseDuration(pool.LeaseMaxTime, durationSeconds)
		if err != nil {
			if transport != nil {
				transport.UnlockResource(sp.Id, token)
			}
			return nil, err
		}

		now := time.Now().UTC()
		leaseId, err := uuid.NewV7()
		if err != nil {
			if transport != nil {
				transport.UnlockResource(sp.Id, token)
			}
			return nil, err
		}
		fresh.LeaseId = leaseId.String()
		fresh.LeaseUserId = user.Id
		fresh.LeaseExtensions = 0
		if never {
			fresh.LeaseExpiresAt = nil
		} else {
			expiresAt := now.Add(time.Duration(seconds) * time.Second)
			fresh.LeaseExpiresAt = &expiresAt
		}
		fresh.UpdatedAt = hlc.Now()
		fields := []string{"LeaseId", "LeaseUserId", "LeaseExpiresAt", "LeaseExtensions", "UpdatedAt"}
		if err := db.SaveSpace(fresh, fields); err != nil {
			if transport != nil {
				transport.UnlockResource(sp.Id, token)
			}
			return nil, err
		}

		s.SyncSpaceLease(fresh)
		if transport != nil {
			transport.GossipSpace(fresh)
			transport.GossipPoolLease(fresh.Id, fresh.LeaseUserId, leaseExpiryUnix(fresh))
		}
		sse.PublishSpaceChanged(fresh.Id, fresh.UserId)

		if transport != nil {
			transport.UnlockResource(sp.Id, token)
		}
		return s.leaseInfo(pool, fresh)
	}
	return nil, ErrPoolExhausted
}

// Extend renews a held lease: the new deadline is now + duration (or never,
// on unlimited pools), bounded by the pool's extension-count cap. Allowed
// until the lease has been reclaimed — including while an expired lease is
// still draining in-flight work, which rescues a lease that ran out by
// accident.
func (s *PoolService) Extend(pool *model.PoolDefinition, user *model.User, leaseId string, durationSeconds int) (*apiclient.LeaseInfo, error) {
	if pool.LeaseMaxTime == 0 {
		// Leasing was disabled after this lease was granted: the lease
		// ends naturally (expiry or release), but it can't be renewed.
		return nil, ErrPoolLeasesDisabled
	}
	space, err := s.leaseMember(pool, leaseId)
	if err != nil {
		return nil, err
	}
	if space.LeaseUserId != user.Id {
		return nil, ErrLeaseNotHolder
	}
	if pool.LeaseMaxExtensions == 0 {
		return nil, ErrLeaseExtendLimit
	}
	if pool.LeaseMaxExtensions > 0 && space.LeaseExtensions >= pool.LeaseMaxExtensions {
		return nil, ErrLeaseExtendLimit
	}
	seconds, never, err := resolveLeaseDuration(pool.LeaseMaxTime, durationSeconds)
	if err != nil {
		return nil, err
	}

	db := database.GetInstance()
	transport := GetTransport()
	token := ""
	if transport != nil {
		if token = transport.LockResource(space.Id); token == "" {
			return nil, fmt.Errorf("member is busy with another operation, retry shortly") // lock unavailable
		}
		defer transport.UnlockResource(space.Id, token)
	}

	// Re-fetch under the lock and re-validate: the lease may have been
	// reclaimed, extended past its cap, or (in a cluster) moved on since
	// the pre-lock lookup loaded its copy.
	fresh, err := db.GetSpace(space.Id)
	if err != nil || fresh == nil || fresh.LeaseId != leaseId {
		return nil, ErrLeaseNotFound
	}
	if fresh.LeaseUserId != user.Id {
		return nil, ErrLeaseNotHolder
	}
	if pool.LeaseMaxExtensions > 0 && fresh.LeaseExtensions >= pool.LeaseMaxExtensions {
		return nil, ErrLeaseExtendLimit
	}

	now := time.Now().UTC()
	if never {
		fresh.LeaseExpiresAt = nil
	} else {
		expiresAt := now.Add(time.Duration(seconds) * time.Second)
		fresh.LeaseExpiresAt = &expiresAt
	}
	fresh.LeaseExtensions++
	fresh.UpdatedAt = hlc.Now()
	fields := []string{"LeaseExpiresAt", "LeaseExtensions", "UpdatedAt"}
	if err := db.SaveSpace(fresh, fields); err != nil {
		return nil, err
	}

	s.SyncSpaceLease(fresh)
	if transport != nil {
		transport.GossipSpace(fresh)
		transport.GossipPoolLease(fresh.Id, fresh.LeaseUserId, leaseExpiryUnix(fresh))
	}
	sse.PublishSpaceChanged(fresh.Id, fresh.UserId)
	return s.leaseInfo(pool, fresh)
}

// Release ends a held lease early. The member does not re-enter shared
// routing immediately: it stays excluded until in-flight method work has
// drained, then the sweep reclaims it (normally within one sweep).
func (s *PoolService) Release(pool *model.PoolDefinition, user *model.User, leaseId string) (*apiclient.LeaseInfo, error) {
	space, err := s.leaseMember(pool, leaseId)
	if err != nil {
		return nil, err
	}
	if space.LeaseUserId != user.Id {
		return nil, ErrLeaseNotHolder
	}

	db := database.GetInstance()
	transport := GetTransport()
	token := ""
	if transport != nil {
		if token = transport.LockResource(space.Id); token == "" {
			return nil, fmt.Errorf("member is busy with another operation, retry shortly") // lock unavailable
		}
		defer transport.UnlockResource(space.Id, token)
	}

	fresh, err := db.GetSpace(space.Id)
	if err != nil || fresh == nil || fresh.LeaseId != leaseId {
		return nil, ErrLeaseNotFound
	}
	if fresh.LeaseUserId != user.Id {
		return nil, ErrLeaseNotHolder
	}

	now := time.Now().UTC()
	fresh.LeaseExpiresAt = &now
	fresh.UpdatedAt = hlc.Now()
	if err := db.SaveSpace(fresh, []string{"LeaseExpiresAt", "UpdatedAt"}); err != nil {
		return nil, err
	}

	s.SyncSpaceLease(fresh)
	if transport != nil {
		transport.GossipSpace(fresh)
		transport.GossipPoolLease(fresh.Id, fresh.LeaseUserId, leaseExpiryUnix(fresh))
	}
	sse.PublishSpaceChanged(fresh.Id, fresh.UserId)
	return s.leaseInfo(pool, fresh)
}

// ListLeases returns the pool's held leases — active plus draining.
func (s *PoolService) ListLeases(pool *model.PoolDefinition, user *model.User) ([]apiclient.LeaseInfo, error) {
	db := database.GetInstance()
	spaces, err := db.GetSpaces()
	if err != nil {
		return nil, err
	}
	leases := []apiclient.LeaseInfo{}
	for _, sp := range poolMembers(pool, spaces) {
		if sp.LeaseId == "" {
			continue
		}
		holder, err := db.GetUser(sp.LeaseUserId)
		if err != nil {
			holder = nil
		}
		leases = append(leases, s.leaseInfoFor(pool, sp, holder))
	}
	return leases, nil
}

// leaseMember finds the pool member currently holding leaseId.
func (s *PoolService) leaseMember(pool *model.PoolDefinition, leaseId string) (*model.Space, error) {
	spaces, err := database.GetInstance().GetSpaces()
	if err != nil {
		return nil, err
	}
	for _, sp := range poolMembers(pool, spaces) {
		if sp.LeaseId == leaseId {
			return sp, nil
		}
	}
	return nil, ErrLeaseNotFound
}

func leaseExpiryUnix(space *model.Space) int64 {
	if space.LeaseExpiresAt == nil {
		return 0 // never expires
	}
	return space.LeaseExpiresAt.Unix()
}

func (s *PoolService) leaseInfo(pool *model.PoolDefinition, space *model.Space) (*apiclient.LeaseInfo, error) {
	holder, err := database.GetInstance().GetUser(space.LeaseUserId)
	if err != nil {
		holder = nil
	}
	info := s.leaseInfoFor(pool, space, holder)
	return &info, nil
}

func (s *PoolService) leaseInfoFor(pool *model.PoolDefinition, space *model.Space, holder *model.User) apiclient.LeaseInfo {
	state := "active"
	if space.LeasePastExpiry() {
		state = "draining"
	}
	username := ""
	if holder != nil {
		username = holder.Username
	}
	return apiclient.LeaseInfo{
		LeaseId:        space.LeaseId,
		PoolId:         pool.Id,
		PoolName:       pool.Name,
		SpaceId:        space.Id,
		SpaceName:      space.Name,
		UserId:         space.LeaseUserId,
		Username:       username,
		ExpiresAt:      space.LeaseExpiresAt,
		ExtensionsUsed: space.LeaseExtensions,
		MaxExtensions:  pool.LeaseMaxExtensions,
		State:          state,
	}
}

// spaceBusy reports whether the space still has method work in flight —
// either calls forwarded through this server's registry or, authoritatively,
// calls executing in the space's agent regardless of which server forwarded
// them. Agents that predate this reporting show zero.
func (s *PoolService) spaceBusy(spaceID string) bool {
	if methods.DefaultRegistry().InFlightForSpace(spaceID) > 0 {
		return true
	}
	if session := getPoolSession(spaceID); session != nil && session.ActiveMethodCalls > 0 {
		return true
	}
	return false
}

// reclaimLease clears a lease from the space record and the routing view,
// returning the member to the shared pool. Leader-sweep only. Takes the same
// distributed mutation lock as acquire/extend/release and re-validates on a
// fresh read under it — otherwise a concurrent extend on another server (or
// an API goroutine here) could land between the sweep's load and its write,
// and the reclaim would clobber the renewed lease with stale cleared fields.
func (s *PoolService) reclaimLease(space *model.Space) {
	transport := GetTransport()
	token := ""
	if transport != nil {
		if token = transport.LockResource(space.Id); token == "" {
			return // contended (an acquire/extend/release holds it); retry next sweep
		}
		defer transport.UnlockResource(space.Id, token)
	}

	fresh, err := database.GetInstance().GetSpace(space.Id)
	if err != nil || fresh == nil {
		return // retry next sweep
	}
	if fresh.LeaseId == "" {
		// Already reclaimed elsewhere — just converge the local routing view.
		s.MarkLeaseCleared(space.Id)
		return
	}
	if !fresh.LeasePastExpiry() || s.spaceBusy(fresh.Id) {
		return // extended since the sweep loaded its copy, or still draining
	}

	fresh.LeaseClear()
	fresh.UpdatedAt = hlc.Now()
	fields := []string{"LeaseId", "LeaseUserId", "LeaseExpiresAt", "LeaseExtensions", "UpdatedAt"}
	if err := database.GetInstance().SaveSpace(fresh, fields); err != nil {
		return
	}
	s.MarkLeaseCleared(fresh.Id)
	if transport != nil {
		transport.GossipSpace(fresh)
		transport.GossipPoolLeaseClear(fresh.Id)
	}
	sse.PublishSpaceChanged(fresh.Id, fresh.UserId)
}

// reconcileLeases reclaims ended leases: past expiry (or released early),
// a member returns to the shared pool as soon as in-flight method work has
// drained. Never-expiring leases are only ended by Release, which sets an
// expiry and funnels into this same path.
func (s *PoolService) reconcileLeases(members []*model.Space) {
	for _, space := range members {
		if !space.LeasePastExpiry() {
			continue
		}
		if s.spaceBusy(space.Id) {
			continue
		}
		s.reclaimLease(space)
	}
}

// ---------------------------------------------------------------------------
// Lifecycle operations
// ---------------------------------------------------------------------------

func (s *PoolService) Start(pool *model.PoolDefinition, user *model.User) error {
	pool.Active = true
	pool.UpdatedUserId = user.Id
	pool.UpdatedAt = hlc.Now()
	if err := s.savePool(pool); err != nil {
		return err
	}
	db := database.GetInstance()
	spaces, err := db.GetSpaces()
	if err != nil {
		return err
	}
	members := poolMembers(pool, spaces)

	// Delete excess members beyond DesiredCount (they're stopped since pool was stopped)
	if len(members) > pool.DesiredCount {
		for _, sp := range members[pool.DesiredCount:] {
			_ = s.deletePoolSpace(sp)
		}
	}

	// Undrain and start only the keepers
	keepCount := min(len(members), pool.DesiredCount)
	for _, sp := range members[:keepCount] {
		s.undrain(sp.Id)
		s.clearPending(sp.Id)
	}
	for _, sp := range members[:keepCount] {
		if !sp.IsDeleting && !sp.IsDeployed && !sp.IsPending {
			_ = s.startPoolSpace(pool, sp)
		}
	}

	// Create new spaces if still under DesiredCount
	for keepCount < pool.DesiredCount {
		if err := s.createPoolSpace(pool, user); err != nil {
			break
		}
		keepCount++
	}
	return nil
}

func (s *PoolService) Stop(pool *model.PoolDefinition, user *model.User) error {
	// A stop drains and stops every member, which would yank it from under
	// the lease holder; require leases to be ended first.
	db := database.GetInstance()
	spaces, err := db.GetSpaces()
	if err != nil {
		return err
	}
	for _, space := range poolMembers(pool, spaces) {
		if space.LeaseActive() {
			return fmt.Errorf("pool has active leases — release them before stopping the pool")
		}
	}

	pool.Active = false
	pool.UpdatedUserId = user.Id
	pool.UpdatedAt = hlc.Now()
	if err := s.savePool(pool); err != nil {
		return err
	}
	for _, space := range poolMembers(pool, spaces) {
		if space.IsDeleting {
			continue
		}
		if space.IsDeployed || space.IsPending {
			s.drain(space.Id)
			_ = GetContainerService().StopSpace(space)
		}
	}
	return nil
}

func (s *PoolService) SetSize(pool *model.PoolDefinition, desiredCount int, user *model.User) error {
	if desiredCount < 1 {
		return fmt.Errorf("desired_count must be at least 1")
	}
	pool.DesiredCount = desiredCount
	pool.UpdatedUserId = user.Id
	pool.UpdatedAt = hlc.Now()
	return s.savePool(pool)
}

func (s *PoolService) Delete(pool *model.PoolDefinition, user *model.User) error {
	if pool.Active {
		return fmt.Errorf("stop the pool before deleting it")
	}
	db := database.GetInstance()
	spaces, err := db.GetSpaces()
	if err != nil {
		return err
	}
	// All member spaces must be fully stopped before we can delete the pool
	for _, space := range poolMembers(pool, spaces) {
		if space.IsDeployed || space.IsPending {
			return fmt.Errorf("wait for all spaces to stop before deleting the pool")
		}
	}
	// Mark all member spaces as deleting BEFORE tombstoning the pool, so
	// SSE events fire while the pool is still visible in the UI.
	for _, space := range poolMembers(pool, spaces) {
		_ = s.deletePoolSpace(space)
	}
	pool.IsDeleted = true
	pool.Name = pool.Id
	pool.UpdatedUserId = user.Id
	pool.UpdatedAt = hlc.Now()
	return s.savePool(pool)
}

// UpdateStartupScript changes the pool's startup script and applies it to all
// existing member spaces. The pool must be stopped.
func (s *PoolService) UpdateStartupScript(pool *model.PoolDefinition, scriptId string, user *model.User) error {
	if pool.Active {
		return fmt.Errorf("stop the pool before changing the startup script")
	}
	pool.StartupScriptId = scriptId
	pool.UpdatedUserId = user.Id
	pool.UpdatedAt = hlc.Now()
	if err := s.savePool(pool); err != nil {
		return err
	}
	db := database.GetInstance()
	spaces, err := db.GetSpaces()
	if err != nil {
		return err
	}
	for _, space := range poolMembers(pool, spaces) {
		space.StartupScriptId = scriptId
		space.UpdatedAt = hlc.Now()
		if err := db.SaveSpace(space, []string{"StartupScriptId", "UpdatedAt"}); err != nil {
			continue
		}
		if transport := GetTransport(); transport != nil {
			transport.GossipSpace(space)
		}
		sse.PublishSpaceChanged(space.Id, space.UserId)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Sweep loop
// ---------------------------------------------------------------------------

func (s *PoolService) StartSweep() {
	go func() {
		ticker := time.NewTicker(PoolSweepInterval)
		defer ticker.Stop()
		for range ticker.C {
			if transport := GetTransport(); transport != nil && !transport.IsLeader() {
				continue
			}
			_ = s.SweepOnce()
		}
	}()
}

func (s *PoolService) StartReaper() {
	go func() {
		ticker := time.NewTicker(PoolReapInterval)
		defer ticker.Stop()
		for range ticker.C {
			if transport := GetTransport(); transport != nil && !transport.IsLeader() {
				continue
			}
			_ = s.ReapOrphans()
		}
	}()
}

// ReapOrphans finds spaces whose pool has been deleted (or no longer exists)
// and marks them for deletion. This is a safety net for when the normal sweep
// misses spaces — e.g., after a leader crash mid-deletion, or gossip merge
// leaving stale pool_id references.
func (s *PoolService) ReapOrphans() error {
	db := database.GetInstance()
	pools, err := db.GetPoolDefinitions()
	if err != nil {
		return err
	}
	poolMap := make(map[string]*model.PoolDefinition, len(pools))
	for _, pool := range pools {
		poolMap[pool.Id] = pool
	}
	spaces, err := db.GetSpaces()
	if err != nil {
		return err
	}
	cfg := config.GetServerConfig()
	for _, space := range spaces {
		if space.PoolId == "" || space.IsDeleted || space.IsDeleting {
			continue
		}
		// Each zone reaps only its own pool spaces. The owning pool may be
		// gone entirely, so we gate on the space's zone rather than the pool's.
		if space.Zone != "" && space.Zone != cfg.Zone {
			continue
		}
		pool, exists := poolMap[space.PoolId]
		if !exists || pool.IsDeleted {
			_ = s.deletePoolSpace(space)
		}
	}
	return nil
}

func (s *PoolService) SweepOnce() error {
	db := database.GetInstance()
	pools, err := db.GetPoolDefinitions()
	if err != nil {
		return err
	}
	spaces, err := db.GetSpaces()
	if err != nil {
		return err
	}
	cfg := config.GetServerConfig()
	for _, pool := range pools {
		// A pool's spaces are managed only by the leader of its owning zone.
		// 10 zones => 10 leaders, each reconciling the pools created in its
		// own zone. Empty zone (legacy/unmigrated) is managed everywhere.
		if pool.Zone != "" && pool.Zone != cfg.Zone {
			continue
		}
		members := poolMembers(pool, spaces)
		// Reclaim ended leases first (works for stopped pools too): once a
		// member is back its routing exclusion lifts and it counts as a
		// normal keeper/shrink candidate again.
		s.reconcileLeases(members)
		if pool.IsDeleted {
			s.reconcileDeletedPool(members)
			continue
		}
		if !pool.Active {
			s.reconcileStoppedPool(pool, members)
			continue
		}
		s.reconcile(pool, members)
	}
	return nil
}

func (s *PoolService) reconcileDeletedPool(members []*model.Space) {
	for _, space := range members {
		if space.IsDeleting {
			continue
		}
		s.drain(space.Id)
		if space.IsPending {
			continue
		}
		_ = s.deletePoolSpace(space)
	}
}

// reconcileStoppedPool ensures all spaces are stopped for a stopped pool, and
// deletes excess stopped members to shrink the pool to DesiredCount.
func (s *PoolService) reconcileStoppedPool(pool *model.PoolDefinition, members []*model.Space) {
	// Wait for any pending transitions
	for _, sp := range members {
		if sp.IsPending {
			return
		}
	}

	// Retry stuck deletions
	for _, sp := range members {
		if sp.IsDeleting {
			GetContainerService().DeleteSpace(sp)
			return
		}
	}

	// Stop any still-deployed members (drain first to stop new traffic).
	// Leased members are left alone until their lease drains — normally
	// impossible (Stop refuses with active leases) but kept as a guard
	// against races.
	for _, sp := range members {
		if sp.IsDeployed && sp.LeaseId == "" {
			s.drain(sp.Id)
			_ = GetContainerService().StopSpace(sp)
			return
		}
	}

	// All members stopped — delete excess beyond DesiredCount
	if len(members) > pool.DesiredCount {
		excess := members[pool.DesiredCount:]
		for _, sp := range excess {
			if sp.LeaseId != "" {
				continue // still leased or draining; the sweep retries later
			}
			_ = s.deletePoolSpace(sp)
		}
	}
}

// reconcile brings an active pool to DesiredCount.
//
// State machine (runs every PoolSweepInterval):
//
//  1. If any member is IsPending → skip, wait for the transition.
//  2. If any member is IsDeleting → retry DeleteSpace, wait.
//  3. No transitions in progress:
//     - total > DesiredCount:
//     Excess running → drain (stop new traffic), mark. Next sweep → StopSpace.
//     Excess stopped → grace period (2 passes) → deletePoolSpace.
//     - total <= DesiredCount:
//     Start stopped spaces (undrain first). Create new if still under count.
func (s *PoolService) reconcile(pool *model.PoolDefinition, members []*model.Space) {
	for _, sp := range members {
		if sp.IsPending {
			return
		}
	}

	for _, sp := range members {
		if sp.IsDeleting {
			GetContainerService().DeleteSpace(sp)
			return
		}
	}

	user, err := database.GetInstance().GetUser(pool.CreatedUserId)
	if err != nil || user == nil {
		return
	}

	if len(members) > pool.DesiredCount {
		s.handleExcess(pool, members)
		return
	}

	// At or below DesiredCount — clear drain/pending state for all members
	for _, sp := range members {
		s.undrain(sp.Id)
		s.clearPending(sp.Id)
	}

	for _, sp := range members {
		if !sp.IsDeployed {
			_ = s.startPoolSpace(pool, sp)
			return
		}
	}

	for len(members) < pool.DesiredCount {
		if err := s.createPoolSpace(pool, user); err != nil {
			break
		}
		members = append(members, nil)
	}
}

// handleExcess processes spaces beyond DesiredCount.
//
// Running excess spaces are drained first (stop routing new method calls).
// On the next sweep they are still excess → StopSpace.
//
// Stopped excess spaces get a 2-pass grace period:
//   - First pass: mark the space in pendingDeletions.
//   - Second pass: if still excess and stopped → deletePoolSpace.
//
// This allows a space to be reused (undrained + restarted) if DesiredCount
// goes back up before the grace period expires.
func (s *PoolService) handleExcess(pool *model.PoolDefinition, members []*model.Space) {
	// Leased members are never shrink candidates — the lease outranks the
	// shrink until it ends. Fill the keeper set with leased members first,
	// then free members up to DesiredCount; the rest is the excess.
	keepers := make([]*model.Space, 0, len(members))
	var excess []*model.Space
	for _, sp := range members {
		if sp.LeaseId != "" || len(keepers) < pool.DesiredCount {
			keepers = append(keepers, sp)
		} else {
			excess = append(excess, sp)
		}
	}

	// Clear state for keepers — they're staying
	for _, sp := range keepers {
		s.undrain(sp.Id)
		s.clearPending(sp.Id)
	}

	for _, sp := range excess {
		if sp.IsDeployed {
			// Running excess — drain it now, stop on next sweep if still excess
			if !s.isDrained(sp.Id) {
				s.drain(sp.Id)
			} else {
				_ = GetContainerService().StopSpace(sp)
			}
		} else {
			// Stopped excess — grace period before deletion
			if s.isPending(sp.Id) {
				s.clearPending(sp.Id)
				_ = s.deletePoolSpace(sp)
			} else {
				s.markPending(sp.Id)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Drain / pending helpers
// ---------------------------------------------------------------------------

func (s *PoolService) drain(spaceID string) {
	s.mu.Lock()
	s.drained[spaceID] = true
	s.mu.Unlock()
	if transport := GetTransport(); transport != nil {
		transport.GossipPoolDrain(spaceID)
	} else {
		methods.DefaultRegistry().Drain(spaceID)
	}
}

func (s *PoolService) undrain(spaceID string) {
	s.mu.Lock()
	was := s.drained[spaceID]
	delete(s.drained, spaceID)
	s.mu.Unlock()
	if !was {
		return
	}
	if transport := GetTransport(); transport != nil {
		transport.GossipPoolUndrain(spaceID)
	} else {
		methods.DefaultRegistry().Undrain(spaceID)
	}
}

func (s *PoolService) isDrained(spaceID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.drained[spaceID]
}

func (s *PoolService) markPending(spaceID string) {
	s.mu.Lock()
	if _, exists := s.pendingDeletions[spaceID]; !exists {
		s.pendingDeletions[spaceID] = time.Now()
	}
	s.mu.Unlock()
}

func (s *PoolService) isPending(spaceID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.pendingDeletions[spaceID]
	return exists
}

func (s *PoolService) clearPending(spaceID string) {
	s.mu.Lock()
	delete(s.pendingDeletions, spaceID)
	s.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Space helpers
// ---------------------------------------------------------------------------

// memberOrdinal extracts the 0-based member number encoded in a pool member's
// name (poolname-N). Returns false if the name doesn't follow the scheme — e.g.
// a member that was manually renamed. The number lives only in the name; pools
// are small enough that deriving it on demand is cheaper than persisting it.
func memberOrdinal(poolName string, space *model.Space) (int, bool) {
	prefix := poolName + "-"
	if !strings.HasPrefix(space.Name, prefix) {
		return 0, false
	}
	n, err := strconv.Atoi(space.Name[len(prefix):])
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// poolMembers returns the pool's non-deleted spaces sorted by member number
// ascending (un-numbered members last). Ordering is what gives the reconcile
// loop its "keep the lowest numbers, release the highest, restart the lowest
// stopped" behaviour: keepers are members[:n] and excess is members[n:].
func poolMembers(pool *model.PoolDefinition, spaces []*model.Space) []*model.Space {
	var members []*model.Space
	for _, space := range spaces {
		if space.PoolId == pool.Id && !space.IsDeleted {
			members = append(members, space)
		}
	}
	sort.SliceStable(members, func(i, j int) bool {
		oi, oki := memberOrdinal(pool.Name, members[i])
		oj, okj := memberOrdinal(pool.Name, members[j])
		if oki != okj {
			return oki // numbered members sort before un-numbered ones
		}
		return oi < oj
	})
	return members
}

// nextPoolOrdinal returns the lowest non-negative number not currently used by a
// member name. A linear scan is fine: pools never hold enough members to matter.
func nextPoolOrdinal(poolName string, members []*model.Space) int {
	used := make(map[int]bool, len(members))
	for _, m := range members {
		if n, ok := memberOrdinal(poolName, m); ok {
			used[n] = true
		}
	}
	n := 0
	for used[n] {
		n++
	}
	return n
}

func (s *PoolService) createPoolSpace(pool *model.PoolDefinition, user *model.User) error {
	db := database.GetInstance()
	template, err := db.GetTemplate(pool.TemplateId)
	if err != nil {
		return err
	}
	nodeId, err := SelectNodeForSpace(template, "")
	if err != nil {
		return err
	}
	shell := user.PreferredShell
	if shell == "" {
		shell = "zsh"
	}

	// Allocate the lowest free ordinal and persist the new member atomically.
	// createMu serializes allocation so a concurrent Create + sweep on the
	// leader can't hand the same number to two spaces. The lock is released
	// before the (potentially slow) StartSpace call — the ordinal is committed
	// once the space row exists.
	s.createMu.Lock()
	spaces, err := db.GetSpaces()
	if err != nil {
		s.createMu.Unlock()
		return err
	}
	ordinal := nextPoolOrdinal(pool.Name, poolMembers(pool, spaces))
	name := pool.Name + "-" + strconv.Itoa(ordinal)
	// Pool members are created without user input, so any template field
	// defaults are exactly what the member should carry.
	customFields := model.ApplyCustomFieldDefaults(template, nil)
	space := model.NewSpace(name, "Pool member for "+pool.Name, user.Id, pool.TemplateId, shell, &[]model.AltNameEntry{}, "", "", customFields)
	space.PoolId = pool.Id
	space.StartupScriptId = pool.StartupScriptId
	space.NodeId = nodeId
	err = GetSpaceService().CreateSpace(space, user)
	s.createMu.Unlock()
	if err != nil {
		return err
	}

	if pool.Active {
		return GetContainerService().StartSpace(space, template, user)
	}
	return nil
}

func (s *PoolService) startPoolSpace(pool *model.PoolDefinition, space *model.Space) error {
	db := database.GetInstance()
	user, err := db.GetUser(pool.CreatedUserId)
	if err != nil || user == nil {
		return fmt.Errorf("pool owner not found")
	}
	template, err := db.GetTemplate(pool.TemplateId)
	if err != nil || template == nil {
		return fmt.Errorf("template not found")
	}
	return GetContainerService().StartSpace(space, template, user)
}

// deletePoolSpace initiates deletion of a STOPPED pool space via the normal
// two-phase flow (IsDeleting → containerService.DeleteSpace → IsDeleted).
func (s *PoolService) deletePoolSpace(space *model.Space) error {
	if space == nil || space.IsDeleted || space.IsDeleting || space.IsPending {
		return nil
	}
	s.drain(space.Id)
	space.IsDeleting = true
	space.UpdatedAt = hlc.Now()
	if err := database.GetInstance().SaveSpace(space, []string{"IsDeleting", "UpdatedAt"}); err != nil {
		return err
	}
	if transport := GetTransport(); transport != nil {
		transport.GossipSpace(space)
	}
	sse.PublishSpaceChanged(space.Id, space.UserId)
	GetContainerService().DeleteSpace(space)
	return nil
}

// ---------------------------------------------------------------------------
// Port routing support
// ---------------------------------------------------------------------------

// IsDrained returns true if the pool sweep has drained the space (stopped
// routing new method calls to it). Used by the HTTP/TCP proxy to skip
// pool members that are being removed.
func (s *PoolService) IsDrained(spaceID string) bool {
	return s.isDrained(spaceID)
}

// MarkDrained sets the local drain flag without re-gossiping. Called by
// the cluster handler on peer nodes when receiving a drain message from
// the leader, so that HTTP/TCP routing also skips the drained member.
func (s *PoolService) MarkDrained(spaceID string) {
	s.mu.Lock()
	s.drained[spaceID] = true
	s.mu.Unlock()
}

// MarkUndrained clears the local drain flag without re-gossiping. Called by
// the cluster handler on peer nodes when receiving an undrain message.
func (s *PoolService) MarkUndrained(spaceID string) {
	s.mu.Lock()
	delete(s.drained, spaceID)
	s.mu.Unlock()
}

// PickMemberForRouting selects a healthy, deployed, non-drained, non-leased
// member of the pool using round-robin. Returns nil if no suitable member
// exists. Exclusively leased members are skipped — the holder reaches its
// member by the member's own name instead.
func (s *PoolService) PickMemberForRouting(poolName, userId string) *model.Space {
	db := database.GetInstance()
	pool, err := db.GetPoolDefinitionByName(userId, poolName)
	if err != nil || pool == nil || pool.IsDeleted {
		return nil
	}
	spaces, err := db.GetSpaces()
	if err != nil {
		return nil
	}
	var candidates []*model.Space
	for _, sp := range spaces {
		if sp.PoolId != pool.Id || sp.IsDeleted || sp.IsDeleting {
			continue
		}
		if !sp.IsDeployed || sp.IsPending {
			continue
		}
		if s.isDrained(sp.Id) {
			continue
		}
		if sp.LeaseId != "" {
			continue // exclusively leased (or draining after the lease ended)
		}
		if h := health.Get(sp.Id); h != nil && !h.Healthy {
			continue
		}
		candidates = append(candidates, sp)
	}
	if len(candidates) == 0 {
		return nil
	}
	s.mu.Lock()
	idx := s.rrCounters[pool.Id] % len(candidates)
	s.rrCounters[pool.Id] = (idx + 1) % len(candidates)
	s.mu.Unlock()
	return candidates[idx]
}

// PoolNameForSpace returns the pool name if the space is a pool member, or "".
func PoolNameForSpace(space *model.Space) string {
	if space == nil || space.PoolId == "" {
		return ""
	}
	pool, err := database.GetInstance().GetPoolDefinition(space.PoolId)
	if err != nil || pool == nil || pool.IsDeleted {
		return ""
	}
	return pool.Name
}
