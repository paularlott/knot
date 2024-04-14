package commands_admin

import (
	"fmt"

	"github.com/paularlott/knot/command"
	"github.com/paularlott/knot/database"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	// MySQL
	renameLocationCmd.Flags().BoolP("mysql-enabled", "", false, "Enable MySQL database backend.\nOverrides the "+command.CONFIG_ENV_PREFIX+"_MYSQL_ENABLED environment variable if set.")
	renameLocationCmd.Flags().StringP("mysql-host", "", "", "The MySQL host to connect to (default \"localhost\").\nOverrides the "+command.CONFIG_ENV_PREFIX+"_MYSQL_HOST environment variable if set.")
	renameLocationCmd.Flags().IntP("mysql-port", "", 3306, "The MySQL port to connect to (default \"3306\").\nOverrides the "+command.CONFIG_ENV_PREFIX+"_MYSQL_PORT environment variable if set.")
	renameLocationCmd.Flags().StringP("mysql-user", "", "root", "The MySQL user to connect as (default \"root\").\nOverrides the "+command.CONFIG_ENV_PREFIX+"_MYSQL_USER environment variable if set.")
	renameLocationCmd.Flags().StringP("mysql-password", "", "", "The MySQL password to use.\nOverrides the "+command.CONFIG_ENV_PREFIX+"_MYSQL_PASSWORD environment variable if set.")
	renameLocationCmd.Flags().StringP("mysql-database", "", "knot", "The MySQL database to use (default \"knot\").\nOverrides the "+command.CONFIG_ENV_PREFIX+"_MYSQL_DATABASE environment variable if set.")
	renameLocationCmd.Flags().IntP("mysql-connection-max-idle", "", 2, "The maximum number of idle connections in the connection pool (default \"10\").\nOverrides the "+command.CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_IDLE environment variable if set.")
	renameLocationCmd.Flags().IntP("mysql-connection-max-open", "", 100, "The maximum number of open connections to the database (default \"100\").\nOverrides the "+command.CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_OPEN environment variable if set.")
	renameLocationCmd.Flags().IntP("mysql-connection-max-lifetime", "", 5, "The maximum amount of time in minutes a connection may be reused (default \"5\").\nOverrides the "+command.CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_LIFETIME environment variable if set.")

	// BadgerDB
	renameLocationCmd.Flags().BoolP("badgerdb-enabled", "", false, "Enable BadgerDB database backend.\nOverrides the "+command.CONFIG_ENV_PREFIX+"_BADGERDB_ENABLED environment variable if set.")
	renameLocationCmd.Flags().StringP("badgerdb-path", "", "./badger", "The path to the BadgerDB database (default \"./badger\").\nOverrides the "+command.CONFIG_ENV_PREFIX+"_BADGERDB_PATH environment variable if set.")

	// Redis
	renameLocationCmd.Flags().BoolP("redis-enabled", "", false, "Enable Redis database backend.\nOverrides the "+command.CONFIG_ENV_PREFIX+"_REDIS_ENABLED environment variable if set.")
	renameLocationCmd.Flags().StringP("redis-host", "", "localhost:6379", "The redis server (default \"localhost:6379\").\nOverrides the "+command.CONFIG_ENV_PREFIX+"_REDIS_HOST environment variable if set.")
	renameLocationCmd.Flags().StringP("redis-password", "", "", "The password to use for the redis server.\nOverrides the "+command.CONFIG_ENV_PREFIX+"_REDIS_PASSWORD environment variable if set.")
	renameLocationCmd.Flags().IntP("redis-db", "", 0, "The redis database to use (default \"0\").\nOverrides the "+command.CONFIG_ENV_PREFIX+"_REDIS_DB environment variable if set.")
}

var renameLocationCmd = &cobra.Command{
	Use:   "rename-location <old> <new> [flags]",
	Short: "Rename a location",
	Long: `Rename a location.

NOTE: The location name is updated within the database however spaces and volumes are not moved.

  old   The old location name
  new   The new location name`,
	Args: cobra.ExactArgs(2),
	PreRun: func(cmd *cobra.Command, args []string) {
		// MySQL
		viper.BindPFlag("server.mysql.enabled", cmd.Flags().Lookup("mysql-enabled"))
		viper.BindEnv("server.mysql.enabled", command.CONFIG_ENV_PREFIX+"_MYSQL_ENABLED")
		viper.SetDefault("server.mysql.enabled", false)
		viper.BindPFlag("server.mysql.host", cmd.Flags().Lookup("mysql-host"))
		viper.BindEnv("server.mysql.host", command.CONFIG_ENV_PREFIX+"_MYSQL_HOST")
		viper.SetDefault("server.mysql.host", "localhost")
		viper.BindPFlag("server.mysql.port", cmd.Flags().Lookup("mysql-port"))
		viper.BindEnv("server.mysql.port", command.CONFIG_ENV_PREFIX+"_MYSQL_PORT")
		viper.SetDefault("server.mysql.port", 3306)
		viper.BindPFlag("server.mysql.user", cmd.Flags().Lookup("mysql-user"))
		viper.BindEnv("server.mysql.user", command.CONFIG_ENV_PREFIX+"_MYSQL_USER")
		viper.SetDefault("server.mysql.user", "root")
		viper.BindPFlag("server.mysql.password", cmd.Flags().Lookup("mysql-password"))
		viper.BindEnv("server.mysql.password", command.CONFIG_ENV_PREFIX+"_MYSQL_PASSWORD")
		viper.SetDefault("server.mysql.password", "")
		viper.BindPFlag("server.mysql.database", cmd.Flags().Lookup("mysql-database"))
		viper.BindEnv("server.mysql.database", command.CONFIG_ENV_PREFIX+"_MYSQL_DATABASE")
		viper.SetDefault("server.mysql.database", "knot")
		viper.BindPFlag("server.mysql.connection_max_idle", cmd.Flags().Lookup("mysql-connection-max-idle"))
		viper.BindEnv("server.mysql.connection_max_idle", command.CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_IDLE")
		viper.SetDefault("server.mysql.connection_max_idle", 10)
		viper.BindPFlag("server.mysql.connection_max_open", cmd.Flags().Lookup("mysql-connection-max-open"))
		viper.BindEnv("server.mysql.connection_max_open", command.CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_OPEN")
		viper.SetDefault("server.mysql.connection_max_open", 100)
		viper.BindPFlag("server.mysql.connection_max_lifetime", cmd.Flags().Lookup("mysql-connection-max-lifetime"))
		viper.BindEnv("server.mysql.connection_max_lifetime", command.CONFIG_ENV_PREFIX+"_MYSQL_CONNECTION_MAX_LIFETIME")
		viper.SetDefault("server.mysql.connection_max_lifetime", 5)

		// BadgerDB
		viper.BindPFlag("server.badgerdb.enabled", cmd.Flags().Lookup("badgerdb-enabled"))
		viper.BindEnv("server.badgerdb.enabled", command.CONFIG_ENV_PREFIX+"_BADGERDB_ENABLED")
		viper.SetDefault("server.badgerdb.enabled", false)
		viper.BindPFlag("server.badgerdb.path", cmd.Flags().Lookup("badgerdb-path"))
		viper.BindEnv("server.badgerdb.path", command.CONFIG_ENV_PREFIX+"_BADGERDB_PATH")
		viper.SetDefault("server.badgerdb.path", "./badger")

		// Redis
		viper.BindPFlag("server.redis.enabled", cmd.Flags().Lookup("redis-enabled"))
		viper.BindEnv("server.redis.enabled", command.CONFIG_ENV_PREFIX+"_REDIS_ENABLED")
		viper.SetDefault("server.redis.enabled", false)
		viper.BindPFlag("server.redis.host", cmd.Flags().Lookup("redis-host"))
		viper.BindEnv("server.redis.host", command.CONFIG_ENV_PREFIX+"_REDIS_HOST")
		viper.SetDefault("server.redis.host", "localhost:6379")
		viper.BindPFlag("server.redis.password", cmd.Flags().Lookup("redis-password"))
		viper.BindEnv("server.redis.password", command.CONFIG_ENV_PREFIX+"_REDIS_PASSWORD")
		viper.SetDefault("server.redis.password", "")
		viper.BindPFlag("server.redis.db", cmd.Flags().Lookup("redis-db"))
		viper.BindEnv("server.redis.db", command.CONFIG_ENV_PREFIX+"_REDIS_DB")
		viper.SetDefault("server.redis.db", 0)
	},
	Run: func(cmd *cobra.Command, args []string) {

		// Display what is going to happen and warning
		fmt.Println("Renaming location ", args[0], "to", args[1])
		fmt.Print("This command will not move any spaces or volumes between locations.\n\n")

		// Prompt the user to confirm the deletion
		var confirm string
		fmt.Printf("Are you sure you want to rename location %s (yes/no): ", args[0])
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			fmt.Println("Rename cancelled.")
			return
		}

		// Connect to the database
		db := database.GetInstance()

		// Load all volumes and update their locations
		fmt.Print("Updating volumes\n")
		volumes, err := db.GetVolumes()
		if err != nil {
			fmt.Println("Error getting volumes: ", err)
			return
		}

		for _, volume := range volumes {
			fmt.Print("Checking Volume: ", volume.Name)
			if volume.Location == args[0] {
				volume.Location = args[1]
				err := db.SaveVolume(volume)
				if err != nil {
					fmt.Println("Error updating volume: ", err)
					return
				}

				fmt.Print(" - Updated\n")
			} else {
				fmt.Print(" - Skipping\n")
			}
		}

		// Load all spaces and update their locations
		fmt.Print("\nUpdating spaces\n")
		spaces, err := db.GetSpaces()
		if err != nil {
			fmt.Println("Error getting spaces: ", err)
			return
		}

		for _, space := range spaces {
			fmt.Print("Checking Space: ", space.Name)
			if space.Location == args[0] {
				space.Location = args[1]
				err := db.SaveSpace(space)
				if err != nil {
					fmt.Println("Error updating space: ", err)
					return
				}

				fmt.Print(" - Updated\n")
			} else {
				fmt.Print(" - Skipping\n")
			}
		}

		fmt.Print("\nLocation renamed\n")
	},
}
