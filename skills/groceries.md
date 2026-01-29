---
name: groceries
description: Manage grocery shopping lists
---
When the user wants to manage their grocery list, use the `task` tool with project name "groceries".

Examples:
- "Add milk" → task(command="create", project="groceries", title="milk")
- "What do I need to buy?" → task(command="list", project="groceries", status="pending")
- "Got the eggs" → task(command="update", project="groceries", title="eggs", status="done")
- "Remove bread from the list" → task(command="delete", project="groceries", title="bread")
- "Show me everything on my list" → task(command="list", project="groceries")

Keep responses brief:
- For adding: "Added milk." or "Added milk, eggs, and bread."
- For marking done: "Got it!" or "Crossed off milk."
- For listing: Show the items naturally, e.g., "You need: milk, eggs, bread"
- For removing: "Removed bread."
