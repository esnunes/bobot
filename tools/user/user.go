package user

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

type UserTool struct {
	db      *db.CoreDB
	baseURL string
}

func NewUserTool(db *db.CoreDB, baseURL string) *UserTool {
	return &UserTool{db: db, baseURL: baseURL}
}

func (t *UserTool) Name() string {
	return "user"
}

func (t *UserTool) Description() string {
	return "Manage users: invite new users, block/unblock users, list users and invites"
}

func (t *UserTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"enum":        []string{"invite", "block", "unblock", "list", "invites", "revoke"},
				"description": "The operation to perform",
			},
			"username": map[string]any{
				"type":        "string",
				"description": "Username for block/unblock commands",
			},
			"code": map[string]any{
				"type":        "string",
				"description": "Invite code for revoke command",
			},
		},
		"required": []string{"command"},
	}
}

func (t *UserTool) AdminOnly() bool {
	return true
}

func (t *UserTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	userData := auth.UserDataFromContext(ctx)
	// Check admin role
	if userData.Role != "admin" {
		return "", fmt.Errorf("this command requires admin privileges")
	}

	command, _ := input["command"].(string)
	username, _ := input["username"].(string)
	code, _ := input["code"].(string)

	switch command {
	case "invite":
		return t.invite(userData.UserID)
	case "block":
		return t.block(userData.UserID, username)
	case "unblock":
		return t.unblock(username)
	case "list":
		return t.list()
	case "invites":
		return t.listInvites()
	case "revoke":
		return t.revoke(code)
	default:
		return "", fmt.Errorf("unknown command: %s", command)
	}
}

func (t *UserTool) invite(createdBy int64) (string, error) {
	code := generateInviteCode()
	_, err := t.db.CreateInvite(createdBy, code)
	if err != nil {
		return "", fmt.Errorf("failed to create invite: %w", err)
	}

	url := fmt.Sprintf("%s/signup?code=%s", t.baseURL, code)
	return fmt.Sprintf("Invite created! Share this signup URL:\n%s", url), nil
}

func (t *UserTool) block(adminID int64, username string) (string, error) {
	if username == "" {
		return "", fmt.Errorf("username is required")
	}

	user, err := t.db.GetUserByUsername(username)
	if err == db.ErrNotFound {
		return "", fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return "", err
	}

	if user.ID == adminID {
		return "", fmt.Errorf("cannot block yourself")
	}

	if err := t.db.BlockUser(user.ID); err != nil {
		return "", err
	}

	// Revoke all sessions for blocked user
	if err := t.db.CreateSessionRevocation(user.ID, "user_blocked"); err != nil {
		return "", err
	}

	return fmt.Sprintf("User '%s' has been blocked.", username), nil
}

func (t *UserTool) unblock(username string) (string, error) {
	if username == "" {
		return "", fmt.Errorf("username is required")
	}

	user, err := t.db.GetUserByUsername(username)
	if err == db.ErrNotFound {
		return "", fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return "", err
	}

	if err := t.db.UnblockUser(user.ID); err != nil {
		return "", err
	}

	return fmt.Sprintf("User '%s' has been unblocked.", username), nil
}

func (t *UserTool) list() (string, error) {
	users, err := t.db.ListUsers()
	if err != nil {
		return "", err
	}

	if len(users) == 0 {
		return "No users found.", nil
	}

	var sb strings.Builder
	sb.WriteString("Users:\n")
	sb.WriteString("| Username | Display Name | Role | Status | Created |\n")
	sb.WriteString("|----------|--------------|------|--------|--------|\n")
	for _, u := range users {
		status := "active"
		if u.Blocked {
			status = "blocked"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			u.Username, u.DisplayName, u.Role, status, u.CreatedAt.Format("2006-01-02")))
	}
	return sb.String(), nil
}

func (t *UserTool) listInvites() (string, error) {
	invites, err := t.db.GetPendingInvites()
	if err != nil {
		return "", err
	}

	if len(invites) == 0 {
		return "No pending invites.", nil
	}

	var sb strings.Builder
	sb.WriteString("Pending Invites:\n")
	sb.WriteString("| Code | Created By | Created At |\n")
	sb.WriteString("|------|------------|------------|\n")
	for _, inv := range invites {
		creator, _ := t.db.GetUserByID(inv.CreatedBy)
		creatorName := "unknown"
		if creator != nil {
			creatorName = creator.Username
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n",
			inv.Code, creatorName, inv.CreatedAt.Format("2006-01-02 15:04")))
	}
	return sb.String(), nil
}

func (t *UserTool) revoke(code string) (string, error) {
	if code == "" {
		return "", fmt.Errorf("invite code is required")
	}

	err := t.db.RevokeInvite(code)
	if err == db.ErrNotFound {
		return "", fmt.Errorf("invite not found or already used")
	}
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Invite '%s' has been revoked.", code), nil
}

func generateInviteCode() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
