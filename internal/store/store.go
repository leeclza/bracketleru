// Package store nyimpen turnamen. Ada dua implementasi:
//   - FileStore: file JSON di folder lokal (buat development)
//   - SQLStore: database Postgres, mis. Neon (buat production di Vercel)
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"bracketleru/internal/bracket"

	_ "github.com/jackc/pgx/v5/stdlib" // daftarin driver "pgx"
)

var ErrNotFound = errors.New("turnamen tidak ditemukan")

// Store itu interface: handler web cuma tau method ini, gak peduli
// datanya disimpan di file atau database.
type Store interface {
	Get(ctx context.Context, id string) (*bracket.Tournament, error)
	Save(ctx context.Context, t *bracket.Tournament) error
	Delete(ctx context.Context, id string) error
	// ListByOwner balikin turnamen milik user (terbaru dulu, maks limit).
	ListByOwner(ctx context.Context, ownerID string, limit int) ([]*bracket.Tournament, error)
}

func sortNewest(ts []*bracket.Tournament, limit int) []*bracket.Tournament {
	sort.Slice(ts, func(i, j int) bool { return ts[i].CreatedAt.After(ts[j].CreatedAt) })
	if len(ts) > limit {
		ts = ts[:limit]
	}
	return ts
}

var validID = regexp.MustCompile(`^[a-z0-9]{4,32}$`)

// ---------- FileStore ----------

type FileStore struct {
	dir string
	mu  sync.Mutex
}

func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &FileStore{dir: dir}, nil
}

func (s *FileStore) path(id string) string { return filepath.Join(s.dir, id+".json") }

func (s *FileStore) Get(_ context.Context, id string) (*bracket.Tournament, error) {
	if !validID.MatchString(id) {
		return nil, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var t bracket.Tournament
	return &t, json.Unmarshal(data, &t)
}

func (s *FileStore) Save(_ context.Context, t *bracket.Tournament) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.WriteFile(s.path(t.ID), data, 0o644)
}

func (s *FileStore) Delete(_ context.Context, id string) error {
	if !validID.MatchString(id) {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.Remove(s.path(id))
}

func (s *FileStore) ListByOwner(_ context.Context, ownerID string, limit int) ([]*bracket.Tournament, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var out []*bracket.Tournament
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		var t bracket.Tournament
		if json.Unmarshal(data, &t) == nil && t.OwnerID == ownerID {
			out = append(out, &t)
		}
	}
	return sortNewest(out, limit), nil
}

// ---------- SQLStore (Postgres, mis. Neon) ----------

type SQLStore struct {
	db *sql.DB
}

// NewSQLStore nyambung ke Postgres.
// dsn contohnya "postgresql://user:pass@ep-xxx.neon.tech/neondb?sslmode=require".
func NewSQLStore(dsn string) (*SQLStore, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	// di serverless tiap instance cukup sedikit koneksi
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(time.Minute)

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS tournaments (
		id         TEXT PRIMARY KEY,
		owner_id   TEXT NOT NULL DEFAULT '',
		data       TEXT NOT NULL,
		updated_at BIGINT NOT NULL
	)`)
	if err != nil {
		return nil, err
	}
	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_tournaments_owner ON tournaments(owner_id)`); err != nil {
		return nil, err
	}
	return &SQLStore{db: db}, nil
}

func (s *SQLStore) Get(ctx context.Context, id string) (*bracket.Tournament, error) {
	if !validID.MatchString(id) {
		return nil, ErrNotFound
	}
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT data FROM tournaments WHERE id = $1`, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var t bracket.Tournament
	return &t, json.Unmarshal([]byte(data), &t)
}

func (s *SQLStore) Save(ctx context.Context, t *bracket.Tournament) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO tournaments (id, owner_id, data, updated_at) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (id) DO UPDATE SET owner_id = EXCLUDED.owner_id, data = EXCLUDED.data, updated_at = EXCLUDED.updated_at`,
		t.ID, t.OwnerID, string(data), time.Now().Unix())
	return err
}

func (s *SQLStore) ListByOwner(ctx context.Context, ownerID string, limit int) ([]*bracket.Tournament, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT data FROM tournaments WHERE owner_id = $1 ORDER BY updated_at DESC LIMIT $2`, ownerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*bracket.Tournament
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var t bracket.Tournament
		if json.Unmarshal([]byte(data), &t) == nil {
			out = append(out, &t)
		}
	}
	return sortNewest(out, limit), rows.Err()
}

func (s *SQLStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM tournaments WHERE id = $1`, id)
	return err
}
