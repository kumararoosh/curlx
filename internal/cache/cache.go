package cache

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/adrg/xdg"
	_ "modernc.org/sqlite"
)

// Body is a saved HTTP request body.
type Body struct {
	ID          int64
	Name        string
	OperationID string // scoped to a spec endpoint; empty = global
	Content     string
	UpdatedAt   time.Time
}

// Cache stores named request bodies in a local SQLite database.
type Cache struct {
	db *sql.DB
}

// Open opens (or creates) the request body cache.
func Open() (*Cache, error) {
	dbPath, err := xdg.DataFile("curlx/cache.db")
	if err != nil {
		return nil, fmt.Errorf("resolving cache path: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening cache db: %w", err)
	}

	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrating cache db: %w", err)
	}

	return &Cache{db: db}, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS bodies (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			name         TEXT NOT NULL,
			operation_id TEXT NOT NULL DEFAULT '',
			content      TEXT NOT NULL,
			updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(name, operation_id)
		)
	`)
	return err
}

// Save inserts or replaces a named body. Use operationID="" for global bodies.
func (c *Cache) Save(name, operationID, content string) error {
	_, err := c.db.Exec(`
		INSERT INTO bodies (name, operation_id, content, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(name, operation_id) DO UPDATE SET
			content    = excluded.content,
			updated_at = excluded.updated_at
	`, name, operationID, content)
	return err
}

// List returns all bodies scoped to an operationID, plus all global bodies.
func (c *Cache) List(operationID string) ([]Body, error) {
	rows, err := c.db.Query(`
		SELECT id, name, operation_id, content, updated_at
		FROM bodies
		WHERE operation_id = ? OR operation_id = ''
		ORDER BY operation_id DESC, updated_at DESC
	`, operationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bodies []Body
	for rows.Next() {
		var b Body
		if err := rows.Scan(&b.ID, &b.Name, &b.OperationID, &b.Content, &b.UpdatedAt); err != nil {
			return nil, err
		}
		bodies = append(bodies, b)
	}
	return bodies, rows.Err()
}

// Promote copies a scoped body to a global one (operationID="").
func (c *Cache) Promote(id int64) error {
	_, err := c.db.Exec(`
		INSERT INTO bodies (name, operation_id, content, updated_at)
		SELECT name || ' (global)', '', content, CURRENT_TIMESTAMP
		FROM bodies WHERE id = ?
		ON CONFLICT(name, operation_id) DO NOTHING
	`, id)
	return err
}

// Delete removes a body by ID.
func (c *Cache) Delete(id int64) error {
	_, err := c.db.Exec(`DELETE FROM bodies WHERE id = ?`, id)
	return err
}

// Close closes the underlying database connection.
func (c *Cache) Close() error {
	return c.db.Close()
}
