package db

import (
	"database/sql"

	_ "github.com/lib/pq"
)

var tables = [...]string{"category", "flag", "news", "score", "session",
	"setting", "support", "task", "team"}

func createSchema(db *sql.DB) error {

	_, err := db.Exec("CREATE SCHEMA IF NOT EXISTS public")
	if err != nil {
		return err
	}

	var errs []error

	errs = append(errs, createCategoryTable(db))
	errs = append(errs, createFlagTable(db))
	errs = append(errs, createNewsTable(db))
	errs = append(errs, createScoreTable(db))
	errs = append(errs, createSessionTable(db))
	errs = append(errs, createSettingTable(db))
	errs = append(errs, createSupportTable(db))
	errs = append(errs, createTaskTable(db))
	errs = append(errs, createTeamTable(db))

	for _, e := range errs {
		if e != nil {
			return e
		}
	}

	return nil
}

func OpenDatabase(path string) (db *sql.DB, err error) {

	db, err = sql.Open("postgres", path)
	if err != nil {
		return
	}

	err = createSchema(db)
	if err != nil {
		return
	}

	return
}

func cleanTable(db *sql.DB, table string) (err error) {
	_, err = db.Exec("DELETE FROM " + table)
	return
}

func restartSequence(db *sql.DB, table string) (err error) {
	_, err = db.Exec("ALTER SEQUENCE " + table + "_id_seq RESTART WITH 1;")
	return
}

func CleanDatabase(db *sql.DB) (err error) {

	for _, table := range tables {

		err = cleanTable(db, table)
		if err != nil {
			return
		}

		if table == "setting" {
			continue
		}

		err = restartSequence(db, table)
		if err != nil {
			return
		}
	}

	return
}

func dropSchema(db *sql.DB) (err error) {

	_, err = db.Exec("DROP SCHEMA public CASCADE")
	if err != nil {
		return
	}
	return
}

func InitDatabase(path string) (db *sql.DB, err error) {

	db, err = OpenDatabase(path)
	if err != nil {
		return
	}

	dropSchema(db)

	err = createSchema(db)
	if err != nil {
		return
	}

	return
}
