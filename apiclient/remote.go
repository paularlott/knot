package apiclient

import (
	"errors"

	"github.com/paularlott/knot/database/model"
)

type TemplateVarValues struct {
	Id        string `json:"templatevar_id"`
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
	Value     string `json:"value"`
}

func (c *ApiClient) GetTemplateVarValues() ([]*model.TemplateVar, int, error) {
	response := &[]TemplateVarValues{}

	code, err := c.httpClient.Get("/api/v1/remote/templatevars/values", response)
	if err != nil {
		return nil, code, err
	}

	templateVars := make([]*model.TemplateVar, len(*response))
	for i, templateVar := range *response {
		templateVars[i] = &model.TemplateVar{
			Id:        templateVar.Id,
			Name:      templateVar.Name,
			Protected: templateVar.Protected,
			Value:     templateVar.Value,
		}
	}

	return templateVars, code, nil
}

func (c *ApiClient) GetTemplateObject(templateId string) (*model.Template, int, error) {
	response := &TemplateDetails{}

	code, err := c.httpClient.Get("/api/v1/remote/templates/"+templateId, response)
	if err != nil {
		return nil, code, err
	}

	template := &model.Template{
		Id:          templateId,
		Name:        response.Name,
		Description: response.Description,
		Job:         response.Job,
		Volumes:     response.Volumes,
		Groups:      response.Groups,
		Hash:        response.Hash,
	}

	return template, code, nil
}

func (c *ApiClient) RemoteGetUser(userId string) (*model.User, error) {
	response := UserResponse{}

	code, err := c.httpClient.Get("/api/v1/remote/users/"+userId, &response)
	if err != nil {
		if code == 404 {
			return nil, errors.New("user not found")
		} else {
			return nil, err
		}
	}

	user := &model.User{
		Id:              response.Id,
		Username:        response.Username,
		Email:           response.Email,
		ServicePassword: response.ServicePassword,
		SSHPublicKey:    response.SSHPublicKey,
		Roles:           response.Roles,
		Groups:          response.Groups,
		Active:          response.Active,
		MaxSpaces:       response.MaxSpaces,
		MaxDiskSpace:    response.MaxDiskSpace,
		PreferredShell:  response.PreferredShell,
		Timezone:        response.Timezone,
		LastLoginAt:     response.LastLoginAt,
		CreatedAt:       response.CreatedAt,
		UpdatedAt:       response.UpdatedAt,
	}

	return user, nil
}

func (c *ApiClient) RemoteUpdateSpace(space *model.Space) (int, error) {
	request := &UpdateSpaceRequest{
		UserId:      space.UserId,
		TemplateId:  space.TemplateId,
		Name:        space.Name,
		AltNames:    space.AltNames,
		AgentURL:    space.AgentURL,
		Shell:       space.Shell,
		VolumeSizes: space.VolumeSizes,
		IsDeployed:  space.IsDeployed,
		Location:    space.Location,
	}

	code, err := c.httpClient.Put("/api/v1/remote/spaces/"+space.Id, request, nil, 200)
	if err != nil {
		return code, err
	}

	return code, nil
}

func (c *ApiClient) RemoteDeleteSpace(spaceId string) (int, error) {
	return c.httpClient.Delete("/api/v1/remote/spaces/"+spaceId, nil, nil, 200)
}

func (c *ApiClient) RemoteUpdateVolume(volume *model.Volume) (int, error) {
	request := VolumeDefinition{
		Name:       volume.Name,
		Definition: volume.Definition,
		Location:   volume.Location,
		Active:     volume.Active,
	}

	return c.httpClient.Put("/api/v1/remote/volumes/"+volume.Id, request, nil, 200)
}
