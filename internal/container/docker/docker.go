package docker

import "github.com/spf13/viper"

type DockerClient struct {
	Host       string
	DriverName string
}

func NewClient() *DockerClient {
	return &DockerClient{
		Host:       viper.GetString("server.docker.host"),
		DriverName: "docker",
	}
}
