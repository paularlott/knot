package commands_admin

import (
	"context"

	"github.com/paularlott/knot/internal/config"

	"github.com/paularlott/cli"
)

var AdminCmd = &cli.Command{
	Name:        "admin",
	Usage:       "Admin Operations",
	Description: "Run administration operations for the server.",
	MaxArgs:     cli.NoArgs,
	Flags: []cli.Flag{
		// MySQL flags
		&cli.BoolFlag{
			Name:         "mysql-enabled",
			Usage:        "Enable MySQL database backend.",
			ConfigPath:   []string{"server.mysql.enabled"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_MYSQL_ENABLED"},
			DefaultValue: false,
			Global:       true,
		},
		&cli.StringFlag{
			Name:         "mysql-host",
			Usage:        "The MySQL host to connect to.",
			ConfigPath:   []string{"server.mysql.host"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_MYSQL_HOST"},
			DefaultValue: "localhost",
			Global:       true,
		},
		&cli.IntFlag{
			Name:         "mysql-port",
			Usage:        "The MySQL port to connect to.",
			ConfigPath:   []string{"server.mysql.port"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_MYSQL_PORT"},
			DefaultValue: 3306,
			Global:       true,
		},
		&cli.StringFlag{
			Name:         "mysql-user",
			Usage:        "The MySQL user to connect as.",
			ConfigPath:   []string{"server.mysql.user"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_MYSQL_USER"},
			DefaultValue: "root",
			Global:       true,
		},
		&cli.StringFlag{
			Name:         "mysql-password",
			Usage:        "The MySQL password to use.",
			ConfigPath:   []string{"server.mysql.password"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_MYSQL_PASSWORD"},
			DefaultValue: "",
			Global:       true,
		},
		&cli.StringFlag{
			Name:         "mysql-database",
			Usage:        "The MySQL database to use.",
			ConfigPath:   []string{"server.mysql.database"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_MYSQL_DATABASE"},
			DefaultValue: "knot",
			Global:       true,
		},
		&cli.IntFlag{
			Name:         "mysql-connection-max-idle",
			Usage:        "The maximum number of idle connections in the connection pool.",
			ConfigPath:   []string{"server.mysql.connection_max_idle"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_MYSQL_CONNECTION_MAX_IDLE"},
			DefaultValue: 10,
			Global:       true,
		},
		&cli.IntFlag{
			Name:         "mysql-connection-max-open",
			Usage:        "The maximum number of open connections to the database.",
			ConfigPath:   []string{"server.mysql.connection_max_open"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_MYSQL_CONNECTION_MAX_OPEN"},
			DefaultValue: 100,
			Global:       true,
		},
		&cli.IntFlag{
			Name:         "mysql-connection-max-lifetime",
			Usage:        "The maximum amount of time in minutes a connection may be reused.",
			ConfigPath:   []string{"server.mysql.connection_max_lifetime"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_MYSQL_CONNECTION_MAX_LIFETIME"},
			DefaultValue: 5,
			Global:       true,
		},

		// BadgerDB flags
		&cli.BoolFlag{
			Name:         "badgerdb-enabled",
			Usage:        "Enable BadgerDB database backend.",
			ConfigPath:   []string{"server.badgerdb.enabled"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_BADGERDB_ENABLED"},
			DefaultValue: false,
			Global:       true,
		},
		&cli.StringFlag{
			Name:         "badgerdb-path",
			Usage:        "The path to the BadgerDB database.",
			ConfigPath:   []string{"server.badgerdb.path"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_BADGERDB_PATH"},
			DefaultValue: "./badger",
			Global:       true,
		},

		// Redis flags
		&cli.BoolFlag{
			Name:         "redis-enabled",
			Usage:        "Enable Redis database backend.",
			ConfigPath:   []string{"server.redis.enabled"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_REDIS_ENABLED"},
			DefaultValue: false,
			Global:       true,
		},
		&cli.StringFlag{
			Name:         "redis-host",
			Usage:        "The redis server.",
			ConfigPath:   []string{"server.redis.host"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_REDIS_HOST"},
			DefaultValue: "localhost:6379",
			Global:       true,
		},
		&cli.StringFlag{
			Name:         "redis-password",
			Usage:        "The password to use for the redis server.",
			ConfigPath:   []string{"server.redis.password"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_REDIS_PASSWORD"},
			DefaultValue: "",
			Global:       true,
		},
		&cli.IntFlag{
			Name:         "redis-db",
			Usage:        "The redis database to use.",
			ConfigPath:   []string{"server.redis.db"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_REDIS_DB"},
			DefaultValue: 0,
			Global:       true,
		},
		&cli.StringFlag{
			Name:         "redis-master-name",
			Usage:        "The name of the master to use for failover clients.",
			ConfigPath:   []string{"server.redis.master_name"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_REDIS_MASTER_NAME"},
			DefaultValue: "",
			Global:       true,
		},
		&cli.StringFlag{
			Name:         "redis-key-prefix",
			Usage:        "The prefix to use for all keys in the redis database.",
			ConfigPath:   []string{"server.redis.key_prefix"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_REDIS_KEY_PREFIX"},
			DefaultValue: "",
			Global:       true,
		},
		&cli.StringFlag{
			Name:         "encrypt",
			Usage:        "The encryption key to use for encrypting stored variables.",
			ConfigPath:   []string{"server.encrypt"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_ENCRYPT"},
			DefaultValue: "",
			Global:       true,
		},
	},
	Commands: []*cli.Command{
		RenameZoneCmd,
		SetPasswordCmd,
		ResetTOTPCmd,
		BackupCmd,
		RestoreCmd,
	},
	PreRun: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		var err error

		if ctx, err = cmd.GetRootCmd().PreRun(ctx, cmd); err != nil {
			return ctx, err
		}

		serverCfg := &config.ServerConfig{
			EncryptionKey: cmd.GetString("encrypt"),
			MySQL: config.MySQLConfig{
				Enabled:               cmd.GetBool("mysql-enabled"),
				Host:                  cmd.GetString("mysql-host"),
				Port:                  cmd.GetInt("mysql-port"),
				User:                  cmd.GetString("mysql-user"),
				Password:              cmd.GetString("mysql-password"),
				Database:              cmd.GetString("mysql-database"),
				ConnectionMaxIdle:     cmd.GetInt("mysql-connection-max-idle"),
				ConnectionMaxOpen:     cmd.GetInt("mysql-connection-max-open"),
				ConnectionMaxLifetime: cmd.GetInt("mysql-connection-max-lifetime"),
			},
			BadgerDB: config.BadgerDBConfig{
				Enabled: cmd.GetBool("badgerdb-enabled"),
				Path:    cmd.GetString("badgerdb-path"),
			},
			Redis: config.RedisConfig{
				Enabled:    cmd.GetBool("redis-enabled"),
				Hosts:      cmd.GetStringSlice("redis-hosts"),
				Password:   cmd.GetString("redis-password"),
				DB:         cmd.GetInt("redis-db"),
				MasterName: cmd.GetString("redis-master-name"),
				KeyPrefix:  cmd.GetString("redis-key-prefix"),
			},
			Audit: config.AuditConfig{
				Retention: cmd.GetInt("audit-retention"),
			},
		}
		config.SetServerConfig(serverCfg)

		return ctx, nil
	},
}
