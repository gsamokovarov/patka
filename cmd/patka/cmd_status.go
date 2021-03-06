package main

import (
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/gsamokovarov/patka/lib/patka"
)

var statusCmd = &Command{
	Name:    "status",
	Usage:   "",
	Summary: "dump the migration status for the current DB",
	Help:    `status extended help here...`,
	Run:     statusRun,
}

type StatusData struct {
	Source string
	Status string
}

func statusRun(cmd *Command, args ...string) {

	conf, err := dbConfFromFlags()
	if err != nil {
		log.Fatal(err)
	}

	db, err := patka.OpenDBFromDBConf(conf)
	if err != nil {
		log.Fatal("couldn't open DB:", err)
	}
	defer db.Close()

	applied, err := patka.AppliedDBVersions(conf, db)
	if err != nil {
		log.Fatal(err)
	}

	migrations, e := patka.CollectMigrations(conf.MigrationsDir, true, applied)
	if e != nil {
		log.Fatal(e)
	}

	// must ensure that the version table exists if we're running on a pristine DB
	if _, e := patka.EnsureDBVersion(conf, db); e != nil {
		log.Fatal(e)
	}

	fmt.Printf("patka: status for environment '%v'\n", conf.Env)
	fmt.Println("    Applied At                  Migration")
	fmt.Println("    =======================================")
	for _, m := range migrations {
		printMigrationStatus(db, m.Version, filepath.Base(m.Source))
	}
}

func printMigrationStatus(db *sql.DB, version int64, script string) {
	var row patka.MigrationRecord
	q := fmt.Sprintf("SELECT tstamp, is_applied FROM patka_db_version WHERE version_id=%d ORDER BY tstamp DESC LIMIT 1", version)
	e := db.QueryRow(q).Scan(&row.TStamp, &row.IsApplied)

	if e != nil && e != sql.ErrNoRows {
		log.Fatal(e)
	}

	var appliedAt string

	if row.IsApplied {
		appliedAt = row.TStamp.Format(time.ANSIC)
	} else {
		appliedAt = "Pending"
	}

	fmt.Printf("    %-24s -- %v\n", appliedAt, script)
}
