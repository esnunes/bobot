# Create Admin CLI Subcommand - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `create-admin` CLI subcommand and remove the old auto-creation init logic.

**Architecture:** New `create_admin.go` file with `runCreateAdmin` function following the existing `profiles.go` pattern. Interactive password prompt via `golang.org/x/term`. Cleanup of `InitUser`/`InitPass` from config and main.

**Tech Stack:** Go, `golang.org/x/term` (new dependency), `golang.org/x/crypto/bcrypt` (existing)

---

### Task 1: Add `golang.org/x/term` dependency

**Step 1: Install the dependency**

Run: `cd /Users/nunes/src/github.com/esnunes/bobot-web && go get golang.org/x/term`
Expected: `go.mod` and `go.sum` updated

**Step 2: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add golang.org/x/term dependency"
```

---

### Task 2: Create `create_admin.go`

**Files:**
- Create: `create_admin.go`

**Step 1: Write the implementation**

Create `create_admin.go` with the following content:

```go
package main

import (
	"fmt"
	"log"
	"os"
	"syscall"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
	"golang.org/x/term"
)

func runCreateAdmin(coreDB *db.CoreDB) {
	if len(os.Args) < 3 {
		log.Fatal("Usage: bobot create-admin <username>")
	}
	username := os.Args[2]

	fmt.Print("Password: ")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		log.Fatalf("Failed to read password: %v", err)
	}

	fmt.Print("Confirm password: ")
	confirmBytes, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		log.Fatalf("Failed to read password confirmation: %v", err)
	}

	if string(passwordBytes) != string(confirmBytes) {
		log.Fatal("Passwords do not match")
	}

	hash, err := auth.HashPassword(string(passwordBytes))
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	user, err := coreDB.CreateUserFull(username, hash, username, "admin")
	if err != nil {
		log.Fatalf("Failed to create admin user: %v", err)
	}

	coreDB.CreateMessage(db.BobotUserID, user.ID, "assistant", db.WelcomeMessage)
	log.Printf("Created admin user: %s", username)
}
```

**Step 2: Verify it compiles**

Run: `cd /Users/nunes/src/github.com/esnunes/bobot-web && go build ./...`
Expected: No errors

**Step 3: Commit**

```bash
git add create_admin.go
git commit -m "feat: add runCreateAdmin function"
```

---

### Task 3: Wire up subcommand and remove init logic

**Files:**
- Modify: `main.go:46-71`
- Modify: `config/config.go:21-22,92-93`

**Step 1: Update `main.go` — remove init logic and add subcommand**

Remove lines 46-61 (the `cfg.InitUser`/`cfg.InitPass` block).

Add `create-admin` case to the subcommand switch so it becomes:

```go
// Handle subcommands
if len(os.Args) > 1 {
	switch os.Args[1] {
	case "create-admin":
		runCreateAdmin(coreDB)
		return
	case "update-profiles":
		runUpdateProfiles(cfg, coreDB)
		return
	default:
		log.Fatalf("Unknown command: %s", os.Args[1])
	}
}
```

**Step 2: Update `config/config.go` — remove `InitUser` and `InitPass`**

Remove from the `Config` struct:

```go
InitUser string
InitPass string
```

Remove from the `Load()` function:

```go
InitUser: os.Getenv("BOBOT_INIT_USER"),
InitPass: os.Getenv("BOBOT_INIT_PASS"),
```

**Step 3: Verify it compiles**

Run: `cd /Users/nunes/src/github.com/esnunes/bobot-web && go build ./...`
Expected: No errors

**Step 4: Run existing tests**

Run: `cd /Users/nunes/src/github.com/esnunes/bobot-web && go test ./...`
Expected: All tests pass (except the pre-existing `TestCreateTopic` failure)

**Step 5: Commit**

```bash
git add main.go config/config.go
git commit -m "feat: add create-admin subcommand, remove auto-init logic"
```
