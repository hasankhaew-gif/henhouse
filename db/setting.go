package db

import (
	"database/sql"
)

func createSettingTable(db *sql.DB) (err error) {

	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS "setting" (
		key	TEXT PRIMARY KEY,
		value	TEXT NOT NULL
	)`)

	return
}

func SetSetting(db *sql.DB, key, value string) (err error) {

	stmt, err := db.Prepare("INSERT INTO setting (key, value) " +
		"VALUES ($1, $2) " +
		"ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value")
	if err != nil {
		return
	}

	defer stmt.Close()

	_, err = stmt.Exec(key, value)
	if err != nil {
		return
	}

	return
}

func GetSetting(db *sql.DB, key string) (value string, err error) {

	stmt, err := db.Prepare("SELECT value FROM setting WHERE key=$1")
	if err != nil {
		return
	}

	defer stmt.Close()

	err = stmt.QueryRow(key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return
	}

	return
}
