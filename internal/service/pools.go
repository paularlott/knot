package service

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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
)

type PoolService struct {
	mu               sync.Mutex
	pendingDeletions map[string]time.Time // space ID -> first seen as excess stopped
	drained          map[string]bool      // space ID -> currently drained by pool sweep
	rrCounters       map[string]int       // pool ID -> round-robin cursor
	createMu         sync.Mutex           // serializes member ordinal allocation
}

type PoolSessionState struct {
	CPUPercent       float64
	MemoryUsedBytes  uint64
	MemoryLimitBytes uint64
	MethodRPS        float64
	HTTPRPS          float64
	TCPRPS           float64
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
		Id:              pool.Id,
		Name:            pool.Name,
		TemplateId:      pool.TemplateId,
		StartupScriptId: pool.StartupScriptId,
		DesiredCount:    pool.DesiredCount,
		Active:          pool.Active,
		Members:         []apiclient.PoolMemberInfo{},
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
	if pool.StartupScriptId != "" {
		if _, err := db.GetScript(pool.StartupScriptId); err != nil {
			return fmt.Errorf("startup script not found")
		}
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
	pool.Active = false
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

	// Stop any still-deployed members (drain first to stop new traffic)
	for _, sp := range members {
		if sp.IsDeployed {
			s.drain(sp.Id)
			_ = GetContainerService().StopSpace(sp)
			return
		}
	}

	// All members stopped — delete excess beyond DesiredCount
	if len(members) > pool.DesiredCount {
		excess := members[pool.DesiredCount:]
		for _, sp := range excess {
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
//       Excess running → drain (stop new traffic), mark. Next sweep → StopSpace.
//       Excess stopped → grace period (2 passes) → deletePoolSpace.
//     - total <= DesiredCount:
//       Start stopped spaces (undrain first). Create new if still under count.
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
		s.handleExcess(members, pool.DesiredCount)
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
func (s *PoolService) handleExcess(members []*model.Space, desiredCount int) {
	keepers := members[:desiredCount]
	excess := members[desiredCount:]

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
	space := model.NewSpace(name, "Pool member for "+pool.Name, user.Id, pool.TemplateId, shell, &[]model.AltNameEntry{}, "", "", nil)
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

// PickMemberForRouting selects a healthy, deployed, non-drained member of the
// pool using round-robin. Returns nil if no suitable member exists.
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
