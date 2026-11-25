// Copyright 2022 Tailscale Inc & Contributors
// SPDX-License-Identifier: BSD-3-Clause

package golink

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
	"tailscale.com/tstime"
)

// Link is the structure stored for each go short link.
type Link struct {
	Description string `json:",omitempty"` // optional plain-text explanation
	CreatedBy   string `json:",omitempty"` // original creator; empty for legacy links
	Short       string // the "foo" part of http://go/foo
	Long        string // the target URL or text/template pattern to run
	Created     time.Time
	LastEdit    time.Time // when the link was last edited
	Owner       string    // user@domain
}

// ClickStats is the number of clicks a set of links have received in a given
// time period. It is keyed by link short name, with values of total clicks.
type ClickStats map[string]int

// linkID returns the normalized ID for a link short name.
func linkID(short string) string {
	id := url.PathEscape(strings.ToLower(short))
	id = strings.ReplaceAll(id, "-", "")
	return id
}

// SQLiteDB stores Links in a SQLite database.
type SQLiteDB struct {
	db *sql.DB
	mu sync.RWMutex

	clock tstime.Clock // allow overriding time for tests
}

//go:embed schema.sql
var sqlSchema string

// NewSQLiteDB returns a new SQLiteDB that stores links in a SQLite database stored at f.
func NewSQLiteDB(f string) (*SQLiteDB, error) {
	db, err := sql.Open("sqlite", f)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			db.Close()
		}
	}()
	if err := db.Ping(); err != nil {
		return nil, err
	}

	if _, err = db.Exec(sqlSchema); err != nil {
		return nil, err
	}

	// CREATE TABLE does not add columns to existing databases. Apply additive,
	// transactional migrations so older databases and backups remain readable.
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query("SELECT name FROM pragma_table_info('Links')")
	if err != nil {
		return nil, err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		columns[name] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"Description", "CreatedBy"} {
		if !columns[name] {
			if _, err := tx.Exec("ALTER TABLE Links ADD COLUMN " + name + " TEXT NOT NULL DEFAULT ''"); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	success = true
	return &SQLiteDB{db: db}, nil
}

// Now returns the current time.
func (s *SQLiteDB) Now() time.Time {
	return tstime.DefaultClock{Clock: s.clock}.Now()
}

// LoadAll returns all stored Links.
//
// The caller owns the returned values.
func (s *SQLiteDB) LoadAll() ([]*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var links []*Link
	rows, err := s.db.Query("SELECT Short, Long, Created, LastEdit, Owner, Description, CreatedBy FROM Links")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		link := new(Link)
		var created, lastEdit int64
		err := rows.Scan(&link.Short, &link.Long, &created, &lastEdit, &link.Owner, &link.Description, &link.CreatedBy)
		if err != nil {
			return nil, err
		}
		link.Created = time.Unix(created, 0).UTC()
		link.LastEdit = time.Unix(lastEdit, 0).UTC()
		links = append(links, link)
	}
	return links, rows.Err()
}

// Load returns a Link by its short name.
//
// It returns fs.ErrNotExist if the link does not exist.
//
// The caller owns the returned value.
func (s *SQLiteDB) Load(short string) (*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link := new(Link)
	var created, lastEdit int64
	row := s.db.QueryRow("SELECT Short, Long, Created, LastEdit, Owner, Description, CreatedBy FROM Links WHERE ID = ?1 LIMIT 1", linkID(short))
	err := row.Scan(&link.Short, &link.Long, &created, &lastEdit, &link.Owner, &link.Description, &link.CreatedBy)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = fs.ErrNotExist
		}
		return nil, err
	}
	link.Created = time.Unix(created, 0).UTC()
	link.LastEdit = time.Unix(lastEdit, 0).UTC()
	return link, nil
}

// Save saves a Link.
func (s *SQLiteDB) Save(link *Link) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec("INSERT OR REPLACE INTO Links (ID, Short, Long, Created, LastEdit, Owner, Description, CreatedBy) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", linkID(link.Short), link.Short, link.Long, link.Created.Unix(), link.LastEdit.Unix(), link.Owner, link.Description, link.CreatedBy)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("expected to affect 1 row, affected %d", rows)
	}
	return nil
}

// Delete removes a Link using its short name.
func (s *SQLiteDB) Delete(short string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec("DELETE FROM Links WHERE ID = ?", linkID(short))
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("expected to affect 1 row, affected %d", rows)
	}
	return nil
}

// LoadStats returns click stats for links.
func (s *SQLiteDB) LoadStats() (ClickStats, error) {
	allLinks, err := s.LoadAll()
	if err != nil {
		return nil, err
	}
	linkmap := make(map[string]string, len(allLinks)) // map ID => Short
	for _, link := range allLinks {
		linkmap[linkID(link.Short)] = link.Short
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query("SELECT ID, sum(Clicks) FROM Stats GROUP BY ID")
	if err != nil {
		return nil, err
	}
	stats := make(map[string]int)
	for rows.Next() {
		var id string
		var clicks int
		err := rows.Scan(&id, &clicks)
		if err != nil {
			return nil, err
		}
		short := linkmap[id]
		stats[short] = clicks
	}
	return stats, rows.Err()
}

// SaveStats records click stats for links.  The provided map includes
// incremental clicks that have occurred since the last time SaveStats
// was called.
func (s *SQLiteDB) SaveStats(stats ClickStats) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(context.TODO(), nil)
	if err != nil {
		return err
	}
	now := s.Now().Unix()
	for short, clicks := range stats {
		_, err := tx.Exec("INSERT INTO Stats (ID, Created, Clicks) VALUES (?, ?, ?)", linkID(short), now, clicks)
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// DeleteStats deletes click stats for a link.
func (s *SQLiteDB) DeleteStats(short string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM Stats WHERE ID = ?", linkID(short))
	if err != nil {
		return err
	}
	return nil
}

// GetLinksByOwner returns all Links owned by the specified owner.
func (s *SQLiteDB) GetLinksByOwner(owner string) ([]*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var links []*Link
	rows, err := s.db.Query("SELECT Short, Long, Created, LastEdit, Owner, Description, CreatedBy FROM Links WHERE LOWER(Owner) = LOWER(?)", owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		link := new(Link)
		var created, lastEdit int64
		err := rows.Scan(&link.Short, &link.Long, &created, &lastEdit, &link.Owner, &link.Description, &link.CreatedBy)
		if err != nil {
			return nil, err
		}
		link.Created = time.Unix(created, 0).UTC()
		link.LastEdit = time.Unix(lastEdit, 0).UTC()
		links = append(links, link)
	}
	return links, rows.Err()
}

// BackfillCreator attributes legacy links whose original creator was not recorded.
// Use only when an operator knows these links were all created by this person.
func (s *SQLiteDB) BackfillCreator(login string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(login) == "" {
		return errors.New("creator login is required")
	}
	_, err := s.db.Exec("UPDATE Links SET CreatedBy = ? WHERE CreatedBy = ''", login)
	return err
}
