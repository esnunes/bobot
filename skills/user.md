---
name: user
description: Admin commands for user management (admin only)
---
When an admin user wants to manage users or invites, use the `user` tool. This tool is only available to admin users.

Slash command syntax: `/user <command> [args]`

Commands:
- `/user invite` → user(command="invite") - Creates an invite link for a new user
- `/user list` → user(command="list") - Shows all users with their status
- `/user block <username>` → user(command="block", username="<username>") - Blocks a user
- `/user unblock <username>` → user(command="unblock", username="<username>") - Unblocks a user
- `/user invites` → user(command="invites") - Shows pending invites
- `/user revoke <code>` → user(command="revoke", code="<code>") - Revokes an invite

Natural language examples:
- "Create an invite" → user(command="invite")
- "Show me all users" → user(command="list")
- "Block john" → user(command="block", username="john")
- "Unblock jane" → user(command="unblock", username="jane")
- "What invites are pending?" → user(command="invites")
- "Cancel invite abc123" → user(command="revoke", code="abc123")

Keep responses brief:
- For invite: Show the signup URL
- For list: Show the user table
- For block/unblock: Confirm the action
- For invites: Show pending invites or "No pending invites"
- For revoke: Confirm revocation
