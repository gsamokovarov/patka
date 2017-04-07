package main

import (
	"log"

	"bitbucket.org/liamstask/goose/lib/goose"
)

var redoCmd = &Command{
	Name:    "redo",
	Usage:   "",
	Summary: "Re-run the latest migration",
	Help:    `redo extended help here...`,
	Run:     redoRun,
}

func redoRun(cmd *Command, args ...string) {
	conf, err := dbConfFromFlags()
	if err != nil {
		log.Fatal(err)
	}

	db, err := goose.OpenDBFromDBConf(conf)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	appliedBeforeDown, err := goose.AppliedDBVersions(conf, db)
	if err != nil {
		log.Fatal(err)
	}

	if err := goose.RunMigrations(conf, conf.MigrationsDir, false, appliedBeforeDown); err != nil {
		log.Fatal(err)
	}

	appliedAfterDown, err := goose.AppliedDBVersions(conf, db)
	if err != nil {
		log.Fatal(err)
	}

	if err := goose.RunMigrations(conf, conf.MigrationsDir, true, appliedAfterDown); err != nil {
		log.Fatal(err)
	}
}
