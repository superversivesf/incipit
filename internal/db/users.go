package db

import (
	"database/sql"
	"fmt"

	"github.com/jason/incipit/internal/models"
)

func (d *DB) CreateUser(username, passwordHash, role string) (int64, error) {
	result, err := d.db.Exec(
		`INSERT INTO users (username, password_hash, role)
		 VALUES (?, ?, ?)
		 ON CONFLICT(username) DO UPDATE SET password_hash = excluded.password_hash`,
		username, passwordHash, role,
	)
	if err != nil {
		return 0, fmt.Errorf("creating/updating user %s: %w", username, err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting user ID: %w", err)
	}
	return id, nil
}

func (d *DB) GetUser(username string) (*models.User, error) {
	var u models.User
	err := d.db.QueryRow(
		`SELECT id, username, password_hash, role, created FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Created)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user %s not found: %w", username, err)
		}
		return nil, fmt.Errorf("getting user %s: %w", username, err)
	}
	return &u, nil
}

func (d *DB) ListUsers() ([]models.User, error) {
	rows, err := d.db.Query(
		`SELECT id, username, password_hash, role, created FROM users ORDER BY username`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Created); err != nil {
			return nil, fmt.Errorf("scanning user row: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (d *DB) DeleteUser(username string) error {
	_, err := d.db.Exec(`DELETE FROM users WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("deleting user %s: %w", username, err)
	}
	return nil
}
