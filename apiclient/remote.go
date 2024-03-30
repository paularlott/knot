package apiclient

import "github.com/paularlott/knot/database/model"

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
