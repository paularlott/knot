package database

import (
	"errors"
	"sync"

	driver_badgerdb "github.com/paularlott/knot/database/drivers/badgerdb"
	driver_mysql "github.com/paularlott/knot/database/drivers/mysql"
	"github.com/paularlott/knot/database/model"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

var (
  once sync.Once
  dbInstance IDbDriver
  ErrTemplateInUse = errors.New("template in use")
)

// IDbDriver is the interface for the database drivers
type IDbDriver interface {
  Connect() error

  // Users
  SaveUser(user *model.User) error
  DeleteUser(user *model.User) error
  GetUser(id string) (*model.User, error)
  GetUserByEmail(email string) (*model.User, error)
  GetUserByUsername(email string) (*model.User, error)
  GetUsers() ([]*model.User, error)
  GetUserCount() (int, error)

  // Sessions
  SaveSession(session *model.Session) error
  DeleteSession(session *model.Session) error
  GetSession(id string) (*model.Session, error)
  GetSessionsForUser(userId string) ([]*model.Session, error)

  // Tokens
  SaveToken(token *model.Token) error
  DeleteToken(token *model.Token) error
  GetToken(id string) (*model.Token, error)
  GetTokensForUser(userId string) ([]*model.Token, error)

  // Space
  SaveSpace(space *model.Space) error
  DeleteSpace(space *model.Space) error
  GetSpace(id string) (*model.Space, error)
  GetSpacesForUser(userId string) ([]*model.Space, error)
  GetSpaceByName(userId string, spaceName string) (*model.Space, error)
  GetSpacesByTemplateId(templateId string) ([]*model.Space, error)
  GetSpaces() ([]*model.Space, error)

  // Templates
  SaveTemplate(template *model.Template) error
  DeleteTemplate(template *model.Template) error
  GetTemplate(id string) (*model.Template, error)
  GetTemplates() ([]*model.Template, error)

  // Groups
  SaveGroup(group *model.Group) error
  DeleteGroup(group *model.Group) error
  GetGroup(id string) (*model.Group, error)
  GetGroups() ([]*model.Group, error)

  // Template Variables
  SaveTemplateVar(variable *model.TemplateVar) error
  DeleteTemplateVar(variable *model.TemplateVar) error
  GetTemplateVar(id string) (*model.TemplateVar, error)
  GetTemplateVars() ([]*model.TemplateVar, error)
}

// Get returns the database driver and on first call initializes it
func GetInstance() IDbDriver {
  once.Do(func() {
    if viper.GetBool("server.mysql.enabled") {
      // Connect to and use MySQL
      log.Debug().Msg("db: MySQL enabled")

      dbInstance = &driver_mysql.MySQLDriver{}

    } else if viper.GetBool("server.badgerdb.enabled") {
      // Connect to and use BadgerDB
      log.Debug().Msg("db: BadgerDB enabled")

      dbInstance = &driver_badgerdb.BadgerDbDriver{}

    } else {
      // Fail with no database
      log.Fatal().Msg("db: no database enabled")
    }

    // Initialize the database
    err := dbInstance.Connect()
    if err != nil {
      log.Fatal().Err(err).Msg("db: failed to connect to database")
    }
  })

  return dbInstance
}
