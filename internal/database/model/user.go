package model

import (
	"math/rand"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/util/crypt"
	"golang.org/x/crypto/bcrypt"
)

// ExternalProvider holds the identity and token for a linked OAuth provider.
type ExternalProvider struct {
	ProviderUID  string `json:"provider_uid"`            // provider's own stable user ID (e.g. GitHub integer ID)
	Username     string `json:"username"`                // human-readable handle, may change
	Token        string `json:"token"`                   // encrypted access token
	RefreshToken string `json:"refresh_token,omitempty"` // encrypted refresh token
}

// User object
type User struct {
	Id                    string                      `json:"user_id" db:"user_id,pk" msgpack:"user_id"`
	Username              string                      `json:"username" db:"username" msgpack:"username"`
	Email                 string                      `json:"email" db:"email" msgpack:"email"`
	Password              string                      `json:"password" db:"password" msgpack:"password"`
	TOTPSecret            string                      `json:"totp_secret" db:"totp_secret" msgpack:"totp_secret"`
	ServicePassword       string                      `json:"service_password" db:"service_password" msgpack:"service_password"`
	SSHPublicKey          string                      `json:"ssh_public_key" db:"ssh_public_key" msgpack:"ssh_public_key"`
	SSHPrivateKey         string                      `json:"ssh_private_key" db:"ssh_private_key" msgpack:"ssh_private_key"`
	GitHubUsername        string                      `json:"github_username" db:"github_username" msgpack:"github_username"`
	ExternalAuthProviders map[string]ExternalProvider `json:"external_auth_providers" db:"external_auth_providers,json" msgpack:"external_auth_providers"`
	Roles                 []string                    `json:"roles" db:"roles,json" msgpack:"roles"`
	Groups                []string                    `json:"groups" db:"groups,json" msgpack:"groups"`
	// LinkedUsers holds the other members of this user's switch group.
	// Every member stores the same group minus itself, so the profile
	// menu offers the full group from any member and the list survives
	// switches. Maintained only through the link/unlink endpoints.
	LinkedUsers    []string       `json:"linked_users" db:"linked_users,json" msgpack:"linked_users"`
	Active         bool           `json:"active" db:"active" msgpack:"active"`
	IsDeleted      bool           `json:"is_deleted" db:"is_deleted" msgpack:"is_deleted"`
	MaxSpaces      uint32         `json:"max_spaces" db:"max_spaces" msgpack:"max_spaces"`
	ComputeUnits   uint32         `json:"compute_units" db:"compute_units" msgpack:"compute_units"`
	StorageUnits   uint32         `json:"storage_units" db:"storage_units" msgpack:"storage_units"`
	MaxTunnels     uint32         `json:"max_tunnels" db:"max_tunnels" msgpack:"max_tunnels"`
	PreferredShell string         `json:"preferred_shell" db:"preferred_shell" msgpack:"preferred_shell"`
	Timezone       string         `json:"timezone" db:"timezone" msgpack:"timezone"`
	Preferences    map[string]any `json:"preferences" db:"preferences,json" msgpack:"preferences"`
	LastLoginAt    *time.Time     `json:"last_login_at" db:"last_login_at" msgpack:"last_login_at"`
	UpdatedAt      hlc.Timestamp  `json:"updated_at" db:"updated_at" msgpack:"updated_at"`
	CreatedAt      time.Time      `json:"created_at" db:"created_at" msgpack:"created_at"`
}

type Usage struct {
	ComputeUnits               uint32
	StorageUnits               uint32
	NumberSpaces               int
	NumberSpacesDeployed       int
	NumberSpacesDeployedInZone int
}

type Quota struct {
	ComputeUnits uint32
	StorageUnits uint32
	MaxSpaces    uint32
	MaxTunnels   uint32
}

func NewUser(username string, email string, password string, roles []string, groups []string, sshPublicKey string, preferredShell string, timezone string, maxSpaces uint32, githubUsername string, computeUnits uint32, storageUnits uint32, maxTunnels uint32) *User {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal(err.Error())
	}

	user := &User{
		Id:              id.String(),
		Username:        username,
		Email:           email,
		Active:          true,
		SSHPublicKey:    sshPublicKey,
		GitHubUsername:  githubUsername,
		PreferredShell:  preferredShell,
		Roles:           roles,
		Groups:          groups,
		Timezone:        timezone,
		MaxSpaces:       maxSpaces,
		ComputeUnits:    computeUnits,
		StorageUnits:    storageUnits,
		MaxTunnels:      maxTunnels,
		ServicePassword: generateRandomString(16),
		UpdatedAt:       hlc.Now(),
		CreatedAt:       time.Now().UTC(),
	}

	user.SetPassword(password)

	return user
}

// Set the password for the user
func (u *User) SetPassword(password string) error {
	// Create bcrypt password
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err == nil {
		u.Password = string(bytes)
	}

	return err
}

// Check the password for the user
func (u *User) CheckPassword(password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)) == nil
}

func (u *User) HasPermission(permission uint16) bool {
	for _, role := range u.Roles {

		// If role exists in rolePermissions map then check if the permission belongs to the role
		if r, ok := roleCache[role]; ok {
			for _, p := range r.Permissions {
				if p == permission {
					return true
				}
			}
		}
	}

	return false
}

// HasPluginPermission reports whether any of the user's roles carries the
// fully qualified plugin grant (e.g. "plugin.metrics.read"). Plugin grants
// are text so they are cluster-order independent and survive plugin
// uninstall/reinstall inertly (PLUGINS2.md §5). The fixed admin role cannot
// be granted per-plugin permissions through the API, so it passes every
// plugin permission check.
// PassesPluginGate reports whether the user passes a declared plugin
// permission gate: an empty permission means any logged-in user, a set
// one requires the qualified grant (admins pass every check). This is the
// one predicate every declared gate — pages, handlers, menus, tools,
// field handlers — checks through.
func (u *User) PassesPluginGate(permission string) bool {
	return permission == "" || u.HasPluginPermission(permission)
}

func (u *User) HasPluginPermission(name string) bool {
	if u.IsAdmin() {
		return true
	}
	for _, role := range u.Roles {
		if r, ok := roleCache[role]; ok {
			for _, p := range r.PluginPermissions {
				if p == name {
					return true
				}
			}
		}
	}
	return false
}

// GrantedPermissionKeys returns the stable snake_case keys of the built-in
// permissions the user holds across their roles; the fixed admin role
// holds all of them. Keys — not display names — are the machine surface
// (user.has_permission), so rewording a permission's display string can
// never break plugin logic. The enforcement surface remains HasPermission.
func (u *User) GrantedPermissionKeys() []string {
	out := []string{}
	seen := map[uint16]bool{}
	if u.IsAdmin() {
		for _, pn := range PermissionNames {
			if !seen[uint16(pn.Id)] {
				seen[uint16(pn.Id)] = true
				if key := permissionKeys[uint16(pn.Id)]; key != "" {
					out = append(out, key)
				}
			}
		}
		return out
	}
	for _, role := range u.Roles {
		if r, ok := roleCache[role]; ok {
			for _, p := range r.Permissions {
				if !seen[p] {
					seen[p] = true
					if key := permissionKeys[p]; key != "" {
						out = append(out, key)
					}
				}
			}
		}
	}
	return out
}

// GrantedPluginPermissions returns the qualified plugin grants (e.g.
// "plugin.metrics.read") the user holds across their roles. The admin role
// passes every plugin permission check without carrying grants; callers
// treat is-admin as the superset signal.
func (u *User) GrantedPluginPermissions() []string {
	out := []string{}
	seen := map[string]bool{}
	for _, role := range u.Roles {
		if r, ok := roleCache[role]; ok {
			for _, p := range r.PluginPermissions {
				if !seen[p] {
					seen[p] = true
					out = append(out, p)
				}
			}
		}
	}
	return out
}

func (u *User) HasAnyGroup(groups *[]string) bool {

	// If user has no groups then return false
	if len(u.Groups) == 0 {
		return false
	}

	// If user has groups then check if any of the groups match
	for _, group := range u.Groups {
		for _, g := range *groups {
			if g == group {
				return true
			}
		}
	}

	return false
}

func (u *User) IsAdmin() bool {
	for _, role := range u.Roles {
		if role == RoleAdminUUID {
			return true
		}
	}

	return false
}

// SetOAuthTokens stores encrypted OAuth tokens for the given provider.
// Refresh tokens are preserved when the provider omits them on a later login.
func (u *User) SetOAuthTokens(providerID, token, refreshToken, encryptionKey string) {
	if u.ExternalAuthProviders == nil {
		return
	}
	if ep, ok := u.ExternalAuthProviders[providerID]; ok {
		ep.Token = crypt.EncryptB64(encryptionKey, token)
		if refreshToken != "" {
			ep.RefreshToken = crypt.EncryptB64(encryptionKey, refreshToken)
		}
		u.ExternalAuthProviders[providerID] = ep
	}
}

// GetOAuthToken returns the decrypted OAuth access token for the given provider, or empty string.
func (u *User) GetOAuthToken(providerID, encryptionKey string) string {
	if u.ExternalAuthProviders == nil {
		return ""
	}
	ep, ok := u.ExternalAuthProviders[providerID]
	if !ok || ep.Token == "" {
		return ""
	}
	return crypt.DecryptB64(encryptionKey, ep.Token)
}

// GetOAuthRefreshToken returns the decrypted OAuth refresh token for the given provider, or empty string.
func (u *User) GetOAuthRefreshToken(providerID, encryptionKey string) string {
	if u.ExternalAuthProviders == nil {
		return ""
	}
	ep, ok := u.ExternalAuthProviders[providerID]
	if !ok || ep.RefreshToken == "" {
		return ""
	}
	return crypt.DecryptB64(encryptionKey, ep.RefreshToken)
}

// ClearOAuthTokens removes the stored tokens for the given provider.
func (u *User) SetSSHPrivateKeyEncrypted(privateKey string, encryptionKey string) {
	u.SSHPrivateKey = crypt.EncryptB64Safe(encryptionKey, privateKey)
}

func (u *User) GetSSHPrivateKeyDecrypted(encryptionKey string) string {
	return crypt.DecryptB64Safe(encryptionKey, u.SSHPrivateKey)
}

func (u *User) ClearOAuthTokens(providerID string) {
	if ep, ok := u.ExternalAuthProviders[providerID]; ok {
		ep.Token = ""
		ep.RefreshToken = ""
		u.ExternalAuthProviders[providerID] = ep
	}
}

// Preference keys stored under User.Preferences.
const PrefNavStarred = "nav.starred"

// GetNavStarred returns the user's pinned (starred) navigation URLs in their
// chosen display order, or nil if none are set. JSON round-trips decode the
// stored array as []any, so coerce back to []string here.
func (u *User) GetNavStarred() []string {
	if u.Preferences == nil {
		return nil
	}
	switch arr := u.Preferences[PrefNavStarred].(type) {
	case []string:
		return arr
	case []any:
		out := make([]string, 0, len(arr))
		for _, v := range arr {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	return nil
}

// SetNavStarred stores the given navigation URLs as the user's pinned set in
// the supplied order. An empty slice clears the preference (opts out of the
// starred layout and returns the menu to its default arrangement).
func (u *User) SetNavStarred(order []string) {
	if u.Preferences == nil {
		u.Preferences = map[string]any{}
	}
	if len(order) == 0 {
		delete(u.Preferences, PrefNavStarred)
		if len(u.Preferences) == 0 {
			u.Preferences = nil
		}
		return
	}
	u.Preferences[PrefNavStarred] = order
}

// LinkUsers joins the switch groups of a and b into one and returns the
// merged member ids (including a.Id and b.Id). a and b's in-memory records
// are updated; the caller persists every member of the returned set —
// transitive members from either side need their lists rewritten too, via
// SetLinkedGroup.
func LinkUsers(a, b *User) []string {
	if a == nil || b == nil || a.Id == b.Id {
		return nil
	}
	group := map[string]bool{a.Id: true, b.Id: true}
	for _, id := range a.LinkedUsers {
		group[id] = true
	}
	for _, id := range b.LinkedUsers {
		group[id] = true
	}

	out := sortedKeys(group)
	a.LinkedUsers = linkedListFor(a.Id, out)
	b.LinkedUsers = linkedListFor(b.Id, out)
	a.UpdatedAt = hlc.Now()
	b.UpdatedAt = hlc.Now()
	return out
}

// UnlinkUser detaches other from user's switch group and returns the
// remaining member ids (including user.Id). Other keeps no links while the
// remaining members keep each other; the caller persists every returned
// member plus other.
func UnlinkUser(user, other *User) []string {
	if user == nil || other == nil {
		return nil
	}
	group := map[string]bool{user.Id: true}
	for _, id := range user.LinkedUsers {
		group[id] = true
	}
	delete(group, other.Id)

	out := sortedKeys(group)
	user.LinkedUsers = linkedListFor(user.Id, out)
	other.LinkedUsers = nil
	user.UpdatedAt = hlc.Now()
	other.UpdatedAt = hlc.Now()
	return out
}

// SetLinkedGroup writes group (which must contain u's own id) onto u as its
// linked-users list minus itself.
func SetLinkedGroup(u *User, group []string) {
	u.LinkedUsers = linkedListFor(u.Id, group)
	u.UpdatedAt = hlc.Now()
}

// linkedListFor returns group minus self, sorted so every member stores the
// same canonical list.
func linkedListFor(selfId string, group []string) []string {
	links := make([]string, 0, len(group))
	for _, id := range group {
		if id != selfId {
			links = append(links, id)
		}
	}
	sort.Strings(links)
	return links
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}
