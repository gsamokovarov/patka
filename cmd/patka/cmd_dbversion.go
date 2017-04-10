package main

import (
	"github.com/gsamokovarov/patka/lib/patka"
	"fmt"
	"log"
)

var dbVersionCmd = &Command{
	Name:    "dbversion",
	Usage:   "",
	Summary: "Print the current version of the database",
	Help:    `dbversion extended help here...`,
	Run:     dbVersionRun,
}

func dbVersionRun(cmd *Command, args ...string) {
	conf, err := dbConfFromFlags()
	if err != nil {
		log.Fatal(err)
	}

	current, err := patka.GetDBVersion(conf)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("patka: dbversion %v\n", current)
}
