package scriptling

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

func GetSkillsLibrary(client *apiclient.ApiClient, userId string) *object.Library {
	builder := object.NewLibraryBuilder("knot.skill", "Knot skill management functions")

	builder.FunctionWithHelp("create", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return skillCreate(ctx, client, userId, kwargs, args...)
	}, "create(content, global=false, groups=[], zones=[]) - Create a new skill")

	builder.FunctionWithHelp("get", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return skillGet(ctx, client, args...)
	}, "get(name_or_id) - Get skill by name or UUID")

	builder.FunctionWithHelp("update", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return skillUpdate(ctx, client, kwargs, args...)
	}, "update(name_or_id, content=None, groups=None, zones=None) - Update skill")

	builder.FunctionWithHelp("delete", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return skillDelete(ctx, client, args...)
	}, "delete(name_or_id) - Delete skill")

	builder.FunctionWithHelp("list", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return skillList(ctx, client, userId, kwargs, args...)
	}, "list(owner=None) - List skills (filtered by permissions/groups/zones)")

	builder.FunctionWithHelp("search", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return skillSearch(ctx, client, args...)
	}, "search(query) - Fuzzy search skills by name/description")

	return builder.Build()
}

func skillCreate(ctx context.Context, client *apiclient.ApiClient, userId string, kwargs object.Kwargs, args ...object.Object) object.Object {
	if err := errors.MinArgs(args, 1); err != nil {
		return err
	}

	content, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("content", err)
	}

	global, _ := kwargs.GetBool("global", false)
	
	ownerUserId := userId
	if global {
		ownerUserId = ""
	}

	request := &apiclient.SkillCreateRequest{
		UserId:  ownerUserId,
		Content: content,
		Groups:  []string{},
		Zones:   []string{},
		Active:  true,
	}

	if groupsObj := kwargs.Get("groups"); groupsObj != nil {
		if groupsList, ok := groupsObj.(*object.List); ok {
			groups := make([]string, len(groupsList.Elements))
			for i, elem := range groupsList.Elements {
				if str, err := elem.AsString(); err == nil {
					groups[i] = str
				}
			}
			request.Groups = groups
		}
	}

	if zonesObj := kwargs.Get("zones"); zonesObj != nil {
		if zonesList, ok := zonesObj.(*object.List); ok {
			zones := make([]string, len(zonesList.Elements))
			for i, elem := range zonesList.Elements {
				if str, err := elem.AsString(); err == nil {
					zones[i] = str
				}
			}
			request.Zones = zones
		}
	}

	var response apiclient.SkillCreateResponse
	_, apiErr := client.Do(ctx, "POST", "api/skill", request, &response)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to create skill: %v", apiErr)}
	}

	return &object.String{Value: response.Id}
}

func skillGet(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	nameOrId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("name_or_id", err)
	}

	var skill apiclient.SkillDetails
	var apiErr error

	// Try as UUID first, then as name
	_, apiErr = client.Do(ctx, "GET", fmt.Sprintf("api/skill/%s", nameOrId), nil, &skill)
	if apiErr != nil {
		_, apiErr = client.Do(ctx, "GET", fmt.Sprintf("api/skill/name/%s", nameOrId), nil, &skill)
		if apiErr != nil {
			return &object.Error{Message: fmt.Sprintf("skill not found: %v", apiErr)}
		}
	}

	return skillToDict(&skill)
}

func skillUpdate(ctx context.Context, client *apiclient.ApiClient, kwargs object.Kwargs, args ...object.Object) object.Object {
	if err := errors.MinArgs(args, 1); err != nil {
		return err
	}

	nameOrId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("name_or_id", err)
	}

	// Get current skill
	var current apiclient.SkillDetails
	_, apiErr := client.Do(ctx, "GET", fmt.Sprintf("api/skill/%s", nameOrId), nil, &current)
	if apiErr != nil {
		_, apiErr = client.Do(ctx, "GET", fmt.Sprintf("api/skill/name/%s", nameOrId), nil, &current)
		if apiErr != nil {
			return &object.Error{Message: fmt.Sprintf("skill not found: %v", apiErr)}
		}
	}

	request := &apiclient.SkillUpdateRequest{
		Content: current.Content,
		Groups:  current.Groups,
		Zones:   current.Zones,
	}

	if content, errObj := kwargs.GetString("content", ""); errObj == nil && content != "" {
		request.Content = content
	}

	if groupsObj := kwargs.Get("groups"); groupsObj != nil {
		if groupsList, ok := groupsObj.(*object.List); ok {
			groups := make([]string, len(groupsList.Elements))
			for i, elem := range groupsList.Elements {
				if str, err := elem.AsString(); err == nil {
					groups[i] = str
				}
			}
			request.Groups = groups
		}
	}

	if zonesObj := kwargs.Get("zones"); zonesObj != nil {
		if zonesList, ok := zonesObj.(*object.List); ok {
			zones := make([]string, len(zonesList.Elements))
			for i, elem := range zonesList.Elements {
				if str, err := elem.AsString(); err == nil {
					zones[i] = str
				}
			}
			request.Zones = zones
		}
	}

	_, apiErr = client.Do(ctx, "PUT", fmt.Sprintf("api/skill/%s", current.Id), request, nil)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to update skill: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

func skillDelete(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	nameOrId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("name_or_id", err)
	}

	// Get skill to find UUID
	var skill apiclient.SkillDetails
	_, apiErr := client.Do(ctx, "GET", fmt.Sprintf("api/skill/%s", nameOrId), nil, &skill)
	if apiErr != nil {
		_, apiErr = client.Do(ctx, "GET", fmt.Sprintf("api/skill/name/%s", nameOrId), nil, &skill)
		if apiErr != nil {
			return &object.Error{Message: fmt.Sprintf("skill not found: %v", apiErr)}
		}
	}

	_, apiErr = client.Do(ctx, "DELETE", fmt.Sprintf("api/skill/%s", skill.Id), nil, nil)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to delete skill: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

func skillList(ctx context.Context, client *apiclient.ApiClient, userId string, kwargs object.Kwargs, args ...object.Object) object.Object {
	owner, _ := kwargs.GetString("owner", "")
	
	url := "api/skill?all_zones=true"
	if owner != "" {
		url = fmt.Sprintf("api/skill?user_id=%s&all_zones=true", owner)
	}

	var response apiclient.SkillList
	_, apiErr := client.Do(ctx, "GET", url, nil, &response)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to list skills: %v", apiErr)}
	}

	elements := make([]object.Object, len(response.Skills))
	for i, skill := range response.Skills {
		pairs := make(map[string]object.DictPair)
		pairs["id"] = object.DictPair{Key: &object.String{Value: "id"}, Value: &object.String{Value: skill.Id}}
		pairs["name"] = object.DictPair{Key: &object.String{Value: "name"}, Value: &object.String{Value: skill.Name}}
		pairs["description"] = object.DictPair{Key: &object.String{Value: "description"}, Value: &object.String{Value: skill.Description}}
		pairs["user_id"] = object.DictPair{Key: &object.String{Value: "user_id"}, Value: &object.String{Value: skill.UserId}}
		pairs["is_managed"] = object.DictPair{Key: &object.String{Value: "is_managed"}, Value: &object.Boolean{Value: skill.IsManaged}}
		elements[i] = &object.Dict{Pairs: pairs}
	}

	return &object.List{Elements: elements}
}

func skillSearch(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	query, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("query", err)
	}

	var response apiclient.SkillList
	_, apiErr := client.Do(ctx, "GET", fmt.Sprintf("api/skill/search?q=%s&all_zones=true", query), nil, &response)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to search skills: %v", apiErr)}
	}

	elements := make([]object.Object, len(response.Skills))
	for i, skill := range response.Skills {
		pairs := make(map[string]object.DictPair)
		pairs["id"] = object.DictPair{Key: &object.String{Value: "id"}, Value: &object.String{Value: skill.Id}}
		pairs["name"] = object.DictPair{Key: &object.String{Value: "name"}, Value: &object.String{Value: skill.Name}}
		pairs["description"] = object.DictPair{Key: &object.String{Value: "description"}, Value: &object.String{Value: skill.Description}}
		pairs["user_id"] = object.DictPair{Key: &object.String{Value: "user_id"}, Value: &object.String{Value: skill.UserId}}
		elements[i] = &object.Dict{Pairs: pairs}
	}

	return &object.List{Elements: elements}
}

func skillToDict(skill *apiclient.SkillDetails) object.Object {
	pairs := make(map[string]object.DictPair)
	pairs["id"] = object.DictPair{Key: &object.String{Value: "id"}, Value: &object.String{Value: skill.Id}}
	pairs["user_id"] = object.DictPair{Key: &object.String{Value: "user_id"}, Value: &object.String{Value: skill.UserId}}
	pairs["name"] = object.DictPair{Key: &object.String{Value: "name"}, Value: &object.String{Value: skill.Name}}
	pairs["description"] = object.DictPair{Key: &object.String{Value: "description"}, Value: &object.String{Value: skill.Description}}
	pairs["content"] = object.DictPair{Key: &object.String{Value: "content"}, Value: &object.String{Value: skill.Content}}
	pairs["is_managed"] = object.DictPair{Key: &object.String{Value: "is_managed"}, Value: &object.Boolean{Value: skill.IsManaged}}

	groupElements := make([]object.Object, len(skill.Groups))
	for i, group := range skill.Groups {
		groupElements[i] = &object.String{Value: group}
	}
	pairs["groups"] = object.DictPair{Key: &object.String{Value: "groups"}, Value: &object.List{Elements: groupElements}}

	zoneElements := make([]object.Object, len(skill.Zones))
	for i, zone := range skill.Zones {
		zoneElements[i] = &object.String{Value: zone}
	}
	pairs["zones"] = object.DictPair{Key: &object.String{Value: "zones"}, Value: &object.List{Elements: zoneElements}}

	return &object.Dict{Pairs: pairs}
}
