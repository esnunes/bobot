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

func (c *Client) do(req *http.Request) (any, error) {
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

	return response, nil
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

	rawDevices, ok := resp.([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected response: 'response' is not an array")
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

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	m, ok := resp.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected response format for device state")
	}
	return m, nil
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

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	m, ok := resp.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected response format for device control")
	}
	return m, nil
}
