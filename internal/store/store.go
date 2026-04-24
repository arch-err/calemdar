// Package store is the SQLite cache for calemdar. It is a *projection* of
// the markdown files on disk — never the source of truth. Rebuild via
// `calemdar reindex` if anything drifts.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/vault"

	_ "modernc.org/sqlite"
)

// DBFile is the relative path from the vault root to the cache.
const DBFile = ".calemdar/cache.db"

// Store is a handle to the cache DB.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the cache DB inside the vault and ensures schema.
func Open(v *vault.Vault) (*Store, error) {
	dbPath := filepath.Join(v.Root, DBFile)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("store: mkdir: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS series (
  id              TEXT PRIMARY KEY,
  slug            TEXT NOT NULL,
  calendar        TEXT NOT NULL,
  title           TEXT NOT NULL,
  freq            TEXT NOT NULL,
  interval_n      INTEGER NOT NULL,
  byday_json      TEXT,
  bymonthday_json TEXT,
  start_date      TEXT NOT NULL,
  until_date      TEXT,
  start_time      TEXT,
  end_time        TEXT,
  all_day         INTEGER NOT NULL,
  exceptions_json TEXT,
  root_path       TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS occurrences (
  path          TEXT PRIMARY KEY,
  series_id     TEXT,
  calendar      TEXT NOT NULL,
  date          TEXT NOT NULL,
  title         TEXT NOT NULL,
  start_time    TEXT,
  end_time      TEXT,
  all_day       INTEGER NOT NULL,
  user_owned    INTEGER NOT NULL,
  expanded_at   TEXT
  -- series_id is a soft reference: store is a projection, reindex keeps it coherent.
);

CREATE INDEX IF NOT EXISTS occurrences_date     ON occurrences(date);
CREATE INDEX IF NOT EXISTS occurrences_series   ON occurrences(series_id);
CREATE INDEX IF NOT EXISTS occurrences_calendar ON occurrences(calendar);
`
	_, err := s.db.Exec(ddl)
	return err
}

// ---------- series ----------

// UpsertSeries writes or replaces a series row matching r.ID.
func (s *Store) UpsertSeries(r *model.Root) error {
	bydayJSON, _ := json.Marshal(r.ByDay)
	bymonthdayJSON, _ := json.Marshal(r.ByMonthDay)
	exceptionsJSON, _ := json.Marshal(r.Exceptions)

	_, err := s.db.Exec(`
		INSERT INTO series (id, slug, calendar, title, freq, interval_n,
			byday_json, bymonthday_json, start_date, until_date,
			start_time, end_time, all_day, exceptions_json, root_path)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			slug=excluded.slug,
			calendar=excluded.calendar,
			title=excluded.title,
			freq=excluded.freq,
			interval_n=excluded.interval_n,
			byday_json=excluded.byday_json,
			bymonthday_json=excluded.bymonthday_json,
			start_date=excluded.start_date,
			until_date=excluded.until_date,
			start_time=excluded.start_time,
			end_time=excluded.end_time,
			all_day=excluded.all_day,
			exceptions_json=excluded.exceptions_json,
			root_path=excluded.root_path
	`,
		r.ID, r.Slug, r.Calendar, r.Title, r.Freq, r.Interval,
		string(bydayJSON), string(bymonthdayJSON),
		r.StartDate, nullIfEmpty(r.Until),
		nullIfEmpty(r.StartTime), nullIfEmpty(r.EndTime),
		boolToInt(r.AllDay), string(exceptionsJSON), r.Path,
	)
	return err
}

// DeleteSeries removes a series and nullifies series_id on its occurrences.
func (s *Store) DeleteSeries(id string) error {
	_, err := s.db.Exec(`DELETE FROM series WHERE id = ?`, id)
	return err
}

// GetSeries returns a series by ID, or nil if not found.
func (s *Store) GetSeries(id string) (*model.Root, error) {
	row := s.db.QueryRow(`SELECT slug, calendar, title, freq, interval_n,
		byday_json, bymonthday_json, start_date, until_date,
		start_time, end_time, all_day, exceptions_json, root_path
		FROM series WHERE id = ?`, id)
	return scanSeries(row, id)
}

// ListSeries returns all series ordered by slug.
func (s *Store) ListSeries() ([]*model.Root, error) {
	rows, err := s.db.Query(`SELECT id, slug, calendar, title, freq, interval_n,
		byday_json, bymonthday_json, start_date, until_date,
		start_time, end_time, all_day, exceptions_json, root_path
		FROM series ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Root
	for rows.Next() {
		var id string
		var r model.Root
		var bydayJSON, bymonthdayJSON, exceptionsJSON string
		var until, startTime, endTime sql.NullString
		var allDay int
		if err := rows.Scan(&id, &r.Slug, &r.Calendar, &r.Title, &r.Freq, &r.Interval,
			&bydayJSON, &bymonthdayJSON, &r.StartDate, &until,
			&startTime, &endTime, &allDay, &exceptionsJSON, &r.Path); err != nil {
			return nil, err
		}
		r.ID = id
		r.Until = until.String
		r.StartTime = startTime.String
		r.EndTime = endTime.String
		r.AllDay = allDay != 0
		_ = json.Unmarshal([]byte(bydayJSON), &r.ByDay)
		_ = json.Unmarshal([]byte(bymonthdayJSON), &r.ByMonthDay)
		_ = json.Unmarshal([]byte(exceptionsJSON), &r.Exceptions)
		out = append(out, &r)
	}
	return out, rows.Err()
}

// ---------- occurrences ----------

// UpsertOccurrence writes or replaces an occurrence row matching e.Path.
// calendar is the top-level events/<calendar>/ folder.
func (s *Store) UpsertOccurrence(e *model.Event, calendar string) error {
	_, err := s.db.Exec(`
		INSERT INTO occurrences (path, series_id, calendar, date, title,
			start_time, end_time, all_day, user_owned, expanded_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(path) DO UPDATE SET
			series_id=excluded.series_id,
			calendar=excluded.calendar,
			date=excluded.date,
			title=excluded.title,
			start_time=excluded.start_time,
			end_time=excluded.end_time,
			all_day=excluded.all_day,
			user_owned=excluded.user_owned,
			expanded_at=excluded.expanded_at
	`,
		e.Path, nullIfEmpty(e.SeriesID), calendar, e.Date, e.Title,
		nullIfEmpty(e.StartTime), nullIfEmpty(e.EndTime),
		boolToInt(e.AllDay), boolToInt(e.UserOwned), nullIfEmpty(e.SeriesExpandedAt),
	)
	return err
}

// DeleteOccurrence removes the row matching path.
func (s *Store) DeleteOccurrence(path string) error {
	_, err := s.db.Exec(`DELETE FROM occurrences WHERE path = ?`, path)
	return err
}

// ListOccurrencesInRange returns occurrences with from <= date <= to.
// Dates are YYYY-MM-DD strings (lexicographic comparison is correct).
func (s *Store) ListOccurrencesInRange(from, to string) ([]*model.Event, error) {
	rows, err := s.db.Query(`SELECT path, series_id, date, title,
		start_time, end_time, all_day, user_owned, expanded_at
		FROM occurrences WHERE date >= ? AND date <= ? ORDER BY date, start_time`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Event
	for rows.Next() {
		var e model.Event
		var seriesID, startTime, endTime, expandedAt sql.NullString
		var allDay, userOwned int
		if err := rows.Scan(&e.Path, &seriesID, &e.Date, &e.Title,
			&startTime, &endTime, &allDay, &userOwned, &expandedAt); err != nil {
			return nil, err
		}
		e.SeriesID = seriesID.String
		e.StartTime = startTime.String
		e.EndTime = endTime.String
		e.AllDay = allDay != 0
		e.UserOwned = userOwned != 0
		e.SeriesExpandedAt = expandedAt.String
		e.Type = "single"
		out = append(out, &e)
	}
	return out, rows.Err()
}

// Wipe truncates both tables. Used by reindex.
func (s *Store) Wipe() error {
	if _, err := s.db.Exec(`DELETE FROM occurrences`); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM series`)
	return err
}

// ---------- helpers ----------

func scanSeries(row *sql.Row, id string) (*model.Root, error) {
	var r model.Root
	var bydayJSON, bymonthdayJSON, exceptionsJSON string
	var until, startTime, endTime sql.NullString
	var allDay int
	if err := row.Scan(&r.Slug, &r.Calendar, &r.Title, &r.Freq, &r.Interval,
		&bydayJSON, &bymonthdayJSON, &r.StartDate, &until,
		&startTime, &endTime, &allDay, &exceptionsJSON, &r.Path); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	r.ID = id
	r.Until = until.String
	r.StartTime = startTime.String
	r.EndTime = endTime.String
	r.AllDay = allDay != 0
	_ = json.Unmarshal([]byte(bydayJSON), &r.ByDay)
	_ = json.Unmarshal([]byte(bymonthdayJSON), &r.ByMonthDay)
	_ = json.Unmarshal([]byte(exceptionsJSON), &r.Exceptions)
	return &r, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
