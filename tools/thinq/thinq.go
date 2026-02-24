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

func (t *ThinqTool) Name() string    { return "thinq" }
func (t *ThinqTool) AdminOnly() bool { return false }

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
				"description": "Device ID or alias name, required for the commands status, power, and set",
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
		val := "ON"
		if display == "off" {
			val = "OFF"
		}
		cmd["display"] = map[string]any{"light": val}
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
		if dl, ok := disp["light"].(string); ok {
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
