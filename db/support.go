package db

import (
	"database/sql"
	"time"
)

type SupportRequest struct {
	ID        int
	TeamID    int
	Type      string
	Contact   string
	Attach    string
	Text      string
	TgStatus  string
	Done      bool
	Timestamp time.Time
}

func createSupportTable(db *sql.DB) (err error) {

	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS "support" (
		id		SERIAL PRIMARY KEY,
		team_id		INTEGER NOT NULL,
		type		TEXT NOT NULL,
		contact		TEXT NOT NULL,
		attach		TEXT NOT NULL,
		text		TEXT NOT NULL,
		tg_status	TEXT NOT NULL DEFAULT '',
		done		BOOLEAN NOT NULL DEFAULT false,
		timestamp	TIMESTAMP with time zone DEFAULT now()
	)`)

	return
}

func AddSupportRequest(db *sql.DB, s *SupportRequest) (err error) {

	stmt, err := db.Prepare("INSERT INTO support " +
		"(team_id, type, contact, attach, text, tg_status) " +
		"VALUES ($1, $2, $3, $4, $5, $6) RETURNING id")
	if err != nil {
		return
	}

	defer stmt.Close()

	err = stmt.QueryRow(s.TeamID, s.Type, s.Contact, s.Attach, s.Text,
		s.TgStatus).Scan(&s.ID)
	if err != nil {
		return
	}

	return
}

func GetSupportRequests(db *sql.DB) (reqs []SupportRequest, err error) {

	rows, err := db.Query("SELECT id, team_id, type, contact, attach, " +
		"text, tg_status, done, timestamp FROM support " +
		"ORDER BY id DESC")
	if err != nil {
		return
	}

	defer rows.Close()

	for rows.Next() {
		var s SupportRequest

		err = rows.Scan(&s.ID, &s.TeamID, &s.Type, &s.Contact,
			&s.Attach, &s.Text, &s.TgStatus, &s.Done, &s.Timestamp)
		if err != nil {
			return
		}

		reqs = append(reqs, s)
	}

	return
}

func GetSupportRequest(db *sql.DB, id int) (s SupportRequest, err error) {

	stmt, err := db.Prepare("SELECT id, team_id, type, contact, attach, " +
		"text, tg_status, done, timestamp FROM support WHERE id=$1")
	if err != nil {
		return
	}

	defer stmt.Close()

	err = stmt.QueryRow(id).Scan(&s.ID, &s.TeamID, &s.Type, &s.Contact,
		&s.Attach, &s.Text, &s.TgStatus, &s.Done, &s.Timestamp)
	if err != nil {
		return
	}

	return
}

func SetSupportTgStatus(db *sql.DB, id int, status string) (err error) {

	stmt, err := db.Prepare("UPDATE support SET tg_status=$1 WHERE id=$2")
	if err != nil {
		return
	}

	defer stmt.Close()

	_, err = stmt.Exec(status, id)
	if err != nil {
		return
	}

	return
}

func SetSupportAttach(db *sql.DB, id int, attach string) (err error) {

	stmt, err := db.Prepare("UPDATE support SET attach=$1 WHERE id=$2")
	if err != nil {
		return
	}

	defer stmt.Close()

	_, err = stmt.Exec(attach, id)
	if err != nil {
		return
	}

	return
}

func SetSupportDone(db *sql.DB, id int, done bool) (err error) {

	stmt, err := db.Prepare("UPDATE support SET done=$1 WHERE id=$2")
	if err != nil {
		return
	}

	defer stmt.Close()

	_, err = stmt.Exec(done, id)
	if err != nil {
		return
	}

	return
}

func DeleteSupportRequest(db *sql.DB, id int) (err error) {

	stmt, err := db.Prepare("DELETE FROM support WHERE id=$1")
	if err != nil {
		return
	}

	defer stmt.Close()

	_, err = stmt.Exec(id)
	if err != nil {
		return
	}

	return
}

func CountNewSupportRequests(db *sql.DB) (count int, err error) {

	err = db.QueryRow("SELECT COUNT(*) FROM support " +
		"WHERE NOT done").Scan(&count)
	if err != nil {
		return
	}

	return
}
