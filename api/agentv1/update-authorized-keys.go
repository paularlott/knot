package agentv1

import (
	"net/http"

	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/rest"
	"github.com/spf13/viper"

	"github.com/rs/zerolog/log"
)

type AgentUpdateAuthorizedKeysRequest struct {
	Key            string `json:"key"`
	GitHubUsername string `json:"github_username"`
}

type AgentUpdateAuthorizedKeysResponse struct {
	Status bool `json:"status"`
}

var (
	lastPublicSSHKey   string = ""
	lastGitHubUsername string = ""
)

func HandleAgentUpdateAuthorizedKeys(w http.ResponseWriter, r *http.Request) {
	request := AgentUpdateAuthorizedKeysRequest{}

	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
		return
	}

	if viper.GetBool("agent.update_authorized_keys") && viper.GetInt("agent.port.ssh") > 0 {
		// If the key is the same as the last key then skip
		if request.Key != lastPublicSSHKey || request.GitHubUsername != lastGitHubUsername {
			log.Debug().Msg("updating authorized_keys")

			lastPublicSSHKey = request.Key
			lastGitHubUsername = request.GitHubUsername
			err = util.UpdateAuthorizedKeys(request.Key, request.GitHubUsername)
			if err != nil {
				log.Debug().Msgf("failed to update authorized_keys: %s", err)
			}
		} else {
			log.Debug().Msg("authorized_keys already up to date")
		}
	}

	rest.SendJSON(http.StatusOK, w, AgentUpdateAuthorizedKeysResponse{
		Status: true,
	})
}

func CallAgentUpdateAuthorizedKeys(client *rest.RESTClient, sshKey string, githubUsername string) bool {
	response := &AgentUpdateAuthorizedKeysResponse{}
	statusCode, err := client.Post(
		"/update-authorized-keys",
		AgentUpdateAuthorizedKeysRequest{
			Key:            sshKey,
			GitHubUsername: githubUsername,
		},
		response,
		http.StatusOK,
	)
	return statusCode == http.StatusOK && err == nil && response.Status
}
