package main

import (
	"github.com/gsamokovarov/patka/lib/patka"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

var createCmd = &Command{
	Name:    "create",
	Usage:   "",
	Summary: "Create the scaffolding for a new migration",
	Help:    `create extended help here...`,
	Run:     createRun,
}

func createRun(cmd *Command, args ...string) {

	if len(args) < 1 {
		log.Fatal("patka create: migration name required")
	}

	migrationType := "go" // default to Go migrations
	if len(args) >= 2 {
		migrationType = args[1]
	}

	conf, err := dbConfFromFlags()
	if err != nil {
		log.Fatal(err)
	}

	if err = os.MkdirAll(conf.MigrationsDir, 0777); err != nil {
		log.Fatal(err)
	}

	n, err := patka.CreateMigration(args[0], migrationType, conf.MigrationsDir, time.Now())
	if err != nil {
		log.Fatal(err)
	}

	a, e := filepath.Abs(n)
	if e != nil {
		log.Fatal(e)
	}

	fmt.Println("patka: created", a)
}
