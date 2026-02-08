# ThinQ Tool Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `thinq` tool that integrates with the LG ThinQ Connect API to list devices, check status, and control air conditioners.

**Architecture:** New `tools/thinq/` package with three files: `db.go` (SQLite caching), `client.go` (HTTP API client), `thinq.go` (tool interface implementation). Conditionally registered in `main.go` when `THINQ_TOKEN` env var is set.

**Tech Stack:** Go stdlib `net/http`, `database/sql`, `modernc.org/sqlite`, `encoding/json`

**Worktree:** `.worktrees/feature-thinq-tool`

---

### Task 1: ThinqDB — schema and device caching

**Files:**
- Create: `tools/thinq/db.go`
- Create: `tools/thinq/db_test.go`

**Step 1: Write the failing tests**

Create `tools/thinq/db_test.go`:

```go
// tools/thinq/db_test.go
package thinq

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestDB(t *testing.T) *ThinqDB {
	t.Helper()
	tmpDir := t.TempDir()
	db, err := NewThinqDB(filepath.Join(tmpDir, "thinq.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestThinqDB_UpsertAndListDevices(t *testing.T) {
	db := newTestDB(t)

	devices := []Device{
		{DeviceID: "dev-1", Alias: "Bedroom AC", DeviceType: "DEVICE_AIR_CONDITIONER", Model: "LG-AC-01", RawJSON: `{"deviceId":"dev-1"}`},
		{DeviceID: "dev-2", Alias: "Living Room AC", DeviceType: "DEVICE_AIR_CONDITIONER", Model: "LG-AC-02", RawJSON: `{"deviceId":"dev-2"}`},
	}
	if err := db.UpsertDevices(devices); err != nil {
		t.Fatalf("failed to upsert devices: %v", err)
	}

	got, err := db.ListDevices()
	if err != nil {
		t.Fatalf("failed to list devices: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(got))
	}
	if got[0].Alias != "Bedroom AC" {
		t.Errorf("expected alias 'Bedroom AC', got %q", got[0].Alias)
	}
}

func TestThinqDB_ResolveDevice_ByAlias(t *testing.T) {
	db := newTestDB(t)

	db.UpsertDevices([]Device{
		{DeviceID: "dev-1", Alias: "Bedroom AC", DeviceType: "DEVICE_AIR_CONDITIONER"},
	})

	id, err := db.ResolveDevice("bedroom ac")
	if err != nil {
		t.Fatalf("failed to resolve: %v", err)
	}
	if id != "dev-1" {
		t.Errorf("expected dev-1, got %s", id)
	}
}

func TestThinqDB_ResolveDevice_ByID(t *testing.T) {
	db := newTestDB(t)

	db.UpsertDevices([]Device{
		{DeviceID: "dev-1", Alias: "Bedroom AC", DeviceType: "DEVICE_AIR_CONDITIONER"},
	})

	id, err := db.ResolveDevice("dev-1")
	if err != nil {
		t.Fatalf("failed to resolve: %v", err)
	}
	if id != "dev-1" {
		t.Errorf("expected dev-1, got %s", id)
	}
}

func TestThinqDB_ResolveDevice_NotFound(t *testing.T) {
	db := newTestDB(t)

	_, err := db.ResolveDevice("unknown")
	if err == nil {
		t.Error("expected error for unknown device")
	}
}

func TestThinqDB_UpsertAndGetState(t *testing.T) {
	db := newTestDB(t)

	stateJSON := `{"operation":{"airConOperationMode":"POWER_ON"}}`
	if err := db.UpsertState("dev-1", stateJSON); err != nil {
		t.Fatalf("failed to upsert state: %v", err)
	}

	state, updatedAt, err := db.GetState("dev-1")
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}
	if state != stateJSON {
		t.Errorf("expected %q, got %q", stateJSON, state)
	}
	if time.Since(updatedAt) > time.Minute {
		t.Error("updated_at seems too old")
	}
}

func TestThinqDB_GetState_NotFound(t *testing.T) {
	db := newTestDB(t)

	_, _, err := db.GetState("unknown")
	if err == nil {
		t.Error("expected error for unknown device state")
	}
}
```

**Step 2: Create the minimal db.go to make tests compile but fail**

Create `tools/thinq/db.go` with struct and method stubs:

```go
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
```

**Step 3: Run tests to verify they pass**

Run: `go test ./tools/thinq/ -v`
Expected: All 6 tests PASS

**Step 4: Commit**

```bash
git add tools/thinq/db.go tools/thinq/db_test.go
git commit -m "feat(thinq): add ThinqDB with device and state caching"
```

---

### Task 2: API Client

**Files:**
- Create: `tools/thinq/client.go`
- Create: `tools/thinq/client_test.go`

**Step 1: Write the test (using httptest to mock the ThinQ API)**

Create `tools/thinq/client_test.go`:

```go
// tools/thinq/client_test.go
package thinq

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListDevices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/devices" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("x-country") != "US" {
			t.Errorf("unexpected country header: %s", r.Header.Get("x-country"))
		}

		json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{
				"devices": []map[string]any{
					{
						"deviceId": "dev-1",
						"deviceInfo": map[string]any{
							"alias":      "Bedroom AC",
							"deviceType": "DEVICE_AIR_CONDITIONER",
							"modelName":  "LG-AC-01",
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := NewClient("test-token", "US", "test-client")
	c.baseURL = srv.URL

	devices, err := c.ListDevices()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].DeviceID != "dev-1" {
		t.Errorf("expected device ID dev-1, got %s", devices[0].DeviceID)
	}
	if devices[0].Alias != "Bedroom AC" {
		t.Errorf("expected alias 'Bedroom AC', got %s", devices[0].Alias)
	}
}

func TestClient_GetState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/devices/dev-1/state" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{
				"operation": map[string]any{
					"airConOperationMode": "POWER_ON",
				},
				"temperature": map[string]any{
					"currentTemperature": 24.0,
					"targetTemperature":  22.0,
				},
			},
		})
	}))
	defer srv.Close()

	c := NewClient("test-token", "US", "test-client")
	c.baseURL = srv.URL

	state, err := c.GetState("dev-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	op, ok := state["operation"].(map[string]any)
	if !ok {
		t.Fatal("expected operation in state")
	}
	if op["airConOperationMode"] != "POWER_ON" {
		t.Errorf("expected POWER_ON, got %v", op["airConOperationMode"])
	}
}

func TestClient_Control(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/devices/dev-1/control" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("x-conditional-control") != "true" {
			t.Errorf("expected x-conditional-control header")
		}

		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		op, _ := body["operation"].(map[string]any)
		if op["airConOperationMode"] != "POWER_OFF" {
			t.Errorf("expected POWER_OFF in body, got %v", op["airConOperationMode"])
		}

		json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{
				"operation": map[string]any{
					"airConOperationMode": "POWER_OFF",
				},
			},
		})
	}))
	defer srv.Close()

	c := NewClient("test-token", "US", "test-client")
	c.baseURL = srv.URL

	cmd := map[string]any{
		"operation": map[string]any{
			"airConOperationMode": "POWER_OFF",
		},
	}
	result, err := c.Control("dev-1", cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	op, _ := result["operation"].(map[string]any)
	if op["airConOperationMode"] != "POWER_OFF" {
		t.Errorf("expected POWER_OFF in result")
	}
}

func TestClient_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "0101",
				"message": "Unauthorized",
			},
		})
	}))
	defer srv.Close()

	c := NewClient("bad-token", "US", "test-client")
	c.baseURL = srv.URL

	_, err := c.ListDevices()
	if err == nil {
		t.Error("expected error for unauthorized request")
	}
}

func TestBaseURLFromCountry(t *testing.T) {
	tests := []struct {
		country string
		want    string
	}{
		{"US", "https://api-aic.lgthinq.com"},
		{"BR", "https://api-aic.lgthinq.com"},
		{"CA", "https://api-aic.lgthinq.com"},
		{"GB", "https://api-eic.lgthinq.com"},
		{"DE", "https://api-eic.lgthinq.com"},
		{"KR", "https://api-kic.lgthinq.com"},
		{"JP", "https://api-kic.lgthinq.com"},
		{"AU", "https://api-kic.lgthinq.com"},
	}
	for _, tt := range tests {
		t.Run(tt.country, func(t *testing.T) {
			got := baseURLFromCountry(tt.country)
			if got != tt.want {
				t.Errorf("baseURLFromCountry(%q) = %q, want %q", tt.country, got, tt.want)
			}
		})
	}
}
```

**Step 2: Implement client.go**

Create `tools/thinq/client.go`:

```go
// tools/thinq/client.go
package thinq

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

const apiKey = "v6GFvkweNo7DK7yD3ylIZ9w52aKBU0eJ7wLXkSR3"

// americas countries (ThinQ API region: aic)
var americasCountries = map[string]bool{
	"US": true, "CA": true, "BR": true, "MX": true, "AR": true, "CL": true, "CO": true, "PE": true,
}

// asia-pacific countries (ThinQ API region: kic)
var apacCountries = map[string]bool{
	"KR": true, "JP": true, "AU": true, "NZ": true, "CN": true, "TW": true, "HK": true, "SG": true,
	"TH": true, "MY": true, "PH": true, "IN": true, "ID": true, "VN": true,
}

func baseURLFromCountry(country string) string {
	c := strings.ToUpper(country)
	if americasCountries[c] {
		return "https://api-aic.lgthinq.com"
	}
	if apacCountries[c] {
		return "https://api-kic.lgthinq.com"
	}
	// Default to Europe/Middle East/Africa
	return "https://api-eic.lgthinq.com"
}

type Client struct {
	token    string
	country  string
	clientID string
	baseURL  string
	http     *http.Client
}

func NewClient(token, country, clientID string) *Client {
	return &Client{
		token:    token,
		country:  strings.ToUpper(country),
		clientID: clientID,
		baseURL:  baseURLFromCountry(country),
		http:     &http.Client{},
	}
}

func (c *Client) setHeaders(req *http.Request) {
	msgID := base64.StdEncoding.EncodeToString([]byte(uuid.New().String()))
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("x-country", c.country)
	req.Header.Set("x-message-id", msgID)
	req.Header.Set("x-client-id", c.clientID)
	req.Header.Set("x-service-phase", "OP")
	req.Header.Set("Content-Type", "application/json")
}

func (c *Client) do(req *http.Request) (map[string]any, error) {
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("thinq api request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("thinq api error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	response, ok := result["response"]
	if !ok {
		return nil, fmt.Errorf("unexpected response format: missing 'response' field")
	}

	responseMap, ok := response.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected response format: 'response' is not an object")
	}

	return responseMap, nil
}

func (c *Client) ListDevices() ([]Device, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/devices", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}

	rawDevices, ok := resp["devices"].([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected response: missing 'devices' array")
	}

	var devices []Device
	for _, raw := range rawDevices {
		d, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		deviceID, _ := d["deviceId"].(string)
		info, _ := d["deviceInfo"].(map[string]any)
		alias, _ := info["alias"].(string)
		deviceType, _ := info["deviceType"].(string)
		model, _ := info["modelName"].(string)

		rawJSON, _ := json.Marshal(d)

		devices = append(devices, Device{
			DeviceID:   deviceID,
			Alias:      alias,
			DeviceType: deviceType,
			Model:      model,
			RawJSON:    string(rawJSON),
		})
	}

	return devices, nil
}

func (c *Client) GetState(deviceID string) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/devices/"+deviceID+"/state", nil)
	if err != nil {
		return nil, err
	}

	return c.do(req)
}

func (c *Client) Control(deviceID string, command map[string]any) (map[string]any, error) {
	body, err := json.Marshal(command)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/devices/"+deviceID+"/control", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-conditional-control", "true")

	return c.do(req)
}
```

**Note:** This requires adding `github.com/google/uuid` dependency. Run:
```bash
go get github.com/google/uuid
```

**Step 3: Run tests to verify they pass**

Run: `go test ./tools/thinq/ -v`
Expected: All tests PASS (db + client tests)

**Step 4: Commit**

```bash
git add tools/thinq/client.go tools/thinq/client_test.go go.mod go.sum
git commit -m "feat(thinq): add ThinQ API HTTP client"
```

---

### Task 3: Tool implementation (ThinqTool struct, schema, ParseArgs, Execute)

**Files:**
- Create: `tools/thinq/thinq.go`
- Create: `tools/thinq/thinq_test.go`

**Step 1: Write the tests**

Create `tools/thinq/thinq_test.go`:

```go
// tools/thinq/thinq_test.go
package thinq

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestTool(t *testing.T) (*ThinqTool, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/devices" && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"response": map[string]any{
					"devices": []map[string]any{
						{
							"deviceId": "dev-1",
							"deviceInfo": map[string]any{
								"alias":      "Bedroom AC",
								"deviceType": "DEVICE_AIR_CONDITIONER",
								"modelName":  "LG-AC-01",
							},
						},
						{
							"deviceId": "dev-2",
							"deviceInfo": map[string]any{
								"alias":      "Living Room AC",
								"deviceType": "DEVICE_AIR_CONDITIONER",
								"modelName":  "LG-AC-02",
							},
						},
					},
				},
			})

		case r.URL.Path == "/devices/dev-1/state" && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"response": map[string]any{
					"operation": map[string]any{
						"airConOperationMode": "POWER_ON",
					},
					"airConJobMode": map[string]any{
						"currentJobMode": "COOL",
					},
					"temperature": map[string]any{
						"currentTemperature": 26.0,
						"targetTemperature":  22.0,
					},
					"airFlow": map[string]any{
						"windStrength": "MED",
					},
				},
			})

		case r.URL.Path == "/devices/dev-1/control" && r.Method == http.MethodPost:
			json.NewEncoder(w).Encode(map[string]any{
				"response": map[string]any{
					"operation": map[string]any{
						"airConOperationMode": "POWER_OFF",
					},
				},
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	tmpDir := t.TempDir()
	db, err := NewThinqDB(filepath.Join(tmpDir, "thinq.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		srv.Close()
	})

	client := NewClient("test-token", "US", "test-client")
	client.baseURL = srv.URL

	tool := NewThinqTool(client, db)
	return tool, srv
}

func TestThinqTool_Name(t *testing.T) {
	tool, _ := setupTestTool(t)
	if tool.Name() != "thinq" {
		t.Errorf("expected 'thinq', got %q", tool.Name())
	}
}

func TestThinqTool_AdminOnly(t *testing.T) {
	tool, _ := setupTestTool(t)
	if tool.AdminOnly() {
		t.Error("expected AdminOnly to be false")
	}
}

func TestThinqTool_Devices(t *testing.T) {
	tool, _ := setupTestTool(t)

	result, err := tool.Execute(context.Background(), map[string]any{"command": "devices"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Bedroom AC") {
		t.Errorf("expected 'Bedroom AC' in result, got: %s", result)
	}
	if !strings.Contains(result, "Living Room AC") {
		t.Errorf("expected 'Living Room AC' in result, got: %s", result)
	}
}

func TestThinqTool_Status(t *testing.T) {
	tool, _ := setupTestTool(t)

	// First populate device cache
	tool.Execute(context.Background(), map[string]any{"command": "devices"})

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "status",
		"device":  "Bedroom AC",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "POWER_ON") || !strings.Contains(result, "COOL") {
		t.Errorf("expected state info in result, got: %s", result)
	}
}

func TestThinqTool_Power(t *testing.T) {
	tool, _ := setupTestTool(t)

	tool.Execute(context.Background(), map[string]any{"command": "devices"})

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "power",
		"device":  "Bedroom AC",
		"power":   "off",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "POWER_OFF") {
		t.Errorf("expected POWER_OFF in result, got: %s", result)
	}
}

func TestThinqTool_Set(t *testing.T) {
	tool, _ := setupTestTool(t)

	tool.Execute(context.Background(), map[string]any{"command": "devices"})

	result, err := tool.Execute(context.Background(), map[string]any{
		"command":     "set",
		"device":      "Bedroom AC",
		"temperature": 22.0,
		"mode":        "cool",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The mock returns POWER_OFF for all control, but we check it doesn't error
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestThinqTool_ParseArgs(t *testing.T) {
	tool, _ := setupTestTool(t)

	tests := []struct {
		name    string
		raw     string
		want    map[string]any
		wantErr bool
	}{
		{
			name:    "empty input",
			raw:     "",
			wantErr: true,
		},
		{
			name: "devices command",
			raw:  "devices",
			want: map[string]any{"command": "devices"},
		},
		{
			name: "status with device alias",
			raw:  "status bedroom ac",
			want: map[string]any{"command": "status", "device": "bedroom ac"},
		},
		{
			name: "power on",
			raw:  "power bedroom ac on",
			want: map[string]any{"command": "power", "device": "bedroom ac", "power": "on"},
		},
		{
			name: "power off",
			raw:  "power bedroom ac off",
			want: map[string]any{"command": "power", "device": "bedroom ac", "power": "off"},
		},
		{
			name: "set with temp and mode",
			raw:  "set bedroom ac --temp=22 --mode=cool",
			want: map[string]any{"command": "set", "device": "bedroom ac", "temperature": 22.0, "mode": "cool"},
		},
		{
			name: "set with all options",
			raw:  "set bedroom ac --temp=22 --mode=cool --fan_speed=high --display=off",
			want: map[string]any{"command": "set", "device": "bedroom ac", "temperature": 22.0, "mode": "cool", "fan_speed": "high", "display": "off"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tool.ParseArgs(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("key %q: got %v (%T), want %v (%T)", k, got[k], got[k], v, v)
				}
			}
		})
	}
}

func TestThinqTool_StatusFallback(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewThinqDB(filepath.Join(tmpDir, "thinq.db"))
	defer db.Close()

	// Pre-populate cache
	db.UpsertDevices([]Device{{DeviceID: "dev-1", Alias: "Bedroom AC", DeviceType: "DEVICE_AIR_CONDITIONER"}})
	db.UpsertState("dev-1", `{"operation":{"airConOperationMode":"POWER_ON"}}`)

	// Client pointing at non-existent server (will fail)
	client := NewClient("test-token", "US", "test-client")
	client.baseURL = "http://127.0.0.1:1"

	tool := NewThinqTool(client, db)

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "status",
		"device":  "Bedroom AC",
	})
	if err != nil {
		t.Fatalf("expected fallback, got error: %v", err)
	}
	if !strings.Contains(result, "outdated") {
		t.Errorf("expected 'outdated' warning in fallback result, got: %s", result)
	}
	if !strings.Contains(result, "POWER_ON") {
		t.Errorf("expected cached state in fallback result, got: %s", result)
	}
}
```

**Step 2: Implement thinq.go**

Create `tools/thinq/thinq.go`:

```go
// tools/thinq/thinq.go
package thinq

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type ThinqTool struct {
	client *Client
	db     *ThinqDB
}

func NewThinqTool(client *Client, db *ThinqDB) *ThinqTool {
	return &ThinqTool{client: client, db: db}
}

func (t *ThinqTool) Name() string        { return "thinq" }
func (t *ThinqTool) AdminOnly() bool      { return false }

func (t *ThinqTool) Description() string {
	return "Control LG ThinQ smart home devices. List devices, check status, and control air conditioners (power, temperature, mode, fan speed, display)."
}

func (t *ThinqTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"enum":        []string{"devices", "status", "power", "set"},
				"description": "The operation to perform",
			},
			"device": map[string]any{
				"type":        "string",
				"description": "Device ID or alias name",
			},
			"power": map[string]any{
				"type":        "string",
				"enum":        []string{"on", "off"},
				"description": "Power state (for power command)",
			},
			"temperature": map[string]any{
				"type":        "number",
				"description": "Target temperature in Celsius (for set command)",
			},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"cool", "heat", "auto", "dry", "fan"},
				"description": "Operation mode (for set command)",
			},
			"fan_speed": map[string]any{
				"type":        "string",
				"description": "Fan/wind strength (for set command)",
			},
			"display": map[string]any{
				"type":        "string",
				"enum":        []string{"on", "off"},
				"description": "Display light state (for set command)",
			},
		},
		"required": []string{"command"},
	}
}

func (t *ThinqTool) ParseArgs(raw string) (map[string]any, error) {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return nil, fmt.Errorf("missing arguments. Usage: /thinq <command> [device] [options]")
	}

	command := parts[0]
	result := map[string]any{"command": command}

	switch command {
	case "devices":
		return result, nil

	case "status":
		if len(parts) < 2 {
			return nil, fmt.Errorf("usage: /thinq status <device>")
		}
		result["device"] = strings.Join(parts[1:], " ")
		return result, nil

	case "power":
		if len(parts) < 3 {
			return nil, fmt.Errorf("usage: /thinq power <device> <on|off>")
		}
		// Last part is on/off, everything in between is the device name
		powerVal := parts[len(parts)-1]
		if powerVal != "on" && powerVal != "off" {
			return nil, fmt.Errorf("power must be 'on' or 'off', got %q", powerVal)
		}
		result["device"] = strings.Join(parts[1:len(parts)-1], " ")
		result["power"] = powerVal
		return result, nil

	case "set":
		if len(parts) < 2 {
			return nil, fmt.Errorf("usage: /thinq set <device> [--temp=N] [--mode=MODE] [--fan_speed=SPEED] [--display=on|off]")
		}
		// Separate device name parts from flag parts
		var deviceParts []string
		for _, p := range parts[1:] {
			if strings.HasPrefix(p, "--") {
				break
			}
			deviceParts = append(deviceParts, p)
		}
		if len(deviceParts) == 0 {
			return nil, fmt.Errorf("missing device name")
		}
		result["device"] = strings.Join(deviceParts, " ")

		// Parse flags
		for _, p := range parts[1+len(deviceParts):] {
			if strings.HasPrefix(p, "--temp=") {
				val, err := strconv.ParseFloat(strings.TrimPrefix(p, "--temp="), 64)
				if err != nil {
					return nil, fmt.Errorf("invalid temperature: %s", p)
				}
				result["temperature"] = val
			} else if strings.HasPrefix(p, "--mode=") {
				result["mode"] = strings.TrimPrefix(p, "--mode=")
			} else if strings.HasPrefix(p, "--fan_speed=") {
				result["fan_speed"] = strings.TrimPrefix(p, "--fan_speed=")
			} else if strings.HasPrefix(p, "--display=") {
				result["display"] = strings.TrimPrefix(p, "--display=")
			}
		}
		return result, nil

	default:
		return nil, fmt.Errorf("unknown command: %s. Available: devices, status, power, set", command)
	}
}

func (t *ThinqTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	command, _ := input["command"].(string)
	if command == "" {
		return "", fmt.Errorf("missing command")
	}

	switch command {
	case "devices":
		return t.execDevices()
	case "status":
		return t.execStatus(input)
	case "power":
		return t.execPower(input)
	case "set":
		return t.execSet(input)
	default:
		return "", fmt.Errorf("unknown command: %s", command)
	}
}

func (t *ThinqTool) execDevices() (string, error) {
	devices, err := t.client.ListDevices()
	if err != nil {
		// Fallback to cache
		cached, cacheErr := t.db.ListDevices()
		if cacheErr != nil || len(cached) == 0 {
			return "", fmt.Errorf("failed to list devices: %w", err)
		}
		return formatDeviceList(cached) + "\n(outdated — using cached data due to API error)", nil
	}

	if err := t.db.UpsertDevices(devices); err != nil {
		// Non-fatal: cache update failed but we still have fresh data
	}

	return formatDeviceList(devices), nil
}

func (t *ThinqTool) execStatus(input map[string]any) (string, error) {
	device, _ := input["device"].(string)
	if device == "" {
		return "", fmt.Errorf("missing device name or ID")
	}

	deviceID, err := t.db.ResolveDevice(device)
	if err != nil {
		return "", fmt.Errorf("device not found: %q (run /thinq devices first to refresh the device list)", device)
	}

	state, apiErr := t.client.GetState(deviceID)
	if apiErr != nil {
		// Fallback to cached state
		cachedJSON, updatedAt, cacheErr := t.db.GetState(deviceID)
		if cacheErr != nil {
			return "", fmt.Errorf("failed to get device status: %w", apiErr)
		}
		var cachedState map[string]any
		if err := json.Unmarshal([]byte(cachedJSON), &cachedState); err != nil {
			return "", fmt.Errorf("failed to parse cached state: %w", err)
		}
		return formatState(deviceID, cachedState) + fmt.Sprintf("\n(outdated — last updated: %s)", updatedAt.Format("2006-01-02 15:04:05")), nil
	}

	// Cache the fresh state
	stateJSON, _ := json.Marshal(state)
	t.db.UpsertState(deviceID, string(stateJSON))

	return formatState(deviceID, state), nil
}

func (t *ThinqTool) execPower(input map[string]any) (string, error) {
	device, _ := input["device"].(string)
	power, _ := input["power"].(string)
	if device == "" || power == "" {
		return "", fmt.Errorf("missing device or power value")
	}

	deviceID, err := t.db.ResolveDevice(device)
	if err != nil {
		return "", fmt.Errorf("device not found: %q", device)
	}

	mode := "POWER_ON"
	if power == "off" {
		mode = "POWER_OFF"
	}

	cmd := map[string]any{
		"operation": map[string]any{
			"airConOperationMode": mode,
		},
	}

	result, err := t.client.Control(deviceID, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to control device: %w", err)
	}

	stateJSON, _ := json.Marshal(result)
	t.db.UpsertState(deviceID, string(stateJSON))

	return fmt.Sprintf("Device %q powered %s.\n%s", device, power, formatState(deviceID, result)), nil
}

func (t *ThinqTool) execSet(input map[string]any) (string, error) {
	device, _ := input["device"].(string)
	if device == "" {
		return "", fmt.Errorf("missing device name or ID")
	}

	deviceID, err := t.db.ResolveDevice(device)
	if err != nil {
		return "", fmt.Errorf("device not found: %q", device)
	}

	cmd := make(map[string]any)

	if temp, ok := input["temperature"]; ok {
		var tempVal float64
		switch v := temp.(type) {
		case float64:
			tempVal = v
		case json.Number:
			tempVal, _ = v.Float64()
		}
		cmd["temperature"] = map[string]any{"targetTemperature": tempVal}
	}

	if mode, ok := input["mode"].(string); ok {
		modeMap := map[string]string{
			"cool": "COOL",
			"heat": "HEAT",
			"auto": "AUTO",
			"dry":  "AIR_DRY",
			"fan":  "FAN",
		}
		if apiMode, valid := modeMap[mode]; valid {
			cmd["airConJobMode"] = map[string]any{"currentJobMode": apiMode}
		} else {
			return "", fmt.Errorf("invalid mode: %q (valid: cool, heat, auto, dry, fan)", mode)
		}
	}

	if fanSpeed, ok := input["fan_speed"].(string); ok {
		cmd["airFlow"] = map[string]any{"windStrength": strings.ToUpper(fanSpeed)}
	}

	if display, ok := input["display"].(string); ok {
		val := "DISPLAY_ON"
		if display == "off" {
			val = "DISPLAY_OFF"
		}
		cmd["display"] = map[string]any{"displayLight": val}
	}

	if len(cmd) == 0 {
		return "", fmt.Errorf("no settings provided. Use --temp, --mode, --fan_speed, or --display")
	}

	result, err := t.client.Control(deviceID, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to control device: %w", err)
	}

	stateJSON, _ := json.Marshal(result)
	t.db.UpsertState(deviceID, string(stateJSON))

	return fmt.Sprintf("Settings applied to %q.\n%s", device, formatState(deviceID, result)), nil
}

func formatDeviceList(devices []Device) string {
	if len(devices) == 0 {
		return "No devices found."
	}

	var sb strings.Builder
	sb.WriteString("Devices:\n")
	for _, d := range devices {
		name := d.Alias
		if name == "" {
			name = d.DeviceID
		}
		sb.WriteString(fmt.Sprintf("- %s (%s) [%s]\n", name, d.DeviceType, d.DeviceID))
	}
	return sb.String()
}

func formatState(deviceID string, state map[string]any) string {
	var sb strings.Builder

	if op, ok := state["operation"].(map[string]any); ok {
		if mode, ok := op["airConOperationMode"].(string); ok {
			sb.WriteString(fmt.Sprintf("Power: %s\n", mode))
		}
	}
	if jm, ok := state["airConJobMode"].(map[string]any); ok {
		if mode, ok := jm["currentJobMode"].(string); ok {
			sb.WriteString(fmt.Sprintf("Mode: %s\n", mode))
		}
	}
	if temp, ok := state["temperature"].(map[string]any); ok {
		if cur, ok := temp["currentTemperature"]; ok {
			sb.WriteString(fmt.Sprintf("Current temp: %v°C\n", cur))
		}
		if tgt, ok := temp["targetTemperature"]; ok {
			sb.WriteString(fmt.Sprintf("Target temp: %v°C\n", tgt))
		}
	}
	if af, ok := state["airFlow"].(map[string]any); ok {
		if ws, ok := af["windStrength"].(string); ok {
			sb.WriteString(fmt.Sprintf("Fan speed: %s\n", ws))
		}
	}
	if disp, ok := state["display"].(map[string]any); ok {
		if dl, ok := disp["displayLight"].(string); ok {
			sb.WriteString(fmt.Sprintf("Display: %s\n", dl))
		}
	}

	if sb.Len() == 0 {
		// Fallback: dump raw state as JSON
		raw, _ := json.MarshalIndent(state, "", "  ")
		return string(raw)
	}

	return strings.TrimRight(sb.String(), "\n")
}
```

**Step 3: Run tests to verify they pass**

Run: `go test ./tools/thinq/ -v`
Expected: All tests PASS

**Step 4: Commit**

```bash
git add tools/thinq/thinq.go tools/thinq/thinq_test.go
git commit -m "feat(thinq): add ThinqTool with devices, status, power, set commands"
```

---

### Task 4: Wire into main.go

**Files:**
- Modify: `main.go`

**Step 1: Add the thinq tool registration**

In `main.go`, add the import and conditional registration after the existing tool registrations (line 63).

Add import:
```go
"github.com/esnunes/bobot/tools/thinq"
```

Add after line 63 (`registry.Register(topic.NewTopicTool(coreDB))`):

```go
	// Initialize ThinQ tool (optional, only if configured)
	if thinqToken := os.Getenv("THINQ_TOKEN"); thinqToken != "" {
		thinqClient := thinq.NewClient(thinqToken, os.Getenv("THINQ_COUNTRY"), os.Getenv("THINQ_CLIENT_ID"))
		thinqDB, err := thinq.NewThinqDB(filepath.Join(cfg.DataDir, "tool_thinq.db"))
		if err != nil {
			log.Fatalf("Failed to initialize thinq database: %v", err)
		}
		defer thinqDB.Close()
		registry.Register(thinq.NewThinqTool(thinqClient, thinqDB))
	}
```

**Step 2: Run full test suite**

Run: `go test ./...`
Expected: All existing tests still PASS, plus new thinq tests PASS

**Step 3: Verify build compiles**

Run: `go build ./...`
Expected: Clean build

**Step 4: Commit**

```bash
git add main.go
git commit -m "feat(thinq): wire ThinqTool into main.go"
```

---

### Task 5: Final verification

**Step 1: Run full test suite one more time**

Run: `go test ./... -v`
Expected: All tests PASS

**Step 2: Run go vet**

Run: `go vet ./...`
Expected: No issues

**Step 3: Verify tidy**

Run: `go mod tidy && git diff go.mod go.sum`
Expected: No unexpected changes (or commit if there are changes from `go mod tidy`)
