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
			"response": []map[string]any{
				{
					"deviceId": "dev-1",
					"deviceInfo": map[string]any{
						"alias":      "Bedroom AC",
						"deviceType": "DEVICE_AIR_CONDITIONER",
						"modelName":  "LG-AC-01",
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
