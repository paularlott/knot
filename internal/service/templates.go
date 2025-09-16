package service

import (
	"errors"
	"fmt"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/validate"
)

type TemplateService struct{}

type TemplateListOptions struct {
	User                 *model.User
	IncludeInactive      bool
	IncludeDeleted       bool
	CheckPermissions     bool
	CheckZoneRestriction bool
}

var templateService *TemplateService

func GetTemplateService() *TemplateService {
	if templateService == nil {
		templateService = &TemplateService{}
	}
	return templateService
}

// ListTemplates returns a filtered list of templates based on the provided options
func (s *TemplateService) ListTemplates(opts TemplateListOptions) ([]*model.Template, error) {
	db := database.GetInstance()
	templates, err := db.GetTemplates()
	if err != nil {
		return nil, fmt.Errorf("failed to get templates: %v", err)
	}

	cfg := config.GetServerConfig()
	var result []*model.Template

	for _, template := range templates {
		// Skip deleted templates unless explicitly requested
		if template.IsDeleted && !opts.IncludeDeleted {
			continue
		}

		// Skip inactive templates unless explicitly requested
		if !template.Active && !opts.IncludeInactive {
			continue
		}

		// Check user permissions if required
		if opts.CheckPermissions && opts.User != nil {
			// If template has groups and user doesn't have any of them, skip
			if len(template.Groups) > 0 && !opts.User.HasAnyGroup(&template.Groups) {
				continue
			}
		}

		// Check zone restrictions if required
		if opts.CheckZoneRestriction && !template.IsValidForZone(cfg.Zone) {
			continue
		}

		result = append(result, template)
	}

	return result, nil
}

// GetTemplate retrieves a single template by ID
func (s *TemplateService) GetTemplate(templateId string) (*model.Template, error) {
	if !validate.UUID(templateId) {
		return nil, fmt.Errorf("invalid template ID")
	}

	db := database.GetInstance()
	return db.GetTemplate(templateId)
}

// CreateTemplate creates a new template with validation
func (s *TemplateService) CreateTemplate(template *model.Template, user *model.User) error {
	// Validate permissions
	if !user.HasPermission(model.PermissionManageTemplates) {
		return fmt.Errorf("no permission to manage templates")
	}

	// Validate input
	if err := s.validateTemplateInput(template.Name, template.Platform, template.Job, template.Volumes, int(template.ComputeUnits), int(template.StorageUnits), int(template.MaxUptime), template.MaxUptimeUnit, template.ScheduleEnabled, &template.Schedule, template.CustomFields); err != nil {
		return err
	}

	// Validate groups exist
	if err := s.validateGroups(template.Groups); err != nil {
		return err
	}

	// Set user ID
	template.CreatedUserId = user.Id
	template.UpdatedUserId = user.Id

	// Save to database
	db := database.GetInstance()
	if err := db.SaveTemplate(template, nil); err != nil {
		return fmt.Errorf("failed to save template: %v", err)
	}

	// Gossip the template
	GetTransport().GossipTemplate(template)

	return nil
}

// UpdateTemplate updates an existing template with validation
func (s *TemplateService) UpdateTemplate(template *model.Template, user *model.User) error {
	// Validate permissions
	if !user.HasPermission(model.PermissionManageTemplates) {
		return fmt.Errorf("no permission to manage templates")
	}

	// Get existing template to check if it exists and is manageable
	existing, err := s.GetTemplate(template.Id)
	if err != nil {
		return fmt.Errorf("template not found: %v", err)
	}

	if existing.IsManaged {
		return fmt.Errorf("cannot update managed template")
	}

	// Validate input
	if err := s.validateTemplateInput(template.Name, template.Platform, template.Job, template.Volumes, int(template.ComputeUnits), int(template.StorageUnits), int(template.MaxUptime), template.MaxUptimeUnit, template.ScheduleEnabled, &template.Schedule, template.CustomFields); err != nil {
		return err
	}

	// Validate groups exist
	if err := s.validateGroups(template.Groups); err != nil {
		return err
	}

	// Update metadata
	template.UpdatedUserId = user.Id
	template.UpdatedAt = hlc.Now()
	template.UpdateHash()

	// Save to database
	db := database.GetInstance()
	if err := db.SaveTemplate(template, nil); err != nil {
		return fmt.Errorf("failed to save template: %v", err)
	}

	// Gossip the template
	GetTransport().GossipTemplate(template)

	return nil
}

// DeleteTemplate marks a template as deleted with validation
func (s *TemplateService) DeleteTemplate(templateId string, user *model.User) error {
	// Validate permissions
	if !user.HasPermission(model.PermissionManageTemplates) {
		return fmt.Errorf("no permission to manage templates")
	}

	// Get template
	template, err := s.GetTemplate(templateId)
	if err != nil {
		return fmt.Errorf("template not found: %v", err)
	}

	// Check if template is in use
	db := database.GetInstance()
	spaces, err := db.GetSpacesByTemplateId(templateId)
	if err != nil {
		return fmt.Errorf("failed to check template usage: %v", err)
	}

	activeSpaces := 0
	for _, space := range spaces {
		if !space.IsDeleted {
			activeSpaces++
		}
	}

	if activeSpaces > 0 {
		return fmt.Errorf("template is in use by spaces")
	}

	// Mark as deleted
	template.IsDeleted = true
	template.UpdatedAt = hlc.Now()
	template.UpdatedUserId = user.Id

	if err := db.SaveTemplate(template, []string{"IsDeleted", "UpdatedAt", "UpdatedUserId"}); err != nil {
		if errors.Is(err, database.ErrTemplateInUse) {
			return fmt.Errorf("template is in use")
		}
		return fmt.Errorf("failed to delete template: %v", err)
	}

	// Gossip the template
	GetTransport().GossipTemplate(template)

	return nil
}

// GetTemplateUsage returns usage statistics for a template
func (s *TemplateService) GetTemplateUsage(templateId string) (total int, deployed int, err error) {
	db := database.GetInstance()
	spaces, err := db.GetSpacesByTemplateId(templateId)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get template usage: %v", err)
	}

	for _, space := range spaces {
		if !space.IsDeleted {
			total++
			if space.IsDeployed || space.IsPending {
				deployed++
			}
		}
	}

	return total, deployed, nil
}

// validateTemplateInput validates common template input fields
func (s *TemplateService) validateTemplateInput(name, platform, job, volumes string, computeUnits, storageUnits, maxUptime int, maxUptimeUnit string, scheduleEnabled bool, schedule *[]model.TemplateScheduleDays, customFields []model.TemplateCustomField) error {
	if !validate.Required(name) || !validate.MaxLength(name, 64) {
		return fmt.Errorf("invalid template name given")
	}

	if !validate.OneOf(platform, []string{model.PlatformManual, model.PlatformDocker, model.PlatformPodman, model.PlatformNomad}) {
		return fmt.Errorf("invalid platform")
	}

	if platform != model.PlatformManual {
		if !validate.Required(job) || !validate.MaxLength(job, 10*1024*1024) {
			return fmt.Errorf("job is required and must be less than 10MB")
		}
	}

	if !validate.MaxLength(volumes, 10*1024*1024) {
		return fmt.Errorf("volumes must be less than 10MB")
	}

	if !validate.IsPositiveNumber(computeUnits) {
		return fmt.Errorf("compute units must be a positive number")
	}

	if !validate.IsPositiveNumber(storageUnits) {
		return fmt.Errorf("storage units must be a positive number")
	}

	if !validate.IsPositiveNumber(maxUptime) || !validate.OneOf(maxUptimeUnit, []string{"disabled", "minute", "hour", "day"}) {
		return fmt.Errorf("max uptime must be a positive number and unit must be one of disabled, minute, hour, day")
	}

	if scheduleEnabled && schedule != nil {
		if len(*schedule) != 7 {
			return fmt.Errorf("schedule must have 7 days")
		}
		for _, day := range *schedule {
			if !validate.IsTime(day.From) || !validate.IsTime(day.To) {
				return fmt.Errorf("invalid time format")
			}
		}
	}

	for _, field := range customFields {
		if !validate.Required(field.Name) || !validate.VarName(field.Name) {
			return fmt.Errorf("invalid custom field name given")
		}
		if !validate.MaxLength(field.Description, 256) {
			return fmt.Errorf("custom field description must be less than 256 characters")
		}
	}

	return nil
}

// validateGroups validates that all provided group IDs exist
func (s *TemplateService) validateGroups(groups []string) error {
	db := database.GetInstance()
	for _, groupId := range groups {
		if _, err := db.GetGroup(groupId); err != nil {
			return fmt.Errorf("group %s does not exist", groupId)
		}
	}
	return nil
}
