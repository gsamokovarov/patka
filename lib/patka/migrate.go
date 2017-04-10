package patka

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	_ "github.com/ziutek/mymysql/godrv"
)

var (
	ErrTableDoesNotExist = errors.New("table does not exist")
	ErrNoPreviousVersion = errors.New("no previous version found")
)

type MigrationRecord struct {
	VersionId int64
	TStamp    time.Time
	IsApplied bool // was this a result of up() or down()
}

type Migration struct {
	Version  int64
	Next     int64  // next version, or -1 if none
	Previous int64  // previous version, -1 if none
	Source   string // path to .go or .sql script
}

type migrationSorter []*Migration

// helpers so we can use pkg sort
func (ms migrationSorter) Len() int           { return len(ms) }
func (ms migrationSorter) Swap(i, j int)      { ms[i], ms[j] = ms[j], ms[i] }
func (ms migrationSorter) Less(i, j int) bool { return ms[i].Version < ms[j].Version }

func newMigration(v int64, src string) *Migration {
	return &Migration{v, -1, -1, src}
}

func RunMigrations(conf *DBConf, migrationsDir string, direction bool, applied map[int64]bool) (err error) {

	db, err := OpenDBFromDBConf(conf)
	if err != nil {
		return err
	}
	defer db.Close()

	return RunMigrationsOnDb(conf, migrationsDir, direction, applied, db)
}

// Runs migration on a specific database instance.
func RunMigrationsOnDb(conf *DBConf, migrationsDir string, direction bool, applied map[int64]bool, db *sql.DB) (err error) {

	migrations, err := CollectMigrations(migrationsDir, direction, applied)
	if err != nil {
		return err
	}

	current := currentAppliedDBVersion(applied)

	if len(migrations) == 0 {
		fmt.Printf("patka: no migrations to run. current version: %d\n", current)
		return nil
	}

	ms := migrationSorter(migrations)
	ms.Sort(direction)

	fmt.Printf("patka: migrating db environment '%v', current version: %d\n",
		conf.Env, current)

	for _, m := range ms {

		switch filepath.Ext(m.Source) {
		case ".go":
			err = runGoMigration(conf, m.Source, m.Version, direction)
		case ".sql":
			err = runSQLMigration(conf, db, m.Source, m.Version, direction)
		}

		if err != nil {
			return errors.New(fmt.Sprintf("FAIL %v, quitting migration", err))
		}

		fmt.Println("OK   ", filepath.Base(m.Source))
	}

	return nil
}

// collect all the valid looking migration scripts in the
// migrations folder, and key them by version
func CollectMigrations(dirpath string, direction bool, applied map[int64]bool) (m []*Migration, err error) {

	// extract the numeric component of each migration,
	// filter out any uninteresting files,
	// and ensure we only have one file per migration version.
	filepath.Walk(dirpath, func(name string, info os.FileInfo, err error) error {

		if v, e := NumericComponent(name); e == nil {

			for _, g := range m {
				if v == g.Version {
					log.Fatalf("more than one file specifies the migration for version %d (%s and %s)",
						v, g.Source, filepath.Join(dirpath, name))
				}
			}

			if !direction {
				current := currentAppliedDBVersion(applied)
				if current == v {
					m = []*Migration{newMigration(current, name)}
					return nil

				}
			}

			if !applied[v] {
				m = append(m, newMigration(v, name))
			}
		}

		return nil
	})

	return m, nil
}

func currentAppliedDBVersion(applied map[int64]bool) int64 {

	current := int64(math.MinInt64)

	for version, _ := range applied {
		if version > current {
			current = version
		}
	}

	return current
}

func (ms migrationSorter) Sort(direction bool) {

	// sort ascending or descending by version
	if direction {
		sort.Sort(ms)
	} else {
		sort.Sort(sort.Reverse(ms))
	}

	// now that we're sorted in the appropriate direction,
	// populate next and previous for each migration
	for i, m := range ms {
		prev := int64(-1)
		if i > 0 {
			prev = ms[i-1].Version
			ms[i-1].Next = m.Version
		}
		ms[i].Previous = prev
	}
}

// look for migration scripts with names in the form:
//  XXX_descriptivename.ext
// where XXX specifies the version number
// and ext specifies the type of migration
func NumericComponent(name string) (int64, error) {

	base := filepath.Base(name)

	if ext := filepath.Ext(base); ext != ".go" && ext != ".sql" {
		return 0, errors.New("not a recognized migration file type")
	}

	idx := strings.Index(base, "_")
	if idx < 0 {
		return 0, errors.New("no separator found")
	}

	n, e := strconv.ParseInt(base[:idx], 10, 64)
	if e == nil && n <= 0 {
		return 0, errors.New("migration IDs must be greater than zero")
	}

	return n, e
}

// retrieve the current version for this DB.
// Create and initialize the DB version table if it doesn't exist.
func AppliedDBVersions(conf *DBConf, db *sql.DB) (map[int64]bool, error) {

	applied := make(map[int64]bool)

	rows, err := conf.Driver.Dialect.dbVersionQuery(db)
	if err != nil {
		if err == ErrTableDoesNotExist {
			return applied, createVersionTable(conf, db)
		}
		return applied, err
	}
	defer rows.Close()

	failed := make(map[int64]bool)

	for rows.Next() {
		var row MigrationRecord
		if err = rows.Scan(&row.VersionId, &row.IsApplied); err != nil {
			log.Fatal("error scanning rows:", err)
		}

		// Mark a migration as applied, only if the latest occurrence of it is
		// with truthy is_applied column. Expect version sorted in descending
		// order for this whole scheme to work.
		if row.IsApplied && !failed[row.VersionId] {
			applied[row.VersionId] = true
		} else {
			failed[row.VersionId] = true
		}
	}

	return applied, nil
}

// retrieve the current version for this DB.
// Create and initialize the DB version table if it doesn't exist.
func EnsureDBVersion(conf *DBConf, db *sql.DB) (int64, error) {

	rows, err := conf.Driver.Dialect.dbVersionQuery(db)
	if err != nil {
		if err == ErrTableDoesNotExist {
			return 0, createVersionTable(conf, db)
		}
		return 0, err
	}
	defer rows.Close()

	// The most recent record for each migration specifies
	// whether it has been applied or rolled back.
	// The first version we find that has been applied is the current version.

	toSkip := make([]int64, 0)

	for rows.Next() {
		var row MigrationRecord
		if err = rows.Scan(&row.VersionId, &row.IsApplied); err != nil {
			log.Fatal("error scanning rows:", err)
		}

		// have we already marked this version to be skipped?
		skip := false
		for _, v := range toSkip {
			if v == row.VersionId {
				skip = true
				break
			}
		}

		if skip {
			continue
		}

		// if version has been applied we're done
		if row.IsApplied {
			return row.VersionId, nil
		}

		// latest version of migration has not been applied.
		toSkip = append(toSkip, row.VersionId)
	}

	panic("failure in EnsureDBVersion()")
}

// Create the patka_db_version table
// and insert the initial 0 value into it
func createVersionTable(conf *DBConf, db *sql.DB) error {
	txn, err := db.Begin()
	if err != nil {
		return err
	}

	d := conf.Driver.Dialect

	if _, err := txn.Exec(d.createVersionTableSql()); err != nil {
		txn.Rollback()
		return err
	}

	version := 0
	applied := true
	if _, err := txn.Exec(d.insertVersionSql(), version, applied); err != nil {
		txn.Rollback()
		return err
	}

	return txn.Commit()
}

// wrapper for EnsureDBVersion for callers that don't already have
// their own DB instance
func GetDBVersion(conf *DBConf) (version int64, err error) {

	db, err := OpenDBFromDBConf(conf)
	if err != nil {
		return -1, err
	}
	defer db.Close()

	version, err = EnsureDBVersion(conf, db)
	if err != nil {
		return -1, err
	}

	return version, nil
}

func GetPreviousDBVersion(dirpath string, version int64) (previous int64, err error) {

	previous = -1
	sawGivenVersion := false

	filepath.Walk(dirpath, func(name string, info os.FileInfo, walkerr error) error {

		if !info.IsDir() {
			if v, e := NumericComponent(name); e == nil {
				if v > previous && v < version {
					previous = v
				}
				if v == version {
					sawGivenVersion = true
				}
			}
		}

		return nil
	})

	if previous == -1 {
		if sawGivenVersion {
			// the given version is (likely) valid but we didn't find
			// anything before it.
			// 'previous' must reflect that no migrations have been applied.
			previous = 0
		} else {
			err = ErrNoPreviousVersion
		}
	}

	return
}

// helper to identify the most recent possible version
// within a folder of migration scripts
func GetMostRecentDBVersion(dirpath string) (version int64, err error) {

	version = -1

	filepath.Walk(dirpath, func(name string, info os.FileInfo, walkerr error) error {
		if walkerr != nil {
			return walkerr
		}

		if !info.IsDir() {
			if v, e := NumericComponent(name); e == nil {
				if v > version {
					version = v
				}
			}
		}

		return nil
	})

	if version == -1 {
		err = errors.New("no valid version found")
	}

	return
}

func CreateMigration(name, migrationType, dir string, t time.Time) (path string, err error) {

	if migrationType != "go" && migrationType != "sql" {
		return "", errors.New("migration type must be 'go' or 'sql'")
	}

	timestamp := t.Format("20060102150405")
	filename := fmt.Sprintf("%v_%v.%v", timestamp, name, migrationType)

	fpath := filepath.Join(dir, filename)

	var tmpl *template.Template
	if migrationType == "sql" {
		tmpl = sqlMigrationTemplate
	} else {
		tmpl = goMigrationTemplate
	}

	path, err = writeTemplateToFile(fpath, tmpl, timestamp)

	return
}

// Update the version table for the given migration,
// and finalize the transaction.
func FinalizeMigration(conf *DBConf, txn *sql.Tx, direction bool, v int64) error {

	// XXX: drop patka_db_version table on some minimum version number?
	stmt := conf.Driver.Dialect.insertVersionSql()
	if _, err := txn.Exec(stmt, v, direction); err != nil {
		txn.Rollback()
		return err
	}

	return txn.Commit()
}

var goMigrationTemplate = template.Must(template.New("patka.go-migration").Parse(`
package main

import (
	"database/sql"
)

// Up is executed when this migration is applied
func Up_{{ . }}(txn *sql.Tx) {

}

// Down is executed when this migration is rolled back
func Down_{{ . }}(txn *sql.Tx) {

}
`))

var sqlMigrationTemplate = template.Must(template.New("patka.sql-migration").Parse(`
-- +patka Up
-- SQL in section 'Up' is executed when this migration is applied


-- +patka Down
-- SQL section 'Down' is executed when this migration is rolled back

`))