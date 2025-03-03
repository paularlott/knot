package driver_mysql

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/paularlott/knot/util"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type MySQLDriver struct {
	connection *sql.DB
}

// Performs the real connection to the database, we use this to reconnect if the database moves to a new server etc.
func (db *MySQLDriver) realConnect() error {
	log.Debug().Msg("db: connecting to MySQL")

	host := viper.GetString("server.mysql.host")
	port := viper.GetInt("server.mysql.port")

	// If the host starts with srv+ then lookup the SRV record
	if host[:4] == "srv+" {
		for i := 0; i < 10; i++ {
			hostPort, err := util.LookupSRV(host[4:])
			if err != nil {
				if i == 9 {
					log.Fatal().Err(err).Msg("db: failed to lookup SRV record for MySQL database aborting after 10 attempts")
				} else {
					log.Error().Err(err).Msg("db: failed to lookup SRV record for MySQL database")
				}
				time.Sleep(3 * time.Second)
				continue
			}

			host = (*hostPort)[0].Host
			port, err = strconv.Atoi((*hostPort)[0].Port)
			if err != nil {
				log.Fatal().Err(err).Msg("db: failed to convert MySQL port to integer")
			}

			break
		}
	}

	var err error
	db.connection, err = sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
		viper.GetString("server.mysql.user"),
		viper.GetString("server.mysql.password"),
		host,
		port,
		viper.GetString("server.mysql.database"),
	))
	if err == nil {
		db.connection.SetConnMaxLifetime(time.Minute * time.Duration(viper.GetInt("server.mysql.connection_max_lifetime")))
		db.connection.SetMaxOpenConns(viper.GetInt("server.mysql.connection_max_open"))
		db.connection.SetMaxIdleConns(viper.GetInt("server.mysql.connection_max_idle"))

		log.Debug().Msg("db: connected to MySQL")
	} else {
		log.Error().Err(err).Msg("db: failed to connect to MySQL")
	}

	return err
}

func (db *MySQLDriver) Connect() error {
	err := db.realConnect()
	if err == nil {
		err := db.initialize()
		if err != nil {
			log.Fatal().Err(err).Msg("db: failed to initialize MySQL database")
		}
	}

	// Start a go routine to monitor the database
	go func() {
		for {
			time.Sleep(10 * time.Second)

			log.Debug().Msg("db: testing MySQL connection")

			// Ping the database
			err := db.connection.Ping()
			if err != nil {
				log.Error().Err(err).Msg("db: failed to ping MySQL database")
				db.connection.Close()

				// Attempt to reconnect
				db.realConnect()
			}
		}
	}()

	// TODO remove this
	/* 	fmt.Println("running tests")

	   	var spaces []*model.Space
	   	err = db.read("spaces", &spaces, []string{"Id", "Name"}, "space_id = ?", "0193bd73-634d-73d4-931d-49071205c664")
	   	if err != nil {
	   		log.Fatal().Err(err).Msg("db: failed to read spaces")
	   	}

	   	fmt.Println(spaces[0])

	   	os.Exit(1) */
	/*
		fmt.Println("printing spaces")
		for _, space := range spaces {
			fmt.Println(space.Name, len(space.VolumeData))
			//fmt.Println("the value of test is ", space.Test)

			for _, v := range space.VolumeData {
				fmt.Println("volume data", v)
			}
			fmt.Println("---- end of space ----")
		}

		fmt.Println("updating spaces")
		spaces[0].VolumeData["d1"] = model.SpaceVolume{Id: "d1", Namespace: "d1"}
		spaces[0].VolumeData["d2"] = model.SpaceVolume{Id: "d2", Namespace: "d2"}
		//spaces[0].VolumeData = make(map[string]model.SpaceVolume)
		//spaces[0].Test = []string{"test", "test2", "test3"}
		err = db.UpdateSpace(spaces[0], "VolumeData")
		if err != nil {
			log.Fatal().Err(err).Msg("db: failed to update spaces")
		}
	*/
	/* 	fmt.Println("Testing create")
	   	altNames := []string{}
	   	s := model.NewSpace("dbtest", "ccf1d2dd-65d0-4b96-a590-443766c1bf0a", "018ffb47-0b07-7192-b7df-c28d121dfdc2", "bash", &altNames)
	   	s.VolumeData["d1222"] = model.SpaceVolume{Id: "d1a", Namespace: "d1b"}
	   	fmt.Println("save space")
	   	err = db.SaveSpace(s)
	   	if err != nil {
	   		log.Fatal().Err(err).Msg("db: failed to create space")
	   	}
	   	fmt.Println("new space", s.Id) */

	/* 	fmt.Println("testing delete")
	   	s := &model.Space{Id: "01955b2f-cf23-7a25-8d90-03fb45cf7cde"}

	   	err = db.DeleteSpace(s)
	   	if err != nil {
	   		log.Fatal().Err(err).Msg("db: failed to delete space")
	   	} */

	/* 	fmt.Println("done running tests")
	   	os.Exit(0) */

	return err
}
