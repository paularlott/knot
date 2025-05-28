package database

import (
	"errors"
	"sync"

	driver_badgerdb "github.com/paularlott/knot/internal/database/drivers/badgerdb"
	driver_memory "github.com/paularlott/knot/internal/database/drivers/memory"
	driver_mysql "github.com/paularlott/knot/internal/database/drivers/mysql"
	driver_redis "github.com/paularlott/knot/internal/database/drivers/redis"
	"github.com/paularlott/knot/internal/database/model"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

var (
	once              sync.Once
	dbInstance        DbDriver
	dbSessionInstance SessionStorage
	ErrTemplateInUse  = errors.New("template in use")
)

// DbDriver is the interface for the database drivers
type DbDriver interface {
	Connect() error

	// Users
	SaveUser(user *model.User, updateFields []string) error
	DeleteUser(user *model.User) error
	GetUser(id string) (*model.User, error)
	GetUserByEmail(email string) (*model.User, error)
	GetUserByUsername(email string) (*model.User, error)
	GetUsers() ([]*model.User, error)
	HasUsers() (bool, error)

	// Tokens
	SaveToken(token *model.Token) error
	DeleteToken(token *model.Token) error
	GetToken(id string) (*model.Token, error)
	GetTokensForUser(userId string) ([]*model.Token, error)

	// Space
	SaveSpace(space *model.Space, updateFields []string) error
	DeleteSpace(space *model.Space) error
	GetSpace(id string) (*model.Space, error)
	GetSpacesForUser(userId string) ([]*model.Space, error)
	GetSpaceByName(userId string, spaceName string) (*model.Space, error)
	GetSpacesByTemplateId(templateId string) ([]*model.Space, error)
	GetSpaces() ([]*model.Space, error)

	// Templates
	SaveTemplate(template *model.Template, updateFields []string) error
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

	// Volumes
	SaveVolume(volume *model.Volume, updateFields []string) error
	DeleteVolume(volume *model.Volume) error
	GetVolume(id string) (*model.Volume, error)
	GetVolumes() ([]*model.Volume, error)

	// Roles
	SaveRole(role *model.Role) error
	DeleteRole(role *model.Role) error
	GetRole(id string) (*model.Role, error)
	GetRoles() ([]*model.Role, error)

	// Audit Logs
	HasAuditLog() bool
	SaveAuditLog(auditLog *model.AuditLogEntry) error
	GetNumberOfAuditLogs() (int, error)
	GetAuditLogs(offset int, limit int) ([]*model.AuditLogEntry, error)

	// Config Values
	GetCfgValue(name string) (*model.CfgValue, error)
	SaveCfgValue(cfgValue *model.CfgValue) error
}

type SessionStorage interface {
	SaveSession(session *model.Session) error
	DeleteSession(session *model.Session) error
	GetSession(id string) (*model.Session, error)
	GetSessionsForUser(userId string) ([]*model.Session, error)
	GetSessions() ([]*model.Session, error)
}

// Initialize the database drivers
func initDrivers() {
	once.Do(func() {

		// Initialize the main driver
		if viper.GetBool("server.mysql.enabled") {
			// Connect to and use MySQL
			log.Debug().Msg("db: MySQL enabled")

			dbInstance = &driver_mysql.MySQLDriver{}

		} else if viper.GetBool("server.badgerdb.enabled") {
			// Connect to and use BadgerDB
			log.Debug().Msg("db: BadgerDB enabled")

			dbInstance = &driver_badgerdb.BadgerDbDriver{}

		} else if viper.GetBool("server.redis.enabled") {
			// Connect to and use Redis
			log.Debug().Msg("db: Redis enabled")

			dbInstance = &driver_redis.RedisDbDriver{}
		} else {
			// Fail with no database
			log.Fatal().Msg("db: no database enabled")
		}

		// Initialize the database
		err := dbInstance.Connect()
		if err != nil {
			log.Fatal().Err(err).Msg("db: failed to connect to database")
		} else {
			log.Debug().Msg("db: connected to database")
		}

		// If database driver implements a session storage interface then use it
		if dbInstance, ok := dbInstance.(SessionStorage); ok {
			log.Debug().Msg("db: session storage using main database driver")
			dbSessionInstance = dbInstance
		} else {
			// If redis is enabled then use it for session storage
			if viper.GetBool("server.redis.enabled") {
				// Connect to and use Redis
				driver := &driver_redis.RedisDbDriver{}
				err := driver.Connect()
				if err != nil {
					log.Debug().Msg("db: failed to connect to redis")
				} else {
					log.Debug().Msg("db: Using redis for session storage")

					dbSessionInstance = driver
				}
			}

			// No driver so use memory
			if dbSessionInstance == nil {
				log.Debug().Msg("db: Using memory for session storage")

				driver := &driver_memory.MemoryDbDriver{}
				driver.Connect()
				dbSessionInstance = driver
			}
		}

		// Generate a node ID if it doesn't exist
		nodeId, err := dbInstance.GetCfgValue("node_id")
		if err != nil || nodeId == nil {
			log.Debug().Msg("db: node_id not found, generating a new one")

			u, err := uuid.NewV7()
			if err != nil {
				log.Fatal().Err(err).Msg("db: failed to generate node_id")
			}

			dbInstance.SaveCfgValue(&model.CfgValue{
				Name:  "node_id",
				Value: u.String(),
			})
			if err != nil {
				log.Fatal().Err(err).Msg("db: failed to save node_id")
			}
		}
	})
}

// Returns the database driver and on first call initializes it
func GetInstance() DbDriver {
	initDrivers()
	return dbInstance
}

// Returns the caching database driver and on first call initializes it
func GetSessionStorage() SessionStorage {
	initDrivers()
	return dbSessionInstance
}

func GetUserUsage(userId string, inLocation string) (*model.Usage, error) {
	db := GetInstance()

	// Load the spaces for the user and calculate the quota
	spaces, err := db.GetSpacesForUser(userId)
	if err != nil {
		return nil, err
	}

	usage := &model.Usage{
		ComputeUnits:                   0,
		StorageUnits:                   0,
		NumberSpaces:                   0,
		NumberSpacesDeployed:           0,
		NumberSpacesDeployedInLocation: 0,
	}

	for _, space := range spaces {
		// If space is shared with this user then ignore it
		if space.UserId != userId || space.IsDeleted {
			continue
		}

		usage.NumberSpaces++

		if space.IsDeployed {
			usage.NumberSpacesDeployed++

			if inLocation != "" && space.Location == inLocation {
				usage.NumberSpacesDeployedInLocation++
			}
		}

		// Get the template
		template, err := db.GetTemplate(space.TemplateId)
		if err == nil {
			if space.IsDeployed || space.IsPending {
				usage.ComputeUnits += template.ComputeUnits
			}

			usage.StorageUnits += template.StorageUnits
		}
	}

	return usage, nil
}

func GetUserQuota(user *model.User) (*model.Quota, error) {
	db := GetInstance()

	quota := &model.Quota{
		ComputeUnits: user.ComputeUnits,
		StorageUnits: user.StorageUnits,
		MaxSpaces:    user.MaxSpaces,
		MaxTunnels:   user.MaxTunnels,
	}

	// Get the groups and build a map
	groups, err := db.GetGroups()
	if err != nil {
		return nil, err
	}
	groupMap := make(map[string]*model.Group)
	for _, group := range groups {
		groupMap[group.Id] = group
	}

	// Sum the compute and storage units from groups
	for _, groupId := range user.Groups {
		group, ok := groupMap[groupId]
		if ok {
			quota.MaxSpaces += group.MaxSpaces
			quota.ComputeUnits += group.ComputeUnits
			quota.StorageUnits += group.StorageUnits
			quota.MaxTunnels += group.MaxTunnels
		}
	}

	return quota, nil
}
