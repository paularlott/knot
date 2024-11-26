package apiclient

import (
	"errors"
	"time"
)

type BenchmarkPacket struct {
	Status  bool      `json:"status"`
	Version string    `json:"version"`
	Date    time.Time `json:"date"`
}

func (c *ApiClient) Benchmark(payload *BenchmarkPacket) error {
	var response BenchmarkPacket

	statusCode, err := c.httpClient.Post("/api/v1/benchmark", payload, &response, 200)
	if statusCode != 200 {
		return errors.New("invalid status code")
	}

	return err
}
