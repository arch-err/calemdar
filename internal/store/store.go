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
	"strings"
	"time"

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
  root_path       TEXT NOT NULL UNIQUE,
  body_raw        TEXT
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
  expanded_at   TEXT,
  notify_json   TEXT
  -- series_id is a soft reference: store is a projection, reindex keeps it coherent.
);

CREATE INDEX IF NOT EXISTS occurrences_date     ON occurrences(date);
CREATE INDEX IF NOT EXISTS occurrences_series   ON occurrences(series_id);
CREATE INDEX IF NOT EXISTS occurrences_calendar ON occurrences(calendar);

-- notify_fired holds a row per (event_path, notify_index, planned fire_at)
-- so daemon restarts don't replay history. fired_at carries the
-- wall-clock time at which delivery happened.
CREATE TABLE IF NOT EXISTS notify_fired (
  event_path      TEXT NOT NULL,
  notify_index    INTEGER NOT NULL,
  fire_at_planned TEXT NOT NULL,
  fired_at        TEXT NOT NULL,
  PRIMARY KEY (event_path, notify_index, fire_at_planned)
);

CREATE INDEX IF NOT EXISTS notify_fired_path ON notify_fired(event_path);
`
	_, err := s.db.Exec(ddl)
	if err != nil {
		return err
	}
	// Best-effort migration for existing DBs that pre-date notify_json.
	// SQLite ignores ALTER TABLE ADD COLUMN errors when the column already
	// exists if we wrap and discard "duplicate column" specifically.
	if _, err := s.db.Exec(`ALTER TABLE occurrences ADD COLUMN notify_json TEXT`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	// Same for body_raw on series — added for the deletion-safeguard
	// snapshot. Pre-snapshot DBs still work; rows get filled on next
	// UpsertSeries / Reindex.
	if _, err := s.db.Exec(`ALTER TABLE series ADD COLUMN body_raw TEXT`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	return nil
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
			start_time, end_time, all_day, exceptions_json, root_path, body_raw)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
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
			root_path=excluded.root_path,
			body_raw=excluded.body_raw
	`,
		r.ID, r.Slug, r.Calendar, r.Title, r.Freq, r.Interval,
		string(bydayJSON), string(bymonthdayJSON),
		r.StartDate, nullIfEmpty(r.Until),
		nullIfEmpty(r.StartTime), nullIfEmpty(r.EndTime),
		boolToInt(r.AllDay), string(exceptionsJSON), r.Path,
		nullIfEmpty(r.RawSource),
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
		start_time, end_time, all_day, exceptions_json, root_path, body_raw
		FROM series WHERE id = ?`, id)
	return scanSeries(row, id)
}

// GetSeriesByPath returns a series whose root_path matches path, or nil.
// Used by the auto-restore flow when fsnotify fires a DELETE — at that
// point the file is gone, so there's no id to look up by.
func (s *Store) GetSeriesByPath(path string) (*model.Root, error) {
	row := s.db.QueryRow(`SELECT id, slug, calendar, title, freq, interval_n,
		byday_json, bymonthday_json, start_date, until_date,
		start_time, end_time, all_day, exceptions_json, root_path, body_raw
		FROM series WHERE root_path = ?`, path)

	var id string
	var r model.Root
	var bydayJSON, bymonthdayJSON, exceptionsJSON string
	var until, startTime, endTime, bodyRaw sql.NullString
	var allDay int
	if err := row.Scan(&id, &r.Slug, &r.Calendar, &r.Title, &r.Freq, &r.Interval,
		&bydayJSON, &bymonthdayJSON, &r.StartDate, &until,
		&startTime, &endTime, &allDay, &exceptionsJSON, &r.Path, &bodyRaw); err != nil {
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
	r.RawSource = bodyRaw.String
	_ = json.Unmarshal([]byte(bydayJSON), &r.ByDay)
	_ = json.Unmarshal([]byte(bymonthdayJSON), &r.ByMonthDay)
	_ = json.Unmarshal([]byte(exceptionsJSON), &r.Exceptions)
	return &r, nil
}

// ListSeries returns all series ordered by slug.
func (s *Store) ListSeries() ([]*model.Root, error) {
	rows, err := s.db.Query(`SELECT id, slug, calendar, title, freq, interval_n,
		byday_json, bymonthday_json, start_date, until_date,
		start_time, end_time, all_day, exceptions_json, root_path, body_raw
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
		var until, startTime, endTime, bodyRaw sql.NullString
		var allDay int
		if err := rows.Scan(&id, &r.Slug, &r.Calendar, &r.Title, &r.Freq, &r.Interval,
			&bydayJSON, &bymonthdayJSON, &r.StartDate, &until,
			&startTime, &endTime, &allDay, &exceptionsJSON, &r.Path, &bodyRaw); err != nil {
			return nil, err
		}
		r.ID = id
		r.Until = until.String
		r.StartTime = startTime.String
		r.EndTime = endTime.String
		r.AllDay = allDay != 0
		r.RawSource = bodyRaw.String
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
	var notifyJSON any
	if len(e.Notify) > 0 {
		raw, err := json.Marshal(e.Notify)
		if err != nil {
			return fmt.Errorf("store: marshal notify: %w", err)
		}
		notifyJSON = string(raw)
	}
	_, err := s.db.Exec(`
		INSERT INTO occurrences (path, series_id, calendar, date, title,
			start_time, end_time, all_day, user_owned, expanded_at, notify_json)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(path) DO UPDATE SET
			series_id=excluded.series_id,
			calendar=excluded.calendar,
			date=excluded.date,
			title=excluded.title,
			start_time=excluded.start_time,
			end_time=excluded.end_time,
			all_day=excluded.all_day,
			user_owned=excluded.user_owned,
			expanded_at=excluded.expanded_at,
			notify_json=excluded.notify_json
	`,
		e.Path, nullIfEmpty(e.SeriesID), calendar, e.Date, e.Title,
		nullIfEmpty(e.StartTime), nullIfEmpty(e.EndTime),
		boolToInt(e.AllDay), boolToInt(e.UserOwned), nullIfEmpty(e.SeriesExpandedAt),
		notifyJSON,
	)
	return err
}

// DeleteOccurrence removes the row matching path.
func (s *Store) DeleteOccurrence(path string) error {
	_, err := s.db.Exec(`DELETE FROM occurrences WHERE path = ?`, path)
	return err
}

// GetOccurrenceByPath returns the cached row for path, or nil if not
// present. Used by the sticky-delete dispatch path: the file is gone by
// the time fsnotify fires, so we look the metadata up from the cache
// before dropping the row.
func (s *Store) GetOccurrenceByPath(path string) (*model.Event, error) {
	row := s.db.QueryRow(`SELECT path, series_id, date, title,
		start_time, end_time, all_day, user_owned, expanded_at
		FROM occurrences WHERE path = ?`, path)
	var e model.Event
	var seriesID, startTime, endTime, expandedAt sql.NullString
	var allDay, userOwned int
	if err := row.Scan(&e.Path, &seriesID, &e.Date, &e.Title,
		&startTime, &endTime, &allDay, &userOwned, &expandedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	e.SeriesID = seriesID.String
	e.StartTime = startTime.String
	e.EndTime = endTime.String
	e.AllDay = allDay != 0
	e.UserOwned = userOwned != 0
	e.SeriesExpandedAt = expandedAt.String
	e.Type = "single"
	return &e, nil
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

// UpcomingRow is a scheduler-friendly row: the event payload plus the
// calendar (which Event itself doesn't carry) for tag/log purposes.
type UpcomingRow struct {
	Event    *model.Event
	Calendar string
}

// ListUpcomingWithNotify returns non-all-day occurrences whose
// "date T start_time" falls within [from, to] AND that have at least
// one notify rule. calendars filters to the given set; empty means all.
// Timestamps are parsed in model.Location() (the configured timezone).
//
// Limitation (by design): all-day events are skipped — there's no
// obvious trigger-time for them, and the scheduler cares about
// "about to start" windows.
func (s *Store) ListUpcomingWithNotify(from, to time.Time, calendars []string) ([]UpcomingRow, error) {
	// Coarse pre-filter on date, then fine-filter in-Go by parsing
	// start_time. SQLite's lexicographic comparison on YYYY-MM-DD is
	// correct; we widen by a day on each side to cover events whose
	// local time straddles midnight when from/to do.
	fromDate := from.AddDate(0, 0, -1).Format("2006-01-02")
	toDate := to.AddDate(0, 0, 1).Format("2006-01-02")

	query := `SELECT path, series_id, calendar, date, title,
		start_time, end_time, all_day, user_owned, expanded_at, notify_json
		FROM occurrences
		WHERE date >= ? AND date <= ? AND all_day = 0
		AND notify_json IS NOT NULL AND notify_json != ''`
	args := []any{fromDate, toDate}

	if len(calendars) > 0 {
		placeholders := strings.Repeat("?,", len(calendars))
		placeholders = placeholders[:len(placeholders)-1]
		query += " AND calendar IN (" + placeholders + ")"
		for _, c := range calendars {
			args = append(args, c)
		}
	}
	query += " ORDER BY date, start_time"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	loc := model.Location()
	var out []UpcomingRow
	for rows.Next() {
		var e model.Event
		var seriesID, startTime, endTime, expandedAt, notifyJSON sql.NullString
		var calendar string
		var allDay, userOwned int
		if err := rows.Scan(&e.Path, &seriesID, &calendar, &e.Date, &e.Title,
			&startTime, &endTime, &allDay, &userOwned, &expandedAt, &notifyJSON); err != nil {
			return nil, err
		}
		e.SeriesID = seriesID.String
		e.StartTime = startTime.String
		e.EndTime = endTime.String
		e.AllDay = allDay != 0
		e.UserOwned = userOwned != 0
		e.SeriesExpandedAt = expandedAt.String
		e.Type = "single"

		if notifyJSON.String != "" {
			if err := json.Unmarshal([]byte(notifyJSON.String), &e.Notify); err != nil {
				continue // skip malformed rows
			}
		}
		if len(e.Notify) == 0 {
			continue
		}
		if e.StartTime == "" {
			continue
		}
		ts, err := time.ParseInLocation("2006-01-02 15:04", e.Date+" "+e.StartTime, loc)
		if err != nil {
			continue
		}
		if ts.Before(from) || ts.After(to) {
			continue
		}
		out = append(out, UpcomingRow{Event: &e, Calendar: calendar})
	}
	return out, rows.Err()
}

// IsFired reports whether (eventPath, notifyIdx, fireAt) is already
// recorded in notify_fired. fireAt is canonicalised to RFC3339 (UTC) so
// callers don't have to worry about formatting.
func (s *Store) IsFired(eventPath string, notifyIdx int, fireAt time.Time) (bool, error) {
	key := fireAt.UTC().Format(time.RFC3339)
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM notify_fired WHERE event_path = ? AND notify_index = ? AND fire_at_planned = ?`,
		eventPath, notifyIdx, key,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// RecordFired inserts a row into notify_fired. Idempotent — duplicate
// inserts are ignored.
func (s *Store) RecordFired(eventPath string, notifyIdx int, fireAt, firedAt time.Time) error {
	planned := fireAt.UTC().Format(time.RFC3339)
	actual := firedAt.UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO notify_fired (event_path, notify_index, fire_at_planned, fired_at)
		 VALUES (?,?,?,?)`,
		eventPath, notifyIdx, planned, actual,
	)
	return err
}

// PruneFired deletes notify_fired rows older than cutoff. Called by the
// nightly loop to keep the table from growing unbounded.
func (s *Store) PruneFired(cutoff time.Time) (int64, error) {
	c := cutoff.UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`DELETE FROM notify_fired WHERE fire_at_planned < ?`, c)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
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
	var until, startTime, endTime, bodyRaw sql.NullString
	var allDay int
	if err := row.Scan(&r.Slug, &r.Calendar, &r.Title, &r.Freq, &r.Interval,
		&bydayJSON, &bymonthdayJSON, &r.StartDate, &until,
		&startTime, &endTime, &allDay, &exceptionsJSON, &r.Path, &bodyRaw); err != nil {
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
	r.RawSource = bodyRaw.String
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
