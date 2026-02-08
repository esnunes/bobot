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
				"response": []map[string]any{
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
