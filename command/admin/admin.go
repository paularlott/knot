package commands_admin

import (
	"github.com/paularlott/knot/command"
	"github.com/paularlott/knot/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	// MySQL
	adminCmd.PersistentFlags().BoolP("mysql-enabled", "", false, "Enable MySQL database backend.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MYSQL_ENABLED environment variable if set.")
	adminCmd.PersistentFlags().StringP("mysql-host", "", "", "The MySQL host to connect to (default \"localhost\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MYSQL_HOST environment variable if set.")
	adminCmd.PersistentFlags().IntP("mysql-port", "", 3306, "The MySQL port to connect to (default \"3306\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MYSQL_PORT environment variable if set.")
	adminCmd.PersistentFlags().StringP("mysql-user", "", "root", "The MySQL user to connect as (default \"root\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MYSQL_USER environment variable if set.")
	adminCmd.PersistentFlags().StringP("mysql-password", "", "", "The MySQL password to use.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MYSQL_PASSWORD environment variable if set.")
	adminCmd.PersistentFlags().StringP("mysql-database", "", "knot", "The MySQL database to use (default \"knot\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MYSQL_DATABASE environment variable if set.")
	adminCmd.PersistentFlags().IntP("mysql-connection-max-idle", "", 2, "The maximum number of idle connections in the connection pool (default \"10\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_IDLE environment variable if set.")
	adminCmd.PersistentFlags().IntP("mysql-connection-max-open", "", 100, "The maximum number of open connections to the database (default \"100\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_OPEN environment variable if set.")
	adminCmd.PersistentFlags().IntP("mysql-connection-max-lifetime", "", 5, "The maximum amount of time in minutes a connection may be reused (default \"5\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_LIFETIME environment variable if set.")

	// BadgerDB
	adminCmd.PersistentFlags().BoolP("badgerdb-enabled", "", false, "Enable BadgerDB database backend.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_BADGERDB_ENABLED environment variable if set.")
	adminCmd.PersistentFlags().StringP("badgerdb-path", "", "./badger", "The path to the BadgerDB database (default \"./badger\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_BADGERDB_PATH environment variable if set.")

	// Redis
	adminCmd.PersistentFlags().BoolP("redis-enabled", "", false, "Enable Redis database backend.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_REDIS_ENABLED environment variable if set.")
	adminCmd.PersistentFlags().StringP("redis-host", "", "localhost:6379", "The redis server (default \"localhost:6379\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_REDIS_HOST environment variable if set.")
	adminCmd.PersistentFlags().StringP("redis-password", "", "", "The password to use for the redis server.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_REDIS_PASSWORD environment variable if set.")
	adminCmd.PersistentFlags().IntP("redis-db", "", 0, "The redis database to use (default \"0\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_REDIS_DB environment variable if set.")
	adminCmd.Flags().StringP("redis-master-name", "", "", "The name of the master to use for failover clients (default \"\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_REDIS_MASTER_NAME environment variable if set.")
	adminCmd.PersistentFlags().StringP("redis-key-prefix", "", "", "The prefix to use for all keys in the redis database (default \"\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_REDIS_KEY_PREFIX environment variable if set.")

	command.RootCmd.AddCommand(adminCmd)
	adminCmd.AddCommand(renameLocationCmd)
	adminCmd.AddCommand(setPasswordCmd)
	adminCmd.AddCommand(resetTOTPCmd)
	adminCmd.AddCommand(backupCmd)
	adminCmd.AddCommand(restoreCmd)
}

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Admin Operations",
	Long:  "Run administration operations for the server.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// MySQL
		viper.BindPFlag("server.mysql.enabled", cmd.Flags().Lookup("mysql-enabled"))
		viper.BindEnv("server.mysql.enabled", config.CONFIG_ENV_PREFIX+"_MYSQL_ENABLED")
		viper.SetDefault("server.mysql.enabled", false)
		viper.BindPFlag("server.mysql.host", cmd.Flags().Lookup("mysql-host"))
		viper.BindEnv("server.mysql.host", config.CONFIG_ENV_PREFIX+"_MYSQL_HOST")
		viper.SetDefault("server.mysql.host", "localhost")
		viper.BindPFlag("server.mysql.port", cmd.Flags().Lookup("mysql-port"))
		viper.BindEnv("server.mysql.port", config.CONFIG_ENV_PREFIX+"_MYSQL_PORT")
		viper.SetDefault("server.mysql.port", 3306)
		viper.BindPFlag("server.mysql.user", cmd.Flags().Lookup("mysql-user"))
		viper.BindEnv("server.mysql.user", config.CONFIG_ENV_PREFIX+"_MYSQL_USER")
		viper.SetDefault("server.mysql.user", "root")
		viper.BindPFlag("server.mysql.password", cmd.Flags().Lookup("mysql-password"))
		viper.BindEnv("server.mysql.password", config.CONFIG_ENV_PREFIX+"_MYSQL_PASSWORD")
		viper.SetDefault("server.mysql.password", "")
		viper.BindPFlag("server.mysql.database", cmd.Flags().Lookup("mysql-database"))
		viper.BindEnv("server.mysql.database", config.CONFIG_ENV_PREFIX+"_MYSQL_DATABASE")
		viper.SetDefault("server.mysql.database", "knot")
		viper.BindPFlag("server.mysql.connection_max_idle", cmd.Flags().Lookup("mysql-connection-max-idle"))
		viper.BindEnv("server.mysql.connection_max_idle", config.CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_IDLE")
		viper.SetDefault("server.mysql.connection_max_idle", 10)
		viper.BindPFlag("server.mysql.connection_max_open", cmd.Flags().Lookup("mysql-connection-max-open"))
		viper.BindEnv("server.mysql.connection_max_open", config.CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_OPEN")
		viper.SetDefault("server.mysql.connection_max_open", 100)
		viper.BindPFlag("server.mysql.connection_max_lifetime", cmd.Flags().Lookup("mysql-connection-max-lifetime"))
		viper.BindEnv("server.mysql.connection_max_lifetime", config.CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_LIFETIME")
		viper.SetDefault("server.mysql.connection_max_lifetime", 5)

		// BadgerDB
		viper.BindPFlag("server.badgerdb.enabled", cmd.Flags().Lookup("badgerdb-enabled"))
		viper.BindEnv("server.badgerdb.enabled", config.CONFIG_ENV_PREFIX+"_BADGERDB_ENABLED")
		viper.SetDefault("server.badgerdb.enabled", false)
		viper.BindPFlag("server.badgerdb.path", cmd.Flags().Lookup("badgerdb-path"))
		viper.BindEnv("server.badgerdb.path", config.CONFIG_ENV_PREFIX+"_BADGERDB_PATH")
		viper.SetDefault("server.badgerdb.path", "./badger")

		// Redis
		viper.BindPFlag("server.redis.enabled", cmd.Flags().Lookup("redis-enabled"))
		viper.BindEnv("server.redis.enabled", config.CONFIG_ENV_PREFIX+"_REDIS_ENABLED")
		viper.SetDefault("server.redis.enabled", false)
		viper.BindPFlag("server.redis.host", cmd.Flags().Lookup("redis-host"))
		viper.BindEnv("server.redis.host", config.CONFIG_ENV_PREFIX+"_REDIS_HOST")
		viper.SetDefault("server.redis.host", "localhost:6379")
		viper.BindPFlag("server.redis.password", cmd.Flags().Lookup("redis-password"))
		viper.BindEnv("server.redis.password", config.CONFIG_ENV_PREFIX+"_REDIS_PASSWORD")
		viper.SetDefault("server.redis.password", "")
		viper.BindPFlag("server.redis.db", cmd.Flags().Lookup("redis-db"))
		viper.BindEnv("server.redis.db", config.CONFIG_ENV_PREFIX+"_REDIS_DB")
		viper.SetDefault("server.redis.db", 0)
		viper.BindPFlag("server.redis.master_name", cmd.Flags().Lookup("redis-master-name"))
		viper.BindEnv("server.redis.master_name", config.CONFIG_ENV_PREFIX+"_REDIS_MASTER_NAME")
		viper.SetDefault("server.redis.master_name", "")
		viper.BindPFlag("server.redis.key_prefix", cmd.Flags().Lookup("redis-key-prefix"))
		viper.BindEnv("server.redis.key_prefix", config.CONFIG_ENV_PREFIX+"_REDIS_KEY_PREFIX")
		viper.SetDefault("server.redis.key_prefix", "")

	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}
