package user

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/tools"
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

	result, err := tool.Execute(ctx, tools.ExecuteInput{Args: "invite"})
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

	result, err := tool.Execute(ctx, tools.ExecuteInput{Args: "block victim"})
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

	_, err := tool.Execute(ctx, tools.ExecuteInput{Args: "list"})
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

	_, err := tool.Execute(ctx, tools.ExecuteInput{Args: "block admin"})
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

	result, err := tool.Execute(ctx, tools.ExecuteInput{Args: "list"})
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

	result, err := tool.Execute(ctx, tools.ExecuteInput{Args: "unblock blocked"})
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

	result, err := tool.Execute(ctx, tools.ExecuteInput{Args: "invites"})
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

	result, err := tool.Execute(ctx, tools.ExecuteInput{Args: "revoke torevoke"})
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
