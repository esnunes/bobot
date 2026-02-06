package user

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

func setupTestDB(t *testing.T) *db.CoreDB {
	tmpDir := t.TempDir()
	coreDB, err := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	return coreDB
}

func TestUserTool_InviteCommand(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: admin.ID,
		Role:   "admin",
	})

	result, err := tool.Execute(ctx, map[string]any{"command": "invite"})
	if err != nil {
		t.Fatalf("failed to execute invite: %v", err)
	}

	if !strings.Contains(result, "http://localhost:8080/signup?code=") {
		t.Errorf("expected signup URL, got: %s", result)
	}
}

func TestUserTool_BlockCommand(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	user, _ := coreDB.CreateUserFull("victim", "hash", "Victim", "user")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: admin.ID,
		Role:   "admin",
	})

	result, err := tool.Execute(ctx, map[string]any{"command": "block", "username": "victim"})
	if err != nil {
		t.Fatalf("failed to execute block: %v", err)
	}

	if !strings.Contains(result, "blocked") {
		t.Errorf("expected confirmation, got: %s", result)
	}

	// Verify user is blocked
	updated, _ := coreDB.GetUserByID(user.ID)
	if !updated.Blocked {
		t.Error("expected user to be blocked")
	}
}

func TestUserTool_NonAdminDenied(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("user", "hash", "User", "user")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: user.ID,
		Role:   "user",
	})

	_, err := tool.Execute(ctx, map[string]any{"command": "list"})
	if err == nil {
		t.Error("expected error for non-admin")
	}
	if !strings.Contains(err.Error(), "admin") {
		t.Errorf("expected admin error, got: %v", err)
	}
}

func TestUserTool_CannotBlockSelf(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: admin.ID,
		Role:   "admin",
	})

	_, err := tool.Execute(ctx, map[string]any{"command": "block", "username": "admin"})
	if err == nil {
		t.Error("expected error when blocking self")
	}
}

func TestUserTool_ListCommand(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	coreDB.CreateUserFull("user1", "hash", "User One", "user")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: admin.ID,
		Role:   "admin",
	})

	result, err := tool.Execute(ctx, map[string]any{"command": "list"})
	if err != nil {
		t.Fatalf("failed to execute list: %v", err)
	}

	if !strings.Contains(result, "admin") || !strings.Contains(result, "user1") {
		t.Errorf("expected user list, got: %s", result)
	}
}

func TestUserTool_UnblockCommand(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	user, _ := coreDB.CreateUserFull("blocked", "hash", "Blocked", "user")
	coreDB.BlockUser(user.ID)
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: admin.ID,
		Role:   "admin",
	})

	result, err := tool.Execute(ctx, map[string]any{"command": "unblock", "username": "blocked"})
	if err != nil {
		t.Fatalf("failed to execute unblock: %v", err)
	}

	if !strings.Contains(result, "unblocked") {
		t.Errorf("expected confirmation, got: %s", result)
	}

	// Verify user is unblocked
	updated, _ := coreDB.GetUserByID(user.ID)
	if updated.Blocked {
		t.Error("expected user to be unblocked")
	}
}

func TestUserTool_InvitesCommand(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	coreDB.CreateInvite(admin.ID, "invite1")
	coreDB.CreateInvite(admin.ID, "invite2")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: admin.ID,
		Role:   "admin",
	})

	result, err := tool.Execute(ctx, map[string]any{"command": "invites"})
	if err != nil {
		t.Fatalf("failed to execute invites: %v", err)
	}

	if !strings.Contains(result, "invite1") || !strings.Contains(result, "invite2") {
		t.Errorf("expected invite list, got: %s", result)
	}
}

func TestUserTool_RevokeCommand(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	coreDB.CreateInvite(admin.ID, "torevoke")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: admin.ID,
		Role:   "admin",
	})

	result, err := tool.Execute(ctx, map[string]any{"command": "revoke", "code": "torevoke"})
	if err != nil {
		t.Fatalf("failed to execute revoke: %v", err)
	}

	if !strings.Contains(result, "revoked") {
		t.Errorf("expected confirmation, got: %s", result)
	}

	// Verify invite is revoked
	invite, _ := coreDB.GetInviteByCode("torevoke")
	if !invite.Revoked {
		t.Error("expected invite to be revoked")
	}
}

func TestUserTool_ParseArgs(t *testing.T) {
	tool := &UserTool{}

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
			name: "invite command",
			raw:  "invite",
			want: map[string]any{"command": "invite"},
		},
		{
			name: "list command",
			raw:  "list",
			want: map[string]any{"command": "list"},
		},
		{
			name: "invites command",
			raw:  "invites",
			want: map[string]any{"command": "invites"},
		},
		{
			name: "block with username",
			raw:  "block alice",
			want: map[string]any{"command": "block", "username": "alice"},
		},
		{
			name: "unblock with username",
			raw:  "unblock bob",
			want: map[string]any{"command": "unblock", "username": "bob"},
		},
		{
			name: "revoke with code",
			raw:  "revoke abc123",
			want: map[string]any{"command": "revoke", "code": "abc123"},
		},
		{
			name: "block without username",
			raw:  "block",
			want: map[string]any{"command": "block"},
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
					t.Errorf("key %q: got %v, want %v", k, got[k], v)
				}
			}
			if len(got) != len(tt.want) {
				t.Errorf("got %d keys, want %d keys", len(got), len(tt.want))
			}
		})
	}
}
