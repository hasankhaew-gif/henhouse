package db

import (
	"database/sql"
	"time"
)

type Score struct {
	ID        int
	TeamID    int
	Score     int
	Timestamp time.Time
}

func createScoreTable(db *sql.DB) (err error) {

	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS "score" (
		id		SERIAL PRIMARY KEY,
		team_id		INTEGER NOT NULL,
		score		INTEGER NOT NULL,
		timestamp	TIMESTAMP with time zone DEFAULT now()
	)`)

	return
}

func AddScore(db *sql.DB, score *Score) (err error) {

	stmt, err := db.Prepare("INSERT INTO score (team_id, score) " +
		"VALUES ($1, $2) RETURNING id")
	if err != nil {
		return
	}

	defer stmt.Close()

	err = stmt.QueryRow(score.TeamID, score.Score).Scan(&score.ID)
	if err != nil {
		return
	}

	return
}

func DeleteScoresByTeam(db *sql.DB, teamID int) (err error) {

	stmt, err := db.Prepare("DELETE FROM score WHERE team_id=$1")
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

func GetScoreHistoryAll(db *sql.DB) (scores []Score, err error) {

	rows, err := db.Query(
		`SELECT team_id, MAX(score) AS score,
		        DATE_TRUNC('minute', timestamp) + INTERVAL '1 minute' AS ts
		 FROM score
		 GROUP BY team_id, DATE_TRUNC('minute', timestamp)
		 ORDER BY team_id, ts`)
	if err != nil {
		return
	}

	defer rows.Close()

	for rows.Next() {
		var s Score
		err = rows.Scan(&s.TeamID, &s.Score, &s.Timestamp)
		if err != nil {
			return
		}
		scores = append(scores, s)
	}

	return
}

func GetLastScore(db *sql.DB, teamID int) (s Score, err error) {

	stmt, err := db.Prepare("SELECT id, score, timestamp FROM score " +
		" WHERE team_id=$1 AND id = " +
		"(SELECT MAX(id) FROM score WHERE team_id=$1)")
	if err != nil {
		return
	}

	defer stmt.Close()

	err = stmt.QueryRow(teamID).Scan(&s.ID, &s.Score, &s.Timestamp)
	if err != nil {
		return
	}

	s.TeamID = teamID

	return
}
