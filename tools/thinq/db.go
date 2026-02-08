// tools/thinq/db.go
package thinq

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Device struct {
	DeviceID   string
	Alias      string
	DeviceType string
	Model      string
	RawJSON    string
	UpdatedAt  time.Time
}

type ThinqDB struct {
	db *sql.DB
}

func NewThinqDB(dbPath string) (*ThinqDB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}

	tdb := &ThinqDB{db: db}
	if err := tdb.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return tdb, nil
}

func (t *ThinqDB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS devices (
		device_id TEXT PRIMARY KEY,
		alias TEXT NOT NULL DEFAULT '',
		device_type TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		raw_json TEXT NOT NULL DEFAULT '',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS device_states (
		device_id TEXT PRIMARY KEY,
		state_json TEXT NOT NULL DEFAULT '',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := t.db.Exec(schema)
	return err
}

func (t *ThinqDB) Close() error {
	return t.db.Close()
}

func (t *ThinqDB) UpsertDevices(devices []Device) error {
	tx, err := t.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO devices (device_id, alias, device_type, model, raw_json, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(device_id) DO UPDATE SET
			alias = excluded.alias,
			device_type = excluded.device_type,
			model = excluded.model,
			raw_json = excluded.raw_json,
			updated_at = CURRENT_TIMESTAMP
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, d := range devices {
		if _, err := stmt.Exec(d.DeviceID, d.Alias, d.DeviceType, d.Model, d.RawJSON); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (t *ThinqDB) ListDevices() ([]Device, error) {
	rows, err := t.db.Query("SELECT device_id, alias, device_type, model, raw_json, updated_at FROM devices ORDER BY alias")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.DeviceID, &d.Alias, &d.DeviceType, &d.Model, &d.RawJSON, &d.UpdatedAt); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func (t *ThinqDB) ResolveDevice(nameOrID string) (string, error) {
	// Try case-insensitive alias match first
	var deviceID string
	err := t.db.QueryRow(
		"SELECT device_id FROM devices WHERE LOWER(alias) = LOWER(?)", nameOrID,
	).Scan(&deviceID)
	if err == nil {
		return deviceID, nil
	}

	// Try exact device_id match
	err = t.db.QueryRow(
		"SELECT device_id FROM devices WHERE device_id = ?", nameOrID,
	).Scan(&deviceID)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	return deviceID, err
}

func (t *ThinqDB) UpsertState(deviceID string, stateJSON string) error {
	_, err := t.db.Exec(`
		INSERT INTO device_states (device_id, state_json, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(device_id) DO UPDATE SET
			state_json = excluded.state_json,
			updated_at = CURRENT_TIMESTAMP
	`, deviceID, stateJSON)
	return err
}

func (t *ThinqDB) GetState(deviceID string) (string, time.Time, error) {
	var stateJSON string
	var updatedAt time.Time
	err := t.db.QueryRow(
		"SELECT state_json, updated_at FROM device_states WHERE device_id = ?", deviceID,
	).Scan(&stateJSON, &updatedAt)
	if err == sql.ErrNoRows {
		return "", time.Time{}, ErrNotFound
	}
	return stateJSON, updatedAt, err
}
