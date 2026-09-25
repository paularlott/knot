package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"

	"github.com/paularlott/mcp"
)

// GetAccessibleSkills returns the active, zone-valid, ACL-filtered skills for
// the user (global + own), with user skills overriding globals of the same
// name. Returns nil if the user is nil or the database is unreachable.
func GetAccessibleSkills(user *model.User) []*model.Skill {
	if user == nil {
		return nil
	}

	db := database.GetInstance()
	if db == nil {
		return nil
	}

	skills, err := db.GetSkills()
	if err != nil {
		return nil
	}

	currentZone := config.GetServerConfig().Zone

	byName := make(map[string]*model.Skill)
	for _, skill := range skills {
		if !skill.Active || skill.IsDeleted {
			continue
		}
		if !skill.IsValidForZone(currentZone) {
			continue
		}
		if !service.CanUserAccessSkill(user, skill) {
			continue
		}
		if existing, ok := byName[skill.Name]; ok {
			if skill.IsUserSkill() && !existing.IsUserSkill() {
				byName[skill.Name] = skill
			}
		} else {
			byName[skill.Name] = skill
		}
	}

	if len(byName) == 0 {
		return nil
	}

	result := make([]*model.Skill, 0, len(byName))
	for _, s := range byName {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// skillsProvider exposes knot's database-backed skills over the skills
// extension (SEP-2640): entries for skills/list and skills/get, and the
// synthesized SKILL.md as an ordinary skill:// resource. Scoped to one user —
// every listing and read goes through GetAccessibleSkills, so zone, group and
// user-overrides-global rules apply identically on every path.
type skillsProvider struct {
	user *model.User
}

var (
	_ mcp.SkillProvider    = (*skillsProvider)(nil)
	_ mcp.ResourceProvider = (*skillsProvider)(nil)
)

// NewSkillsProvider returns the per-user skills provider. Attach it to the
// request context with mcp.WithSkillProviders AND mcp.WithResourceProviders
// (it implements both: the extension listing, and the resources the entries
// point at).
func NewSkillsProvider(user *model.User) *skillsProvider {
	return &skillsProvider{user: user}
}

// skillEntryURI is the SKILL.md resource URI for a skill name. The final path
// segment before SKILL.md equals the skill's name, per the Agent Skills
// specification.
func skillEntryURI(name string) string {
	return "skill://" + name + "/SKILL.md"
}

// skillMarkdown renders one database skill as a SKILL.md document: minimal
// frontmatter (name, description) followed by the stored content. The
// listing's frontmatter derives from the same two fields, so listing and
// served file agree by construction — which is exactly what conformance
// checkers compare. Frontmatter values are flattened to one line (a newline
// would terminate the block early).
func skillMarkdown(skill *model.Skill) []byte {
	name := strings.NewReplacer("\r", " ", "\n", " ").Replace(skill.Name)
	description := strings.NewReplacer("\r", " ", "\n", " ").Replace(skill.Description)
	md := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s", name, description, skill.Content)
	return []byte(md)
}

// skillEntry builds the skills/list entry for one database skill: a single
// resource (the SKILL.md) with its digest and size.
func skillEntry(skill *model.Skill) mcp.Skill {
	md := skillMarkdown(skill)
	sum := sha256.Sum256(md)
	return mcp.Skill{
		URI:         skillEntryURI(skill.Name),
		Frontmatter: map[string]any{"name": skill.Name, "description": skill.Description},
		Resources: []mcp.SkillResource{{
			URI:    skillEntryURI(skill.Name),
			Digest: "sha256:" + hex.EncodeToString(sum[:]),
			Size:   int64(len(md)),
		}},
	}
}

// representableSkillName reports whether a database skill's name can be
// served over the skills extension: the name becomes both a path segment of
// the URI (skill://<name>/SKILL.md) and a frontmatter value, so anything
// that would break the path (slashes, empty) or diverge the two (line
// breaks, which a frontmatter block cannot carry) is not publishable.
func representableSkillName(name string) bool {
	return name != "" && !strings.ContainsAny(name, "\r\n/")
}

// ListSkills implements mcp.SkillProvider.
func (p *skillsProvider) ListSkills(ctx context.Context) ([]mcp.Skill, error) {
	skills := GetAccessibleSkills(p.user)
	if len(skills) == 0 {
		return nil, nil
	}
	out := make([]mcp.Skill, 0, len(skills))
	for _, skill := range skills {
		if !representableSkillName(skill.Name) {
			log.WithGroup("mcp").Warn("Skipping skill: name cannot be served as both a skill URI and frontmatter", "name", skill.Name)
			continue
		}
		out = append(out, skillEntry(skill))
	}
	return out, nil
}

// GetResources implements mcp.ResourceProvider: one skill:// descriptor per
// accessible skill, so the files surface in resources/list next to their
// skills/list entries. Skills with unrepresentable names are skipped, as in
// ListSkills.
func (p *skillsProvider) GetResources(ctx context.Context) (*mcp.ProvidedResources, error) {
	skills := GetAccessibleSkills(p.user)
	out := &mcp.ProvidedResources{}
	for _, skill := range skills {
		if !representableSkillName(skill.Name) {
			continue
		}
		out.Resources = append(out.Resources, mcp.MCPResource{
			URI:         skillEntryURI(skill.Name),
			Name:        skill.Name,
			Description: skill.Description,
			MimeType:    "text/markdown",
		})
	}
	return out, nil
}

// ReadResource implements mcp.ResourceProvider, serving the synthesized
// SKILL.md for one of the user's accessible skills. Any other skill:// URI
// (unknown name, or a subpath a database skill cannot have) is a miss, so
// dispatch falls through to the remote servers.
func (p *skillsProvider) ReadResource(ctx context.Context, uri string) (*mcp.ResourceResponse, error) {
	const prefix, suffix = "skill://", "/SKILL.md"
	if !strings.HasPrefix(uri, prefix) || !strings.HasSuffix(uri, suffix) {
		return nil, mcp.ErrUnknownResource
	}
	name := strings.TrimSuffix(strings.TrimPrefix(uri, prefix), suffix)
	if name == "" || strings.Contains(name, "/") {
		return nil, mcp.ErrUnknownResource
	}
	for _, skill := range GetAccessibleSkills(p.user) {
		if skill.Name == name {
			return mcp.NewResourceResponseText(uri, string(skillMarkdown(skill)), "text/markdown"), nil
		}
	}
	return nil, mcp.ErrUnknownResource
}

// operatorRemotes records the clients for operator-configured remote servers
// (server.mcp.remote_servers) as InitializeMCPServer creates them, keyed by
// namespace so re-registration replaces instead of duplicating (two operator
// remotes sharing a namespace would collide in tool federation anyway). The
// internal server holds them for tool federation; this registry lets the
// skills prompt ask each one for skills/list directly (skills are not
// federated through RegisterRemoteServer).
var (
	operatorRemotesMu sync.RWMutex
	operatorRemotes   = map[string]*mcp.Client{}
)

func recordOperatorRemote(namespace string, client *mcp.Client) {
	operatorRemotesMu.Lock()
	defer operatorRemotesMu.Unlock()
	operatorRemotes[namespace] = client
}

func snapshotOperatorRemotes() map[string]*mcp.Client {
	operatorRemotesMu.RLock()
	defer operatorRemotesMu.RUnlock()
	out := make(map[string]*mcp.Client, len(operatorRemotes))
	for namespace, client := range operatorRemotes {
		out[namespace] = client
	}
	return out
}

// skillCacheEntry is a cached per-remote skills listing.
type skillCacheEntry struct {
	skills []mcp.Skill
	expiry time.Time
}

// skillCache caches skills/list results per remote (operator remotes
// globally, user remotes per user) for a minute, so repeat chats don't pay
// the round trips again.
var skillCache sync.Map

// listRemoteSkillLines lists every reachable remote's skills as prompt lines,
// prefixed with the remote's namespace so same-named skills stay
// distinguishable. Each remote is fetched in parallel with its OWN
// one-second budget: skills are additive context, so one slow or dead remote
// must never starve the others. Failures warn and skip.
func listRemoteSkillLines(ctx context.Context, user *model.User) []string {
	type target struct {
		cacheKey  string
		namespace string
		client    *mcp.Client
	}
	var targets []target

	operatorRemotes := snapshotOperatorRemotes()
	operatorNamespaces := make([]string, 0, len(operatorRemotes))
	for namespace := range operatorRemotes {
		operatorNamespaces = append(operatorNamespaces, namespace)
	}
	sort.Strings(operatorNamespaces)
	for _, namespace := range operatorNamespaces {
		targets = append(targets, target{
			cacheKey:  "operator/" + namespace,
			namespace: namespace,
			client:    operatorRemotes[namespace],
		})
	}
	if user != nil {
		if servers, err := NewRemoteServerProvider(user).enabledServers(); err == nil {
			for _, server := range servers {
				if client, err := remoteManager.getOrCreateClient(server); err == nil {
					targets = append(targets, target{
						cacheKey:  "user/" + user.Id + "/" + server.Namespace,
						namespace: server.Namespace,
						client:    client,
					})
				}
			}
		}
	}

	type remoteResult struct {
		namespace string
		skills    []mcp.Skill
	}
	var (
		mu      sync.Mutex
		results []remoteResult
		wg      sync.WaitGroup
	)

	for _, t := range targets {
		if cached, ok := skillCache.Load(t.cacheKey); ok {
			if entry := cached.(skillCacheEntry); time.Now().Before(entry.expiry) {
				mu.Lock()
				results = append(results, remoteResult{t.namespace, entry.skills})
				mu.Unlock()
				continue
			}
		}
		wg.Add(1)
		go func(t target) {
			defer wg.Done()
			remoteCtx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			if err := t.client.Initialize(remoteCtx); err != nil {
				log.WithGroup("mcp").Warn("skills prompt: remote not reachable, skipping",
					"namespace", t.namespace, "error", err)
				return
			}
			skills, err := t.client.ListSkills(remoteCtx)
			if err != nil {
				log.WithGroup("mcp").Warn("skills prompt: remote skills/list failed, skipping",
					"namespace", t.namespace, "error", err)
				return
			}
			skillCache.Store(t.cacheKey, skillCacheEntry{skills: skills, expiry: time.Now().Add(time.Minute)})
			mu.Lock()
			results = append(results, remoteResult{t.namespace, skills})
			mu.Unlock()
		}(t)
	}
	wg.Wait()

	sort.Slice(results, func(i, j int) bool { return results[i].namespace < results[j].namespace })

	var lines []string
	for _, res := range results {
		for _, skill := range res.skills {
			lines = append(lines, remoteSkillLine(res.namespace, skill))
		}
	}
	return lines
}

// remoteSkillLine renders one remote skill as "- namespace/name:
// description (uri)". The name comes from the remote's own frontmatter; the
// namespace prefix keeps it distinguishable from same-named local or other
// remote skills.
func remoteSkillLine(namespace string, skill mcp.Skill) string {
	name, _ := skill.Frontmatter["name"].(string)
	if name == "" {
		name = strings.TrimSuffix(strings.TrimPrefix(skill.URI, "skill://"), "/SKILL.md")
	}
	if namespace != "" {
		name = namespace + "/" + name
	}
	if description, _ := skill.Frontmatter["description"].(string); description != "" {
		return "- " + name + ": " + description + " (" + skill.URI + ")"
	}
	return "- " + name + " (" + skill.URI + ")"
}

// BuildSkillsPrompt returns a skills section to append to the system prompt:
// this knot instance's own skills (from the database, so zone restrictions and
// group-based access control apply) plus the skills of every reachable remote
// MCP server, one line each as "name: description (uri)". retrieval names how
// the calling surface reads a skill back and is appended to the header when
// non-empty, e.g. "Call the lmchatkit__get_skill tool with the skill URI".
// Returns an empty string when no skills are available.
func BuildSkillsPrompt(ctx context.Context, user *model.User, retrieval string) string {
	skills := GetAccessibleSkills(user)

	var lines []string
	for _, skill := range skills {
		if skill.Description != "" {
			lines = append(lines, fmt.Sprintf("- %s: %s (%s)", skill.Name, skill.Description, skillEntryURI(skill.Name)))
		} else {
			lines = append(lines, fmt.Sprintf("- %s (%s)", skill.Name, skillEntryURI(skill.Name)))
		}
	}
	if ctx != nil {
		lines = append(lines, listRemoteSkillLines(ctx, user)...)
	}
	if len(lines) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\nThe following skills are available.")
	if retrieval != "" {
		sb.WriteString(" " + retrieval)
	}
	sb.WriteString("\n")
	for _, line := range lines {
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return sb.String()
}
