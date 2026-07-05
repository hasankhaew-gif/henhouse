package db

import (
	"database/sql"
	"time"
)

type News struct {
	ID        int
	Title     string
	Body      string
	Tag       string
	Timestamp time.Time
}

func createNewsTable(db *sql.DB) (err error) {

	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS "news" (
		id		SERIAL PRIMARY KEY,
		title		TEXT NOT NULL,
		body		TEXT NOT NULL,
		tag		TEXT NOT NULL,
		timestamp	TIMESTAMP with time zone DEFAULT now()
	)`)

	return
}

func AddNews(db *sql.DB, n *News) (err error) {

	stmt, err := db.Prepare("INSERT INTO news (title, body, tag) " +
		"VALUES ($1, $2, $3) RETURNING id")
	if err != nil {
		return
	}

	defer stmt.Close()

	err = stmt.QueryRow(n.Title, n.Body, n.Tag).Scan(&n.ID)
	if err != nil {
		return
	}

	return
}

func GetNews(db *sql.DB) (news []News, err error) {

	rows, err := db.Query("SELECT id, title, body, tag, timestamp " +
		"FROM news ORDER BY timestamp DESC, id DESC")
	if err != nil {
		return
	}

	defer rows.Close()

	for rows.Next() {
		var n News

		err = rows.Scan(&n.ID, &n.Title, &n.Body, &n.Tag, &n.Timestamp)
		if err != nil {
			return
		}

		news = append(news, n)
	}

	return
}

func GetNewsItem(db *sql.DB, newsID int) (n News, err error) {

	stmt, err := db.Prepare("SELECT id, title, body, tag, timestamp " +
		"FROM news WHERE id=$1")
	if err != nil {
		return
	}

	defer stmt.Close()

	err = stmt.QueryRow(newsID).Scan(&n.ID, &n.Title, &n.Body, &n.Tag,
		&n.Timestamp)
	if err != nil {
		return
	}

	return
}

func UpdateNews(db *sql.DB, n *News) (err error) {

	stmt, err := db.Prepare("UPDATE news SET title=$1, body=$2, tag=$3 " +
		"WHERE id=$4")
	if err != nil {
		return
	}

	defer stmt.Close()

	_, err = stmt.Exec(n.Title, n.Body, n.Tag, n.ID)
	if err != nil {
		return
	}

	return
}

func DeleteNews(db *sql.DB, newsID int) (err error) {

	stmt, err := db.Prepare("DELETE FROM news WHERE id=$1")
	if err != nil {
		return
	}

	defer stmt.Close()

	_, err = stmt.Exec(newsID)
	if err != nil {
		return
	}

	return
}
