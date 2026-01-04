package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/sqlite3"
	"github.com/flaticols/perfkit/internal/models"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sqlx.DB
	goqu *goqu.Database
}

func New(dbPath string) (*Store, error) {
	db, err := sqlx.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	s := &Store{
		db:   db,
		goqu: goqu.New("sqlite3", db),
	}

	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	// Base schema - don't include session in initial CREATE TABLE for backwards compat
	schema := `
	CREATE TABLE IF NOT EXISTS profiles (
		id TEXT PRIMARY KEY,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		name TEXT NOT NULL,
		profile_type TEXT NOT NULL,
		project TEXT,
		tags TEXT,
		source TEXT,
		raw_data BLOB,
		raw_size INTEGER,
		profile_time DATETIME,
		duration_ns INTEGER,
		metrics TEXT,
		total_samples INTEGER,
		total_value INTEGER,
		k6_p95 REAL,
		k6_p99 REAL,
		k6_rps REAL,
		k6_error_rate REAL,
		k6_duration_ms INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_profiles_project ON profiles(project);
	CREATE INDEX IF NOT EXISTS idx_profiles_type ON profiles(profile_type);
	CREATE INDEX IF NOT EXISTS idx_profiles_created ON profiles(created_at DESC);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return err
	}

	// Migration: add session column if not exists
	s.db.Exec("ALTER TABLE profiles ADD COLUMN session TEXT")

	// Create session index after column exists
	s.db.Exec("CREATE INDEX IF NOT EXISTS idx_profiles_session ON profiles(session)")

	// Migration: add is_cumulative column if not exists
	s.db.Exec("ALTER TABLE profiles ADD COLUMN is_cumulative INTEGER DEFAULT 0")

	return nil
}

func (s *Store) SaveProfile(ctx context.Context, p *models.Profile) error {
	if err := p.MarshalTags(); err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}

	query := `
	INSERT INTO profiles (
		id, created_at, updated_at, name, profile_type, project, session, tags, source,
		raw_data, raw_size, is_cumulative, profile_time, duration_ns, metrics,
		total_samples, total_value, k6_p95, k6_p99, k6_rps, k6_error_rate, k6_duration_ms
	) VALUES (
		:id, :created_at, :updated_at, :name, :profile_type, :project, :session, :tags, :source,
		:raw_data, :raw_size, :is_cumulative, :profile_time, :duration_ns, :metrics,
		:total_samples, :total_value, :k6_p95, :k6_p99, :k6_rps, :k6_error_rate, :k6_duration_ms
	)`

	_, err := s.db.NamedExecContext(ctx, query, p)
	return err
}

func (s *Store) GetProfile(ctx context.Context, id string) (*models.Profile, error) {
	var p models.Profile
	err := s.db.GetContext(ctx, &p, "SELECT * FROM profiles WHERE id = ?", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("profile not found: %s", id)
		}
		return nil, err
	}

	if err := p.UnmarshalTags(); err != nil {
		return nil, fmt.Errorf("unmarshal tags: %w", err)
	}

	return &p, nil
}

func (s *Store) ListProfiles(ctx context.Context, limit, offset int, profileType, project string) ([]*models.Profile, error) {
	ds := s.goqu.From("profiles").
		Select("id", "created_at", "updated_at", "name", "profile_type", "project", "session", "tags", "source", "raw_size", "is_cumulative", "profile_time", "duration_ns", "total_samples", "total_value", "k6_p95", "k6_p99", "k6_rps", "k6_error_rate", "k6_duration_ms").
		Order(goqu.I("created_at").Desc()).
		Limit(uint(limit)).
		Offset(uint(offset))

	if profileType != "" {
		ds = ds.Where(goqu.I("profile_type").Eq(profileType))
	}
	if project != "" {
		ds = ds.Where(goqu.I("project").Eq(project))
	}

	query, args, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}

	var profiles []*models.Profile
	if err := s.db.SelectContext(ctx, &profiles, query, args...); err != nil {
		return nil, err
	}

	for _, p := range profiles {
		_ = p.UnmarshalTags()
	}

	return profiles, nil
}
