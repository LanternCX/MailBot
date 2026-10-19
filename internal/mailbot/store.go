package mailbot

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Email struct {
	ID       string
	Account  string
	Folder   string
	UID      uint32
	Subject  string
	Sender   string
	Date     time.Time
	Body     string
	HTMLBody string
	Images   []InlineImage `json:",omitempty"`
	WebLink  string
}

type InlineImage struct {
	CID  string
	Type string
	Data []byte
}

type Store struct {
	db      *sql.DB
	path    string
	mu      sync.Mutex
	changed chan struct{}
}
type Delivery struct {
	ID       int64
	Email    Email
	Attempts int
	Part     int
	Text     string
	Priority bool
}

func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	f.Close()
	if err = os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS cursors (account TEXT, folder TEXT, validity INTEGER, uid INTEGER, PRIMARY KEY(account,folder));
CREATE TABLE IF NOT EXISTS emails (id TEXT PRIMARY KEY, data BLOB NOT NULL, created INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS deliveries (id INTEGER PRIMARY KEY, email TEXT NOT NULL, target TEXT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0, next INTEGER NOT NULL DEFAULT 0, done INTEGER NOT NULL DEFAULT 0, part INTEGER NOT NULL DEFAULT 0, text TEXT NOT NULL DEFAULT '', UNIQUE(email,target));
CREATE INDEX IF NOT EXISTS pending ON deliveries(target,done,next);
CREATE TABLE IF NOT EXISTS target_limits (target TEXT PRIMARY KEY, until INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	if err = migrateDeliveryPriority(db); err != nil {
		db.Close()
		return nil, err
	}
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS read_links (email TEXT PRIMARY KEY, token TEXT UNIQUE NOT NULL)`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS image_cache (key TEXT PRIMARY KEY, data BLOB NOT NULL, type TEXT NOT NULL, created INTEGER NOT NULL)`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS reading_ai (email TEXT NOT NULL, language TEXT NOT NULL, kind TEXT NOT NULL, data BLOB NOT NULL, PRIMARY KEY(email,language,kind))`); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, path: path, changed: make(chan struct{})}, nil
}

// Upgrade existing queues without discarding pending mail or send progress.
func migrateDeliveryPriority(db *sql.DB) error {
	rows, err := db.Query(`SELECT name FROM pragma_table_info('deliveries') WHERE name='priority'`)
	if err != nil {
		return err
	}
	exists := rows.Next()
	err = rows.Err()
	rows.Close()
	if err != nil || exists {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`ALTER TABLE deliveries ADD COLUMN priority INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	rows, err = tx.Query(`SELECT id,data FROM emails WHERE id IN (SELECT email FROM deliveries WHERE done=0)`)
	if err != nil {
		return err
	}
	var urgent []string
	for rows.Next() {
		var id string
		var data []byte
		if err = rows.Scan(&id, &data); err != nil {
			rows.Close()
			return err
		}
		var e Email
		if err = json.Unmarshal(data, &e); err != nil {
			rows.Close()
			return err
		}
		if verificationMail(e) {
			urgent = append(urgent, id)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range urgent {
		if _, err = tx.Exec(`UPDATE deliveries SET priority=1 WHERE email=? AND done=0`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Changes() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.changed
}

func (s *Store) notify() {
	s.mu.Lock()
	defer s.mu.Unlock()
	close(s.changed)
	s.changed = make(chan struct{})
}

func (s *Store) Close() error { return s.db.Close() }
func emailID(account, folder string, validity, uid uint32) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%d", account, folder, validity, uid)))
	return hex.EncodeToString(h[:16])
}
func (s *Store) Cursor(account, folder string) (uint32, uint32, bool, error) {
	var v, u uint32
	err := s.db.QueryRow(`SELECT validity,uid FROM cursors WHERE account=? AND folder=?`, account, folder).Scan(&v, &u)
	if err == sql.ErrNoRows {
		return 0, 0, false, nil
	}
	return v, u, err == nil, err
}
func (s *Store) Baseline(account, folder string, validity, uid uint32) error {
	_, err := s.db.Exec(`INSERT INTO cursors VALUES(?,?,?,?) ON CONFLICT(account,folder) DO UPDATE SET validity=excluded.validity,uid=excluded.uid`, account, folder, validity, uid)
	return err
}

// Commit advances the checkpoint only after every target has a durable delivery.
func (s *Store) Commit(account, folder string, validity, uid uint32, emails []Email, targets []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, e := range emails {
		b, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT OR IGNORE INTO emails VALUES(?,?,?)`, e.ID, b, time.Now().Unix()); err != nil {
			return err
		}
		for _, target := range targets {
			if _, err = tx.Exec(`INSERT OR IGNORE INTO deliveries(email,target,priority) VALUES(?,?,?)`, e.ID, target, verificationMail(e)); err != nil {
				return err
			}
		}
	}
	if _, err = tx.Exec(`INSERT INTO cursors VALUES(?,?,?,?) ON CONFLICT(account,folder) DO UPDATE SET validity=excluded.validity,uid=excluded.uid`, account, folder, validity, uid); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.notify()
	return nil
}
func (s *Store) Next(target string) (*Delivery, error) {
	return s.next(target, nil)
}

func (s *Store) NextLane(target string, priority bool) (*Delivery, error) {
	return s.next(target, &priority)
}

func (s *Store) next(target string, priority *bool) (*Delivery, error) {
	var d Delivery
	var data []byte
	now := time.Now().Unix()
	query := `SELECT d.id,e.data,d.attempts,d.part,d.text,d.priority FROM deliveries d JOIN emails e ON e.id=d.email WHERE d.target=? AND d.done=0 AND d.next<=? AND NOT EXISTS (SELECT 1 FROM target_limits l WHERE l.target=d.target AND l.until>?)`
	args := []any{target, now, now}
	if priority != nil {
		query += ` AND d.priority=?`
		args = append(args, *priority)
	}
	query += ` ORDER BY d.priority DESC,d.id LIMIT 1`
	err := s.db.QueryRow(query, args...).Scan(&d.ID, &data, &d.Attempts, &d.Part, &d.Text, &d.Priority)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(data, &d.Email)
	return &d, err
}

func (s *Store) TargetDelay(target string) (time.Duration, error) {
	var until int64
	err := s.db.QueryRow(`SELECT until FROM target_limits WHERE target=?`, target).Scan(&until)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return time.Until(time.Unix(until, 0)), nil
}

func (s *Store) LimitTarget(target string, delay time.Duration) error {
	_, err := s.db.Exec(`INSERT INTO target_limits(target,until) VALUES(?,?) ON CONFLICT(target) DO UPDATE SET until=MAX(target_limits.until,excluded.until)`, target, time.Now().Add(delay).Unix())
	return err
}
func (s *Store) SaveText(id int64, text string) error {
	_, err := s.db.Exec(`UPDATE deliveries SET text=? WHERE id=?`, text, id)
	return err
}
func (s *Store) SavePart(id int64, part int) error {
	_, err := s.db.Exec(`UPDATE deliveries SET part=? WHERE id=?`, part, id)
	return err
}
func (s *Store) Done(id int64) error {
	_, err := s.db.Exec(`UPDATE deliveries SET done=1 WHERE id=?`, id)
	return err
}
func (s *Store) Retry(id int64, delay time.Duration) error {
	_, err := s.db.Exec(`UPDATE deliveries SET attempts=attempts+1,next=? WHERE id=?`, time.Now().Add(delay).Unix(), id)
	return err
}

// Save both the failed delivery and the target cooldown atomically so a restart
// cannot send a different queued email during Telegram's retry_after window.
func (s *Store) RetryLimited(id int64, target string, delay time.Duration) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	until := time.Now().Add(delay).Unix()
	if _, err = tx.Exec(`UPDATE deliveries SET attempts=attempts+1,next=? WHERE id=?`, until, id); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO target_limits(target,until) VALUES(?,?) ON CONFLICT(target) DO UPDATE SET until=MAX(target_limits.until,excluded.until)`, target, until); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Email(id string) (Email, error) {
	var e Email
	var data []byte
	err := s.db.QueryRow(`SELECT data FROM emails WHERE id=?`, id).Scan(&data)
	if err != nil {
		return e, err
	}
	err = json.Unmarshal(data, &e)
	return e, err
}

func (s *Store) EmailForTarget(id, target string) (Email, error) {
	var e Email
	var data []byte
	err := s.db.QueryRow(`SELECT e.data FROM emails e JOIN deliveries d ON d.email=e.id WHERE e.id=? AND d.target=?`, id, target).Scan(&data)
	if err != nil {
		return e, err
	}
	err = json.Unmarshal(data, &e)
	return e, err
}
func (s *Store) Setting(key, fallback string) string {
	var v string
	if s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v) != nil {
		return fallback
	}
	return v
}
func (s *Store) Set(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}
func (s *Store) Pending() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM deliveries WHERE done=0`).Scan(&n)
	return n, err
}
func (s *Store) Cleanup() error {
	// Keep pending mail indefinitely; keep completed mail for callback access for 30 days.
	_, err := s.db.Exec(`DELETE FROM deliveries WHERE done=1 AND email IN (SELECT id FROM emails WHERE created<?)`, time.Now().Add(-30*24*time.Hour).Unix())
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM emails WHERE created<? AND id NOT IN (SELECT email FROM deliveries)`, time.Now().Add(-30*24*time.Hour).Unix())
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM read_links WHERE email NOT IN (SELECT id FROM emails)`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM reading_ai WHERE email NOT IN (SELECT id FROM emails)`)
	return err
}
