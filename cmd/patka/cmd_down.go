package main

import (
	"log"

	"github.com/gsamokovarov/patka/lib/patka"
)

var downCmd = &Command{
	Name:    "down",
	Usage:   "",
	Summary: "Roll back the version by 1",
	Help:    `down extended help here...`,
	Run:     downRun,
}

func downRun(cmd *Command, args ...string) {

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

	if err = patka.RunMigrations(conf, conf.MigrationsDir, false, applied); err != nil {
		log.Fatal(err)
	}
}
