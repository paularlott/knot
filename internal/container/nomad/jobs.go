package nomad

import (
	"fmt"
	"net/http"
)

func (client *NomadClient) ParseJobHCL(hcl string) (map[string]interface{}, error) {
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
		fmt.Sprintf("/v1/job/%s?purge=false&namespace=%s", jobId, namespace),
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
