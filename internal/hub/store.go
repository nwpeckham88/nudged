package hub

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

// Store handles persistence for the Hub.
type Store struct {
	db *sql.DB
}

// NewStore opens (or creates) the SQLite database.
func NewStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS agents (
		id TEXT PRIMARY KEY,
		name TEXT,
		addr TEXT,
		last_seen INTEGER,
		apps_json TEXT
	);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *Store) SaveAgent(a *Agent) error {
	appsBytes, err := json.Marshal(a.Apps)
	if err != nil {
		return err
	}

	query := `
	INSERT INTO agents (id, name, addr, last_seen, apps_json)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		name = excluded.name,
		addr = excluded.addr,
		last_seen = excluded.last_seen,
		apps_json = excluded.apps_json;
	`
	_, err = s.db.Exec(query, a.ID, a.Name, a.Addr, a.LastSeen, string(appsBytes))
	return err
}

func (s *Store) LoadAgents() (map[string]*Agent, error) {
	rows, err := s.db.Query("SELECT id, name, addr, last_seen, apps_json FROM agents")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agents := make(map[string]*Agent)
	for rows.Next() {
		var a Agent
		var appsJSON string
		if err := rows.Scan(&a.ID, &a.Name, &a.Addr, &a.LastSeen, &appsJSON); err != nil {
			slog.Warn("failed to scan agent row", "error", err)
			continue
		}
		if err := json.Unmarshal([]byte(appsJSON), &a.Apps); err != nil {
			slog.Warn("failed to unmarshal apps", "agent_id", a.ID, "error", err)
			continue
		}
		agents[a.ID] = &a
	}
	return agents, nil
}

func (s *Store) DeleteAgent(id string) error {
	_, err := s.db.Exec("DELETE FROM agents WHERE id = ?", id)
	return err
}
