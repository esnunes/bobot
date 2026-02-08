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
