package service

import (
	"context"
	"fmt"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	scriptlingmcp "github.com/paularlott/scriptling/extlibs/mcp"
	"github.com/paularlott/scriptling/object"
)

func ExecuteEventScript(script *model.Script, eventParams map[string]object.Object, user *model.User, envelope *EventEnvelope) (string, error) {
	timeout := time.Duration(config.GetServerConfig().MCPToolTimeout) * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ctx = context.WithValue(ctx, "user", user)

	client := apiclient.NewMuxClient(user)

	env, _, cleanup, err := NewServerScriptlingEnv(client, ServerScriptlingOptions{
		User:          user,
		EventParams:   eventParams,
		EventEnvelope: envelope,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create event scriptling environment: %v", err)
	}
	defer cleanup()

	response, exitCode, err := scriptlingmcp.RunToolScript(ctx, env, script.Content, eventParams)
	if exitCode != 0 {
		if response != "" {
			return "", fmt.Errorf("%s", response)
		}
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("script exited with code %d", exitCode)
	}

	return response, err
}

func buildEventMetaDict(envelope *EventEnvelope) *object.Dict {
	db := database.GetInstance()

	meta := map[string]object.Object{
		"type": object.NewString(envelope.EventType),
		"id":   object.NewString(envelope.EventId),
		"ts":   object.NewString(envelope.Ts.Time().UTC().Format(time.RFC3339Nano)),
		"actor": object.NewStringDict(map[string]object.Object{
			"id":       object.NewString(envelope.Actor.Id),
			"username": object.NewString(envelope.Actor.Username),
			"kind":     object.NewString(envelope.Actor.Kind),
		}),
	}

	spaceDict := map[string]object.Object{
		"id":   object.NewString(envelope.SpaceId),
		"name": object.NewString(""),
	}

	spaceUrls := map[string]object.Object{}
	customDict := map[string]object.Object{}

	if envelope.SpaceId != "" {
		space, err := db.GetSpace(envelope.SpaceId)
		if err == nil && space != nil {
			spaceDict["name"] = object.NewString(space.Name)

			routingName := space.Name
			username := ""
			user, err := db.GetUser(space.UserId)
			if err == nil && user != nil {
				username = user.Username
			}

			if space.PoolId != "" {
				pool, err := db.GetPoolDefinition(space.PoolId)
				if err == nil && pool != nil && !pool.IsDeleted {
					routingName = pool.Name
				}
			}

			cfg := config.GetServerConfig()
			wildcardDomain := cfg.WildcardDomain
			if wildcardDomain != "" {
				if wildcardDomain[0] == '*' {
					wildcardDomain = wildcardDomain[1:]
				}
				if wildcardDomain[0] != '.' {
					wildcardDomain = "." + wildcardDomain
				}

				if username != "" && routingName != "" && space.TemplateId != "" {
					tmpl, err := db.GetTemplate(space.TemplateId)
					if err == nil && tmpl != nil {
						for _, port := range tmpl.Ports {
							spaceUrls[port.Name] = object.NewString(
								"https://" + username + "--" + routingName + "--" + fmt.Sprintf("%d", port.Port) + wildcardDomain,
							)
						}
					}
				}
			}

			if space.CustomFields != nil {
				for _, field := range space.CustomFields {
					customDict[field.Name] = object.NewString(field.Value)
				}
			}
		}
	}

	spaceDict["urls"] = object.NewStringDict(spaceUrls)
	meta["space"] = object.NewStringDict(spaceDict)
	meta["space_urls"] = object.NewStringDict(spaceUrls)
	meta["custom"] = object.NewStringDict(customDict)

	return object.NewStringDict(meta)
}
