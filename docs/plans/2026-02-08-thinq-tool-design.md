# ThinQ Tool Design

## Overview

A new `thinq` tool for bobot-web that integrates with the LG ThinQ Connect API. Enables users and the LLM to list smart home devices, check device status, and control air conditioners (power, temperature, mode, fan speed, display).

## Configuration

Environment variables (tool only registered when `THINQ_TOKEN` is set):

| Variable | Required | Description |
|---|---|---|
| `THINQ_TOKEN` | Yes | Personal Access Token from LG ThinQ |
| `THINQ_COUNTRY` | Yes | Country code (e.g., `US`, `GB`, `BR`) |
| `THINQ_CLIENT_ID` | Yes | Client identifier for API requests |

## Commands

| Command | Description | Parameters |
|---|---|---|
| `devices` | List all registered devices | none |
| `status` | Get current state of a device | `device` (ID or alias) |
| `power` | Turn AC on/off | `device`, `power` (on/off) |
| `set` | Change AC settings | `device`, plus optional: `temperature`, `mode`, `fan_speed`, `display` |

Device identification: accepts either `device_id` or device alias (case-insensitive match). Alias resolution uses the cached device list from the database.

### Slash Command Examples

```
/thinq devices
/thinq status bedroom
/thinq power bedroom on
/thinq set bedroom --temp=22 --mode=cool --fan_speed=medium
```

### LLM Schema

```json
{
  "type": "object",
  "properties": {
    "command": {
      "type": "string",
      "enum": ["devices", "status", "power", "set"],
      "description": "The operation to perform"
    },
    "device": {
      "type": "string",
      "description": "Device ID or alias name"
    },
    "power": {
      "type": "string",
      "enum": ["on", "off"],
      "description": "Power state (for power command)"
    },
    "temperature": {
      "type": "number",
      "description": "Target temperature in Celsius (for set command)"
    },
    "mode": {
      "type": "string",
      "enum": ["cool", "heat", "auto", "dry", "fan"],
      "description": "Operation mode (for set command)"
    },
    "fan_speed": {
      "type": "string",
      "description": "Fan/wind strength (for set command)"
    },
    "display": {
      "type": "string",
      "enum": ["on", "off"],
      "description": "Display light state (for set command)"
    }
  },
  "required": ["command"]
}
```

## File Layout

```
tools/thinq/
  thinq.go      — Tool implementation (ThinqTool struct, commands, schema, ParseArgs, Execute)
  client.go     — ThinQ API HTTP client
  db.go         — SQLite database for device/state caching
```

## API Client

**Base URL derivation** from country code:
- US, CA, BR, MX, ... (Americas) → `https://api-aic.lgthinq.com`
- GB, DE, FR, ... (Europe/ME/Africa) → `https://api-eic.lgthinq.com`
- KR, JP, AU, ... (Asia/Pacific) → `https://api-kic.lgthinq.com`

**Required headers** for all requests:
- `Authorization: Bearer {token}`
- `x-api-key: v6GFvkweNo7DK7yD3ylIZ9w52aKBU0eJ7wLXkSR3`
- `x-country: {country}`
- `x-message-id: {random-uuid-base64}`
- `x-client-id: {client_id}`
- `x-service-phase: OP`
- `Content-Type: application/json`

**Methods:**
- `ListDevices() ([]Device, error)` — `GET /devices`
- `GetState(deviceID string) (map[string]any, error)` — `GET /devices/{id}/state`
- `Control(deviceID string, command map[string]any) (map[string]any, error)` — `POST /devices/{id}/control` with `x-conditional-control: true`

## Database & Caching

Separate SQLite database (`tool_thinq.db`), following the `tools/task/db.go` pattern.

### Tables

```sql
CREATE TABLE IF NOT EXISTS devices (
    device_id TEXT PRIMARY KEY,
    alias TEXT,
    device_type TEXT,
    model TEXT,
    raw_json TEXT,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS device_states (
    device_id TEXT PRIMARY KEY,
    state_json TEXT,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Caching Behavior

- **`devices` command**: Calls API, updates `devices` table, returns fresh list. On API failure, falls back to cached list and indicates it's stale.
- **`status` command**: Calls API, updates `device_states`, returns fresh state. On API failure, returns cached state with "(outdated — last updated: {timestamp})" warning.
- **Alias resolution**: Reads from `devices` table — works even when the API is down.
- **After control**: Updated state from API response is cached in `device_states`.

## Control Command Mapping

| User parameter | API payload |
|---|---|
| `power: on` | `{"operation": {"airConOperationMode": "POWER_ON"}}` |
| `power: off` | `{"operation": {"airConOperationMode": "POWER_OFF"}}` |
| `temperature: 22` | `{"temperature": {"targetTemperature": 22}}` |
| `mode: cool` | `{"airConJobMode": {"currentJobMode": "COOL"}}` |
| `mode: heat` | `{"airConJobMode": {"currentJobMode": "HEAT"}}` |
| `mode: auto` | `{"airConJobMode": {"currentJobMode": "AUTO"}}` |
| `mode: dry` | `{"airConJobMode": {"currentJobMode": "AIR_DRY"}}` |
| `mode: fan` | `{"airConJobMode": {"currentJobMode": "FAN"}}` |
| `fan_speed: <value>` | `{"airFlow": {"windStrength": "<VALUE>"}}` |
| `display: on` | `{"display": {"light": "ON"}}` |
| `display: off` | `{"display": {"light": "OFF"}}` |

Multiple settings can be combined in a single `set` command. Parameters are merged into one control payload.

## Wiring (main.go)

```go
if thinqToken := os.Getenv("THINQ_TOKEN"); thinqToken != "" {
    thinqClient := thinq.NewClient(thinqToken, os.Getenv("THINQ_COUNTRY"), os.Getenv("THINQ_CLIENT_ID"))
    thinqDB, err := thinq.NewThinqDB(cfg.DataDir)
    // handle err
    registry.Register(thinq.NewThinqTool(thinqClient, thinqDB))
}
```

Tool only registered when `THINQ_TOKEN` is set.

## Access Control

Not admin-only — all authenticated users can use this tool.

## Response Formatting

- **`devices`**: Table with device name, type, and ID
- **`status`**: Human-readable state (e.g., "Bedroom AC: ON, 22°C, Cool mode, Fan: Medium")
- **`power`/`set`**: Confirmation message with the new state after control
- **Errors**: User-friendly messages (device offline, invalid temperature range, etc.)
