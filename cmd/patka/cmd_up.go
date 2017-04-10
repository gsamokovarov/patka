package main

import (
	"log"

	"github.com/gsamokovarov/patka/lib/patka"
)

var upCmd = &Command{
	Name:    "up",
	Usage:   "",
	Summary: "Migrate the DB to the most recent version available",
	Help:    `up extended help here...`,
	Run:     upRun,
}

func upRun(cmd *Command, args ...string) {

	conf, err := dbConfFromFlags()
	if err != nil {
		log.Fatal(err)
	}

	db, err := patka.OpenDBFromDBConf(conf)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	applied, err := patka.AppliedDBVersions(conf, db)
	if err != nil {
		log.Fatal(err)
	}

	if err := patka.RunMigrations(conf, conf.MigrationsDir, true, applied); err != nil {
		log.Fatal(err)
	}
}
