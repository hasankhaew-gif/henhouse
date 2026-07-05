package db

import (
	"database/sql"
	"time"
)

type Session struct {
	ID        int
	TeamID    int
	Session   string
	Timestamp time.Time
}

func createSessionTable(db *sql.DB) (err error) {

	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS "session" (
		id		SERIAL PRIMARY KEY,
		team_id		INTEGER NOT NULL,
		session		TEXT NOT NULL,
		timestamp	TIMESTAMP with time zone DEFAULT now()
	)`)

	return
}

func AddSession(db *sql.DB, s *Session) (err error) {

	stmt, err := db.Prepare("INSERT INTO session (team_id, session) " +
		"VALUES ($1, $2) RETURNING id")
	if err != nil {
		return
	}

	defer stmt.Close()

	err = stmt.QueryRow(s.TeamID, s.Session).Scan(&s.ID)
	if err != nil {
		return
	}

	return
}

func DeleteSession(db *sql.DB, session string) (err error) {

	stmt, err := db.Prepare("DELETE FROM session WHERE session=$1")
	if err != nil {
		return
	}

	defer stmt.Close()

	_, err = stmt.Exec(session)
	if err != nil {
		return
	}

	return
}

func DeleteSessionsByTeam(db *sql.DB, teamID int) (err error) {

	stmt, err := db.Prepare("DELETE FROM session WHERE team_id=$1")
	if err != nil {
		return
	}

	defer stmt.Close()

	_, err = stmt.Exec(teamID)
	if err != nil {
		return
	}

	return
}

func GetSessionCount(db *sql.DB) (count int, err error) {
	err = db.QueryRow("SELECT COUNT(DISTINCT team_id) FROM session;").Scan(&count)
	return
}

func GetSessionTeam(db *sql.DB, session string) (teamID int, err error) {

	stmt, err := db.Prepare("SELECT team_id FROM session WHERE session=$1")
	if err != nil {
		return
	}

	defer stmt.Close()

	err = stmt.QueryRow(session).Scan(&teamID)
	if err != nil {
		return
	}

	return
}
