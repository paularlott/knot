package service

import (
	"fmt"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/util/validate"
)

type SpaceService struct{}

type SpaceListOptions struct {
	User           *model.User
	UserId         string // Filter by specific user ID
	IncludeDeleted bool
	CheckZone      bool
}

var spaceService *SpaceService

func GetSpaceService() *SpaceService {
	if spaceService == nil {
		spaceService = &SpaceService{}
	}
	return spaceService
}

// ListSpaces returns a filtered list of spaces based on the provided options
func (s *SpaceService) ListSpaces(opts SpaceListOptions) ([]*model.Space, error) {
	db := database.GetInstance()

	var spaces []*model.Space
	var err error

	if opts.UserId != "" {
		spaces, err = db.GetSpacesForUser(opts.UserId)
	} else {
		spaces, err = db.GetSpaces()
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get spaces: %v", err)
	}

	cfg := config.GetServerConfig()
	var result []*model.Space

	for _, space := range spaces {
		// Skip deleted spaces unless explicitly requested
		if space.IsDeleted && !opts.IncludeDeleted {
			continue
		}

		// Check zone restrictions if required
		if opts.CheckZone && space.Zone != "" && space.Zone != cfg.Zone {
			continue
		}

		result = append(result, space)
	}

	return result, nil
}

// GetSpace retrieves a single space by ID with permission checks
func (s *SpaceService) GetSpace(spaceId string, user *model.User) (*model.Space, error) {
	if !validate.UUID(spaceId) {
		return nil, fmt.Errorf("invalid space ID")
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		return nil, fmt.Errorf("space not found: %v", err)
	}

	// Check permissions - user can access their own spaces or if they have manage permission
	if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("space not found")
	}

	return space, nil
}

// CreateSpace creates a new space with validation and quota checks
func (s *SpaceService) CreateSpace(space *model.Space, user *model.User) error {
	// Validate permissions
	if !user.HasPermission(model.PermissionUseSpaces) && !user.HasPermission(model.PermissionManageSpaces) {
		return fmt.Errorf("no permission to manage or use spaces")
	}

	// Validate input
	if err := s.validateSpaceInput(space.Name, space.Description, space.Shell, space.AltNames); err != nil {
		return err
	}

	db := database.GetInstance()
	cfg := config.GetServerConfig()

	// Validate template
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil || template == nil || template.IsDeleted || !template.Active {
		return fmt.Errorf("invalid template given for new space")
	}

	// Check template group permissions
	if !user.HasPermission(model.PermissionManageTemplates) && len(template.Groups) > 0 && !user.HasAnyGroup(&template.Groups) {
		return fmt.Errorf("no permission to use this template")
	}

	// Check if space creation is disabled
	if cfg.DisableSpaceCreate {
		return fmt.Errorf("space creation is disabled")
	}

	// Check quotas if not on leaf node
	if !cfg.LeafNode {
		if err := s.CheckUserQuotas(user, template); err != nil {
			return err
		}
	}

	// Set user ID and zone
	space.UserId = user.Id
	space.Zone = cfg.Zone

	// Save to database
	if err := db.SaveSpace(space, nil); err != nil {
		return fmt.Errorf("failed to save space: %v", err)
	}

	// Gossip the space and notify SSE clients
	GetTransport().GossipSpace(space)
	sse.PublishSpaceCreated(space.Id, space.UserId)

	return nil
}

// UpdateSpace updates an existing space with validation
func (s *SpaceService) UpdateSpace(space *model.Space, user *model.User) error {
	// Validate permissions
	if !user.HasPermission(model.PermissionUseSpaces) && !user.HasPermission(model.PermissionManageSpaces) {
		return fmt.Errorf("no permission to manage or use spaces")
	}

	// Get existing space to check ownership and zone
	existing, err := s.GetSpace(space.Id, user)
	if err != nil {
		return err
	}

	cfg := config.GetServerConfig()
	if existing.Zone != "" && existing.Zone != cfg.Zone {
		return fmt.Errorf("space not on this server")
	}

	// Validate input
	if err := s.validateSpaceInput(space.Name, space.Description, space.Shell, space.AltNames); err != nil {
		return err
	}

	// Validate template if changed
	if space.TemplateId != existing.TemplateId {
		db := database.GetInstance()
		template, err := db.GetTemplate(space.TemplateId)
		if err != nil || template == nil {
			return fmt.Errorf("unknown template")
		}
	}

	// Update metadata
	space.UpdatedAt = hlc.Now()

	// Save to database
	db := database.GetInstance()
	if err := db.SaveSpace(space, []string{"Name", "Description", "TemplateId", "Shell", "IconURL", "AltNames", "CustomFields", "UpdatedAt"}); err != nil {
		return fmt.Errorf("failed to save space: %v", err)
	}

	// Gossip the space and notify SSE clients
	GetTransport().GossipSpace(space)
	sse.PublishSpaceUpdated(space.Id, space.UserId)

	return nil
}

// GetSpaceCustomField retrieves a single custom field value from a space
func (s *SpaceService) GetSpaceCustomField(spaceId string, fieldName string, user *model.User) (string, error) {
	// Validate permissions
	if !user.HasPermission(model.PermissionUseSpaces) && !user.HasPermission(model.PermissionManageSpaces) {
		return "", fmt.Errorf("no permission to manage or use spaces")
	}

	// Get existing space to check ownership and zone
	space, err := s.GetSpace(spaceId, user)
	if err != nil {
		return "", err
	}

	cfg := config.GetServerConfig()
	if space.Zone != "" && space.Zone != cfg.Zone {
		return "", fmt.Errorf("space not on this server")
	}

	// Get the template to check if field is defined
	db := database.GetInstance()
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		return "", fmt.Errorf("failed to get template: %v", err)
	}

	// Check if field is defined in template
	fieldDefined := false
	for _, field := range template.CustomFields {
		if field.Name == fieldName {
			fieldDefined = true
			break
		}
	}

	if !fieldDefined {
		return "", fmt.Errorf("custom field '%s' not defined in template", fieldName)
	}

	// Find and return the custom field value from space (return empty string if not set)
	for _, field := range space.CustomFields {
		if field.Name == fieldName {
			return field.Value, nil
		}
	}

	// Field is defined in template but not set in space - return empty string
	return "", nil
}

// SetSpaceCustomField sets or updates a single custom field on a space
func (s *SpaceService) SetSpaceCustomField(spaceId string, fieldName string, fieldValue string, user *model.User) error {
	// Validate permissions
	if !user.HasPermission(model.PermissionUseSpaces) && !user.HasPermission(model.PermissionManageSpaces) {
		return fmt.Errorf("no permission to manage or use spaces")
	}

	// Get existing space to check ownership and zone
	space, err := s.GetSpace(spaceId, user)
	if err != nil {
		return err
	}

	cfg := config.GetServerConfig()
	if space.Zone != "" && space.Zone != cfg.Zone {
		return fmt.Errorf("space not on this server")
	}

	// Get the template to check if field is defined
	db := database.GetInstance()
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		return fmt.Errorf("failed to get template: %v", err)
	}

	// Check if field is defined in template
	fieldDefined := false
	for _, field := range template.CustomFields {
		if field.Name == fieldName {
			fieldDefined = true
			break
		}
	}

	if !fieldDefined {
		return fmt.Errorf("custom field '%s' not defined in template", fieldName)
	}

	// Update or add the custom field
	fieldFound := false
	for i, field := range space.CustomFields {
		if field.Name == fieldName {
			space.CustomFields[i].Value = fieldValue
			fieldFound = true
			break
		}
	}

	if !fieldFound {
		space.CustomFields = append(space.CustomFields, model.SpaceCustomField{
			Name:  fieldName,
			Value: fieldValue,
		})
	}

	// Update metadata
	space.UpdatedAt = hlc.Now()

	// Save to database
	if err := db.SaveSpace(space, []string{"CustomFields", "UpdatedAt"}); err != nil {
		return fmt.Errorf("failed to save space: %v", err)
	}

	// Gossip the space and notify SSE clients
	GetTransport().GossipSpace(space)
	sse.PublishSpaceUpdated(space.Id, space.UserId)

	return nil
}

// DeleteSpace marks a space as deleted with validation
func (s *SpaceService) DeleteSpace(spaceId string, user *model.User) error {
	// Validate permissions
	if !user.HasPermission(model.PermissionUseSpaces) && !user.HasPermission(model.PermissionManageSpaces) {
		return fmt.Errorf("no permission to manage or use spaces")
	}

	// Get space
	space, err := s.GetSpace(spaceId, user)
	if err != nil {
		return err
	}

	cfg := config.GetServerConfig()
	if space.Zone != "" && space.Zone != cfg.Zone {
		return fmt.Errorf("space not on this server")
	}

	// Mark as deleted
	space.IsDeleted = true
	space.Name = space.Id
	space.UpdatedAt = hlc.Now()

	// Save to database
	db := database.GetInstance()
	if err := db.SaveSpace(space, []string{"IsDeleted", "Name", "UpdatedAt"}); err != nil {
		return fmt.Errorf("failed to delete space: %v", err)
	}

	// Gossip the space and notify SSE clients
	GetTransport().GossipSpace(space)
	sse.PublishSpaceDeleted(space.Id, space.UserId)

	return nil
}

// validateSpaceInput validates common space input fields
func (s *SpaceService) validateSpaceInput(name, description, shell string, altNames []string) error {
	if !validate.Name(name) {
		return fmt.Errorf("invalid name given for space")
	}

	if !validate.MaxLength(description, 1024) {
		return fmt.Errorf("description too long")
	}

	if !validate.OneOf(shell, []string{"bash", "zsh", "fish", "sh"}) {
		return fmt.Errorf("invalid shell given for space")
	}

	for _, altName := range altNames {
		if !validate.Name(altName) {
			return fmt.Errorf("invalid alt name given for space")
		}
	}

	return nil
}

// checkUserQuotas validates user quotas for space creation
func (s *SpaceService) CheckUserQuotas(user *model.User, template *model.Template) error {
	usage, err := database.GetUserUsage(user.Id, "")
	if err != nil {
		return fmt.Errorf("failed to check user usage: %v", err)
	}

	userQuota, err := database.GetUserQuota(user)
	if err != nil {
		return fmt.Errorf("failed to check user quota: %v", err)
	}

	if userQuota.MaxSpaces > 0 && uint32(usage.NumberSpaces+1) > userQuota.MaxSpaces {
		return fmt.Errorf("space quota exceeded")
	}

	if userQuota.StorageUnits > 0 && uint32(usage.StorageUnits+template.StorageUnits) > userQuota.StorageUnits {
		return fmt.Errorf("storage unit quota exceeded")
	}

	return nil
}
