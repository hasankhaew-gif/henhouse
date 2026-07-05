package db

import (
	"database/sql"
	"time"
)

type Flag struct {
	ID        int
	TeamID    int
	TaskID    int
	Flag      string
	Solved    bool
	Timestamp time.Time
}

func createFlagTable(db *sql.DB) (err error) {

	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS "flag" (
		id		SERIAL PRIMARY KEY,
		team_id		INTEGER NOT NULL,
		task_id		INTEGER NOT NULL,
		flag		TEXT NOT NULL,
		solved		BOOLEAN NOT NULL,
		timestamp	TIMESTAMP with time zone DEFAULT now()
	)`)

	return
}

func AddFlag(db *sql.DB, flag *Flag) (err error) {

	stmt, err := db.Prepare("INSERT INTO flag " +
		"(team_id, task_id, flag, solved) " +
		"VALUES ($1, $2, $3, $4) RETURNING id")
	if err != nil {
		return
	}

	defer stmt.Close()

	err = stmt.QueryRow(flag.TeamID, flag.TaskID, flag.Flag,
		flag.Solved).Scan(&flag.ID)
	if err != nil {
		return
	}

	return
}

func GetSolvedFlags(db *sql.DB) (flags []Flag, err error) {

	rows, err := db.Query("SELECT team_id, task_id, timestamp " +
		"FROM flag WHERE solved ORDER BY timestamp, id")
	if err != nil {
		return
	}

	defer rows.Close()

	for rows.Next() {
		var flag Flag
		err = rows.Scan(&flag.TeamID, &flag.TaskID, &flag.Timestamp)
		if err != nil {
			return
		}
		flags = append(flags, flag)
	}

	return
}

func GetFlags(db *sql.DB) (flags []Flag, err error) {

	rows, err := db.Query("SELECT id, team_id, task_id, flag, solved, " +
		"timestamp FROM flag")
	if err != nil {
		return
	}

	defer rows.Close()

	for rows.Next() {
		var f Flag

		err = rows.Scan(&f.ID, &f.TeamID, &f.TaskID, &f.Flag,
			&f.Solved, &f.Timestamp)
		if err != nil {
			return
		}

		flags = append(flags, f)
	}

	return
}

func GetLastFlags(db *sql.DB, limit int) (flags []Flag, err error) {

	stmt, err := db.Prepare("SELECT id, team_id, task_id, flag, solved, " +
		"timestamp FROM flag ORDER BY id DESC LIMIT $1")
	if err != nil {
		return
	}

	defer stmt.Close()

	rows, err := stmt.Query(limit)
	if err != nil {
		return
	}

	defer rows.Close()

	for rows.Next() {
		var f Flag

		err = rows.Scan(&f.ID, &f.TeamID, &f.TaskID, &f.Flag,
			&f.Solved, &f.Timestamp)
		if err != nil {
			return
		}

		flags = append(flags, f)
	}

	return
}

func CountFlags(db *sql.DB) (count int, err error) {
	err = db.QueryRow("SELECT count(*) FROM flag").Scan(&count)
	return
}

func DeleteFlagsByTask(db *sql.DB, taskID int) (err error) {

	stmt, err := db.Prepare("DELETE FROM flag WHERE task_id=$1")
	if err != nil {
		return
	}

	defer stmt.Close()

	_, err = stmt.Exec(taskID)
	if err != nil {
		return
	}

	return
}

func DeleteFlagsByTeam(db *sql.DB, teamID int) (err error) {

	stmt, err := db.Prepare("DELETE FROM flag WHERE team_id=$1")
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

func GetSolvedCount(db *sql.DB, taskID int) (count int, err error) {

	stmt, err := db.Prepare("SELECT count(*) FROM flag " +
		"WHERE task_id=$1 AND solved=TRUE")
	if err != nil {
		return
	}

	defer stmt.Close()

	err = stmt.QueryRow(taskID).Scan(&count)
	if err != nil {
		return
	}

	return
}

func IsSolved(db *sql.DB, teamID, taskID int) (solved bool, err error) {
	stmt, err := db.Prepare("SELECT EXISTS(SELECT id FROM flag " +
		"WHERE team_id=$1 AND task_id=$2 AND solved=TRUE)")

	if err != nil {
		return
	}

	defer stmt.Close()

	err = stmt.QueryRow(teamID, taskID).Scan(&solved)
	if err != nil {
		return
	}

	return
}

func GetSolvedBy(db *sql.DB, taskID int) (teamIDs []int, err error) {

	stmt, err := db.Prepare("SELECT team_id FROM flag " +
		"WHERE task_id=$1 AND solved=TRUE")
	if err != nil {
		return
	}

	defer stmt.Close()

	rows, err := stmt.Query(taskID)
	if err != nil {
		return
	}

	defer rows.Close()

	for rows.Next() {
		var teamID int

		err = rows.Scan(&teamID)
		if err != nil {
			return
		}

		teamIDs = append(teamIDs, teamID)
	}

	return
}
