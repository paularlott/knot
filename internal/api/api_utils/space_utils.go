package api_utils

import (
	"fmt"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/validate"
)

// GetSpaceDetails returns detailed space information with permission checks
func GetSpaceDetails(spaceId string, user *model.User) (*apiclient.SpaceDefinition, error) {
	if spaceId == "" {
		return nil, fmt.Errorf("space_id is required")
	}

	if !validate.UUID(spaceId) {
		return nil, fmt.Errorf("Invalid space ID")
	}

	if !user.HasPermission(model.PermissionUseSpaces) {
		return nil, fmt.Errorf("No permission to access spaces")
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil {
		return nil, fmt.Errorf("Space not found: %v", err)
	}

	// Check ownership or management permissions
	if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("No permission to access this space")
	}

	// Format the creation date in the user's timezone
	var createdAtFormatted string
	if user.Timezone != "" {
		if loc, err := time.LoadLocation(user.Timezone); err == nil {
			createdAtFormatted = space.CreatedAt.In(loc).Format("2 / Jan / 2006 3:04:05pm")
		} else {
			createdAtFormatted = space.CreatedAt.UTC().Format("2 / Jan / 2006 3:04:05pm")
		}
	} else {
		createdAtFormatted = space.CreatedAt.UTC().Format("2 / Jan / 2006 3:04:05pm")
	}

	response := &apiclient.SpaceDefinition{
		UserId:             space.UserId,
		TemplateId:         space.TemplateId,
		Name:               space.Name,
		Description:        space.Description,
		Shell:              space.Shell,
		Zone:               space.Zone,
		AltNames:           space.AltNames,
		IsDeployed:         space.IsDeployed,
		IsPending:          space.IsPending,
		IsDeleting:         space.IsDeleting,
		VolumeData:         space.VolumeData,
		StartedAt:          space.StartedAt.UTC(),
		CreatedAt:          space.CreatedAt.UTC(),
		CreatedAtFormatted: createdAtFormatted,
		IconURL:            space.IconURL,
		CustomFields:       make([]apiclient.CustomFieldValue, len(space.CustomFields)),
	}

	for i, field := range space.CustomFields {
		response.CustomFields[i] = apiclient.CustomFieldValue{
			Name:  field.Name,
			Value: field.Value,
		}
	}

	return response, nil
}
