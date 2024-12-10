package nomad

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/database/model"
)

func (client *NomadClient) ParseJobHCL(hcl string, template *model.Template) (map[string]interface{}, error) {
	var response = make(map[string]interface{})

	_, err := client.httpClient.Post(
		"/v1/jobs/parse",
		map[string]interface{}{
			"JobHCL":       hcl,
			"Canonicalize": false,
		},
		&response,
		http.StatusOK,
	)
	if err != nil {
		return nil, err
	}

	// Work through TaskGroups and Tasks printing the Env array
	taskGroups, ok := response["TaskGroups"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("expected TaskGroups to be of type []interface{}")
	}
	for _, group := range taskGroups {
		groupMap, ok := group.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("expected group to be of type map[string]interface{}")
		}

		for _, task := range groupMap["Tasks"].([]interface{}) {
			taskMap, ok := task.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("expected task to be of type map[string]interface{}")
			}

			envMap, ok := taskMap["Env"].(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("expected Env to be of type map[string]interface{}")
			}

			// If KNOT_ENABLE_TERMINAL is not set, the set it to match the template
			if _, ok := envMap["KNOT_ENABLE_TERMINAL"]; !ok {
				envMap["KNOT_ENABLE_TERMINAL"] = fmt.Sprintf("%t", template.WithTerminal)
			}

			// If KNOT_CODE_SERVER_PORT is not set, then set it
			if template.WithCodeServer {
				if _, ok := envMap["KNOT_CODE_SERVER_PORT"]; !ok {
					envMap["KNOT_CODE_SERVER_PORT"] = "49374"
				}
			}

			// If KNOT_VSCODE_TUNNEL is not set, then set it
			if template.WithVSCodeTunnel {
				if _, ok := envMap["KNOT_VSCODE_TUNNEL"]; !ok {
					envMap["KNOT_VSCODE_TUNNEL"] = "vscodetunnel"
				}
			}

			// If KNOT_SSH_PORT is not set, then set it
			if template.WithSSH {
				if _, ok := envMap["KNOT_SSH_PORT"]; !ok {
					envMap["KNOT_SSH_PORT"] = "22"
				}
			}
		}
	}

	return response, nil
}

func (client *NomadClient) CreateJob(jobJSON *map[string]interface{}) (map[string]interface{}, error) {
	var response = make(map[string]interface{})
	var job = map[string]interface{}{
		"Job": jobJSON,
	}

	_, err := client.httpClient.Post(
		"/v1/jobs",
		job,
		&response,
		http.StatusOK,
	)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (client *NomadClient) DeleteJob(jobId string, namespace string) (map[string]interface{}, error) {
	var response = make(map[string]interface{})

	_, err := client.httpClient.Delete(
		fmt.Sprintf("/v1/job/%s?purge=true&namespace=%s", jobId, namespace),
		nil,
		&response,
		http.StatusOK,
	)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (client *NomadClient) ReadJob(jobId string, namespace string) (int, map[string]interface{}, error) {
	var response = make(map[string]interface{})

	code, err := client.httpClient.Get(
		fmt.Sprintf("/v1/job/%s?namespace=%s", jobId, namespace),
		&response,
	)
	if err != nil {
		return code, nil, err
	}

	return code, response, nil
}
