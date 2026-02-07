# Create Admin CLI Subcommand

## Summary

Add a `create-admin` CLI subcommand to create admin users from the command line. Remove the existing first-admin auto-creation logic that runs at startup.

## Usage

```
./bobot create-admin <username>
```

The command prompts interactively for a password (masked input) with confirmation.

## Changes

### 1. New file: `create_admin.go`

A `runCreateAdmin` function following the `runUpdateProfiles` pattern in `profiles.go`:

- Validate that a username positional argument is provided
- Read password from terminal using `golang.org/x/term` (masked input)
- Prompt for password confirmation; abort on mismatch
- Hash password with `auth.HashPassword`
- Create user via `coreDB.CreateUserFull(username, hash, username, "admin")`
- Send welcome message via `coreDB.CreateMessage(db.BobotUserID, user.ID, "assistant", db.WelcomeMessage)`
- Print success, exit

### 2. Update `main.go`

- Add `create-admin` case to the `os.Args[1]` switch
- Remove the `cfg.InitUser`/`cfg.InitPass` first-admin auto-creation block

### 3. Update `config/config.go`

- Remove `InitUser` and `InitPass` fields from `Config`

## Dependencies

- `golang.org/x/term` for masked password input

## Design decisions

- **Interactive password prompt** over flags/env vars to avoid leaking passwords in shell history or process lists
- **Positional username argument** for simplicity since it's the only required input
- **Display name defaults to username** — kept minimal, can be changed later
- **Welcome message sent** to match existing behavior for all new users
