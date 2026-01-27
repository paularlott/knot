package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/util"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

func HandleGetSkills(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	filterUserId := r.URL.Query().Get("user_id")
	allZones := r.URL.Query().Get("all_zones") == "true"

	if filterUserId != "" {
		if filterUserId == user.Id {
			if !user.HasPermission(model.PermissionManageOwnSkills) {
				rest.WriteResponse(http.StatusOK, w, r, apiclient.SkillList{Count: 0, Skills: []apiclient.SkillInfo{}})
				return
			}
		} else {
			if !user.HasPermission(model.PermissionManageGlobalSkills) {
				rest.WriteResponse(http.StatusOK, w, r, apiclient.SkillList{Count: 0, Skills: []apiclient.SkillInfo{}})
				return
			}
		}
	} else {
		canSeeGlobals := user.HasPermission(model.PermissionManageGlobalSkills)
		canSeeOwn := user.HasPermission(model.PermissionManageOwnSkills)

		if !canSeeGlobals && !canSeeOwn {
			rest.WriteResponse(http.StatusOK, w, r, apiclient.SkillList{Count: 0, Skills: []apiclient.SkillInfo{}})
			return
		}

		if canSeeOwn && !canSeeGlobals {
			filterUserId = user.Id
		}
	}

	skillService := service.GetSkillService()
	skills, err := skillService.ListSkills(service.SkillListOptions{
		FilterUserId:         filterUserId,
		User:                 user,
		IncludeDeleted:       false,
		CheckZoneRestriction: !allZones,
	})
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	response := apiclient.SkillList{
		Count:  0,
		Skills: []apiclient.SkillInfo{},
	}

	seenSkills := make(map[string]bool)

	for _, skill := range skills {
		response.Skills = append(response.Skills, apiclient.SkillInfo{
			Id:          skill.Id,
			UserId:      skill.UserId,
			Name:        skill.Name,
			Description: skill.Description,
			Groups:      skill.Groups,
			Zones:       skill.Zones,
			Active:      skill.Active,
			IsManaged:   skill.IsManaged,
		})
		seenSkills[skill.Id] = true
		response.Count++
	}

	if filterUserId == "" && user.HasPermission(model.PermissionManageOwnSkills) {
		ownSkills, err := skillService.ListSkills(service.SkillListOptions{
			FilterUserId:         user.Id,
			User:                 user,
			IncludeDeleted:       false,
			CheckZoneRestriction: !allZones,
		})
		if err == nil {
			for _, skill := range ownSkills {
				if !seenSkills[skill.Id] {
					response.Skills = append(response.Skills, apiclient.SkillInfo{
						Id:          skill.Id,
						UserId:      skill.UserId,
						Name:        skill.Name,
						Description: skill.Description,
						Groups:      skill.Groups,
						Zones:       skill.Zones,
						Active:      skill.Active,
						IsManaged:   skill.IsManaged,
					})
					seenSkills[skill.Id] = true
					response.Count++
				}
			}
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}

func HandleGetSkill(w http.ResponseWriter, r *http.Request) {
	skillIdOrName := r.PathValue("skill_id")
	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	var skill *model.Skill
	var err error

	if validate.UUID(skillIdOrName) {
		skill, err = db.GetSkill(skillIdOrName)
		if err != nil || skill.IsDeleted {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Skill not found"})
			return
		}

		if skill.IsUserSkill() {
			if skill.UserId != user.Id && !user.HasPermission(model.PermissionManageGlobalSkills) {
				rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Skill not found"})
				return
			}
			if skill.UserId == user.Id && !user.HasPermission(model.PermissionManageOwnSkills) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to view this skill"})
				return
			}
		} else {
			if !user.HasPermission(model.PermissionManageGlobalSkills) {
				if len(skill.Groups) > 0 && !user.HasAnyGroup(&skill.Groups) {
					rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Skill not found"})
					return
				}
			}
		}
	} else {
		skill, err = service.ResolveSkillByName(skillIdOrName, user.Id)
		if err != nil {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Skill not found"})
			return
		}

		if !service.CanUserAccessSkill(user, skill) {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Skill not found"})
			return
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, apiclient.SkillDetails{
		Id:          skill.Id,
		UserId:      skill.UserId,
		Name:        skill.Name,
		Description: skill.Description,
		Content:     skill.Content,
		Groups:      skill.Groups,
		Zones:       skill.Zones,
		Active:      skill.Active,
		IsManaged:   skill.IsManaged,
	})
}

func HandleCreateSkill(w http.ResponseWriter, r *http.Request) {
	request := apiclient.SkillCreateRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if len(request.Content) > 4*1024*1024 {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Skill content exceeds 4MB limit"})
		return
	}

	fm, err := util.ParseSkillFrontmatter(request.Content)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: fmt.Sprintf("Invalid frontmatter: %v", err)})
		return
	}

	user := r.Context().Value("user").(*model.User)
	cfg := config.GetServerConfig()
	db := database.GetInstance()

	ownerUserId := request.UserId
	if ownerUserId == "current" {
		ownerUserId = user.Id
	}
	isUserSkill := ownerUserId != ""

	if !cfg.LeafNode {
		if isUserSkill {
			if ownerUserId != user.Id && !user.HasPermission(model.PermissionManageGlobalSkills) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to create skills for other users"})
				return
			}
			if ownerUserId == user.Id && !user.HasPermission(model.PermissionManageOwnSkills) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to create own skills"})
				return
			}
			if len(request.Groups) > 0 {
				rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "User skills cannot have groups"})
				return
			}
		} else {
			if !user.HasPermission(model.PermissionManageGlobalSkills) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to create global skills"})
				return
			}
		}
	} else {
		if isUserSkill && len(request.Groups) > 0 {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "User skills cannot have groups"})
			return
		}
	}

	skill := model.NewSkill(
		fm.Name,
		fm.Description,
		request.Content,
		request.Groups,
		request.Zones,
		ownerUserId,
		user.Id,
	)
	skill.Active = request.Active

	err = db.SaveSkill(skill, nil)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipSkill(skill)
	sse.PublishSkillsChanged(skill.Id)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventSkillCreate,
		fmt.Sprintf("Created skill %s", skill.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"skill_id":        skill.Id,
			"skill_name":      skill.Name,
			"is_user_skill":   isUserSkill,
		},
	)

	rest.WriteResponse(http.StatusCreated, w, r, &apiclient.SkillCreateResponse{
		Status: true,
		Id:     skill.Id,
	})
}

func HandleUpdateSkill(w http.ResponseWriter, r *http.Request) {
	skillIdOrName := r.PathValue("skill_id")
	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	var skill *model.Skill
	var err error

	if validate.UUID(skillIdOrName) {
		skill, err = db.GetSkill(skillIdOrName)
	} else {
		skill, err = service.ResolveSkillByName(skillIdOrName, user.Id)
	}

	if err != nil || skill.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Skill not found"})
		return
	}

	request := apiclient.SkillUpdateRequest{}
	err = rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if len(request.Content) > 4*1024*1024 {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Skill content exceeds 4MB limit"})
		return
	}

	fm, err := util.ParseSkillFrontmatter(request.Content)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: fmt.Sprintf("Invalid frontmatter: %v", err)})
		return
	}

	cfg := config.GetServerConfig()

	if skill.IsManaged {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "Cannot edit managed skill"})
		return
	}

	if !cfg.LeafNode {
		if skill.IsUserSkill() {
			if skill.UserId != user.Id && !user.HasPermission(model.PermissionManageGlobalSkills) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to edit this skill"})
				return
			}
			if skill.UserId == user.Id && !user.HasPermission(model.PermissionManageOwnSkills) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to edit own skills"})
				return
			}
			if len(request.Groups) > 0 {
				rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "User skills cannot have groups"})
				return
			}
		} else {
			if !user.HasPermission(model.PermissionManageGlobalSkills) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to edit global skills"})
				return
			}
		}
	} else {
		if skill.IsUserSkill() && len(request.Groups) > 0 {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "User skills cannot have groups"})
			return
		}
	}

	skill.Name = fm.Name
	skill.Description = fm.Description
	skill.Content = request.Content
	skill.Groups = request.Groups
	skill.Zones = request.Zones
	skill.Active = request.Active
	skill.UpdatedUserId = user.Id
	skill.UpdatedAt = hlc.Now()

	err = db.SaveSkill(skill, nil)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipSkill(skill)
	sse.PublishSkillsChanged(skill.Id)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventSkillUpdate,
		fmt.Sprintf("Updated skill %s", skill.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"skill_id":        skill.Id,
			"skill_name":      skill.Name,
			"is_user_skill":   skill.IsUserSkill(),
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	skillIdOrName := r.PathValue("skill_id")
	user := r.Context().Value("user").(*model.User)
	cfg := config.GetServerConfig()
	db := database.GetInstance()

	var skill *model.Skill
	var err error

	if validate.UUID(skillIdOrName) {
		skill, err = db.GetSkill(skillIdOrName)
	} else {
		skill, err = service.ResolveSkillByName(skillIdOrName, user.Id)
	}

	if err != nil || skill.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Skill not found"})
		return
	}

	if skill.IsManaged {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "Cannot delete managed skill"})
		return
	}

	if !cfg.LeafNode {
		if skill.IsUserSkill() {
			if skill.UserId != user.Id && !user.HasPermission(model.PermissionManageGlobalSkills) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to delete this skill"})
				return
			}
			if skill.UserId == user.Id && !user.HasPermission(model.PermissionManageOwnSkills) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to delete own skills"})
				return
			}
		} else {
			if !user.HasPermission(model.PermissionManageGlobalSkills) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to delete global skills"})
				return
			}
		}
	}

	skillName := skill.Name
	skillId := skill.Id
	skill.Name = skill.Id
	skill.IsDeleted = true
	skill.UpdatedUserId = user.Id
	skill.UpdatedAt = hlc.Now()

	err = db.SaveSkill(skill, nil)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipSkill(skill)
	sse.PublishSkillsDeleted(skill.Id)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventSkillDelete,
		fmt.Sprintf("Deleted skill %s", skillName),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"skill_id":        skillId,
			"skill_name":      skillName,
			"is_user_skill":   skill.IsUserSkill(),
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleSearchSkills(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	query := strings.ToLower(r.URL.Query().Get("q"))
	allZones := r.URL.Query().Get("all_zones") == "true"

	if query == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Query parameter 'q' is required"})
		return
	}

	skillService := service.GetSkillService()

	globalSkills, _ := skillService.ListSkills(service.SkillListOptions{
		FilterUserId:         "",
		User:                 user,
		IncludeDeleted:       false,
		CheckZoneRestriction: !allZones,
	})

	ownSkills, _ := skillService.ListSkills(service.SkillListOptions{
		FilterUserId:         user.Id,
		User:                 user,
		IncludeDeleted:       false,
		CheckZoneRestriction: !allZones,
	})

	allSkills := append(globalSkills, ownSkills...)

	type scoredSkill struct {
		skill *model.Skill
		score float64
	}

	var scored []scoredSkill
	seenSkills := make(map[string]bool)

	for _, skill := range allSkills {
		if seenSkills[skill.Id] {
			continue
		}

		// Calculate fuzzy match score
		queryWords := strings.Fields(query)
		var score float64

		for _, word := range queryWords {
			nameLower := strings.ToLower(skill.Name)
			descLower := strings.ToLower(skill.Description)

			// Exact substring match gets highest score
			if strings.Contains(nameLower, word) {
				score += 1.0
			} else if strings.Contains(descLower, word) {
				score += 0.8
			} else {
				// Fuzzy match on name
				if fuzzyScore := fuzzyMatch(word, nameLower); fuzzyScore > 0.6 {
					score += fuzzyScore * 0.7
				} else if fuzzyScore := fuzzyMatch(word, descLower); fuzzyScore > 0.6 {
					score += fuzzyScore * 0.5
				}
			}
		}

		if score > 0 {
			scored = append(scored, scoredSkill{skill: skill, score: score})
			seenSkills[skill.Id] = true
		}
	}

	response := apiclient.SkillList{
		Count:  len(scored),
		Skills: []apiclient.SkillInfo{},
	}

	for _, s := range scored {
		response.Skills = append(response.Skills, apiclient.SkillInfo{
			Id:          s.skill.Id,
			UserId:      s.skill.UserId,
			Name:        s.skill.Name,
			Description: s.skill.Description,
			Groups:      s.skill.Groups,
			Zones:       s.skill.Zones,
			Active:      s.skill.Active,
			IsManaged:   s.skill.IsManaged,
		})
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}

func fuzzyMatch(query, target string) float64 {
	if len(query) == 0 || len(target) == 0 {
		return 0
	}
	distance := levenshteinDistance(query, target)
	maxLen := max(len(query), len(target))
	return 1.0 - float64(distance)/float64(maxLen)
}

func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	m := len(r1)
	n := len(r2)
	prev := make([]int, n+1)
	curr := make([]int, n+1)
	for j := 0; j <= n; j++ {
		prev[j] = j
	}
	for i := 1; i <= m; i++ {
		curr[0] = i
		for j := 1; j <= n; j++ {
			cost := 0
			if r1[i-1] != r2[j-1] {
				cost = 1
			}
			if prev[j]+1 < curr[j-1]+1 {
				if prev[j]+1 < prev[j-1]+cost {
					curr[j] = prev[j] + 1
				} else {
					curr[j] = prev[j-1] + cost
				}
			} else {
				if curr[j-1]+1 < prev[j-1]+cost {
					curr[j] = curr[j-1] + 1
				} else {
					curr[j] = prev[j-1] + cost
				}
			}
		}
		prev, curr = curr, prev
	}
	return prev[n]
}
