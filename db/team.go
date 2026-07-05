package db

import (
	"database/sql"
)

type Team struct {
	ID    int
	Name  string
	Email string
	Desc  string
	Token string
	Test  bool
}

func createTeamTable(db *sql.DB) (err error) {

	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS "team" (
		id		SERIAL PRIMARY KEY,
		name		TEXT NOT NULL,
		email		TEXT NOT NULL,
		description	TEXT NOT NULL,
		token		TEXT NOT NULL,
		test		BOOLEAN NOT NULL
	)`)

	return
}

func AddTeam(db *sql.DB, t *Team) (err error) {

	stmt, err := db.Prepare("INSERT INTO team (name, email, " +
		"description, token, test) " +
		"VALUES ($1, $2, $3, $4, $5) RETURNING id")
	if err != nil {
		return
	}

	defer stmt.Close()

	err = stmt.QueryRow(t.Name, t.Email, t.Desc, t.Token,
		t.Test).Scan(&t.ID)
	if err != nil {
		return
	}

	return
}

func GetTeams(db *sql.DB) (teams []Team, err error) {

	rows, err := db.Query("SELECT id, name, email, description, token, " +
		"test FROM team")
	if err != nil {
		return
	}

	defer rows.Close()

	for rows.Next() {
		var t Team

		err = rows.Scan(&t.ID, &t.Name, &t.Email, &t.Desc, &t.Token,
			&t.Test)
		if err != nil {
			return
		}

		teams = append(teams, t)
	}

	return
}

func GetTeam(db *sql.DB, teamID int) (t Team, err error) {

	stmt, err := db.Prepare("SELECT id, name, email, description, " +
		"token, test FROM team WHERE id=$1")
	if err != nil {
		return
	}

	defer stmt.Close()

	err = stmt.QueryRow(teamID).Scan(&t.ID, &t.Name, &t.Email, &t.Desc,
		&t.Token, &t.Test)
	if err != nil {
		return
	}

	return
}

func UpdateTeam(db *sql.DB, t *Team) (err error) {

	stmt, err := db.Prepare("UPDATE team SET name=$1, email=$2, " +
		"description=$3, token=$4, test=$5 WHERE id=$6")
	if err != nil {
		return
	}

	defer stmt.Close()

	_, err = stmt.Exec(t.Name, t.Email, t.Desc, t.Token, t.Test, t.ID)
	if err != nil {
		return
	}

	return
}

func DeleteTeam(db *sql.DB, teamID int) (err error) {

	stmt, err := db.Prepare("DELETE FROM team WHERE id=$1")
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

func GetTeamIDByToken(db *sql.DB, token string) (teamID int, err error) {

	stmt, err := db.Prepare("SELECT id FROM team WHERE token=$1")
	if err != nil {
		return
	}

	defer stmt.Close()

	err = stmt.QueryRow(token).Scan(&teamID)
	if err != nil {
		return
	}

	return
}
