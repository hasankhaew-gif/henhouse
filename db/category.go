package db

import "database/sql"

type Category struct {
	ID   int
	Name string
}

func createCategoryTable(db *sql.DB) (err error) {

	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS "category" (
		id	SERIAL PRIMARY KEY,
		name	TEXT NOT NULL UNIQUE
	)`)

	return
}

func AddCategory(db *sql.DB, category *Category) (err error) {

	stmt, err := db.Prepare("INSERT INTO category (name) " +
		"VALUES ($1) RETURNING id")
	if err != nil {
		return
	}

	defer stmt.Close()

	err = stmt.QueryRow(category.Name).Scan(&category.ID)
	if err != nil {
		return
	}

	return
}

func DeleteCategory(db *sql.DB, categoryID int) (err error) {

	stmt, err := db.Prepare("DELETE FROM category WHERE id=$1")
	if err != nil {
		return
	}

	defer stmt.Close()

	_, err = stmt.Exec(categoryID)
	if err != nil {
		return
	}

	return
}

func GetCategories(db *sql.DB) (categories []Category, err error) {

	rows, err := db.Query("SELECT id, name FROM category")
	if err != nil {
		return
	}

	defer rows.Close()

	for rows.Next() {
		var category Category

		err = rows.Scan(&category.ID, &category.Name)
		if err != nil {
			return
		}

		categories = append(categories, category)
	}

	return
}
