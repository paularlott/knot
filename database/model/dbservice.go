package model

import (
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Database Service object
type DbService struct {
  Id string `json:"user_id"`
  Name string `json:"name"`
  DbType string `json:"db_type"`
  DbHost string `json:"db_host"`
  DbPort int `json:"db_port"`
  DbUser string `json:"db_user"`
  DbPassword string `json:"db_password"`
  ProxyHost string `json:"proxy_host"`
  ProxyPort int `json:"proxy_port"`
  ProxyUser string `json:"proxy_user"`
  ProxyPassword string `json:"proxy_password"`
}

const (
  DbServiceTypeMySQL = "mysql"
)

func NewDbService(name string, dbType string, dbHost string, dbPort int, dbUser string, dbPassword string, proxyHost string, proxyPort int, proxyUser string, proxyPassword string) *DbService {
  id, err := uuid.NewV7()
  if err != nil {
    log.Fatal().Msg(err.Error())
  }

  service := &DbService{
    Id: id.String(),
    Name: name,
    DbType: dbType,
    DbHost: dbHost,
    DbPort: dbPort,
    DbUser: dbUser,
    DbPassword: dbPassword,
    ProxyHost: proxyHost,
    ProxyPort: proxyPort,
    ProxyUser: proxyUser,
    ProxyPassword: proxyPassword,
  }

  return service
}
