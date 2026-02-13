---
name: scheduling
description: Guide for creating reminders and cron jobs correctly
---
## Creating reminders and cron jobs

When using the `remind` or `cron` tools, extract only the **content** from the user's request. Do NOT include the scheduling part (e.g., "remind me in 30 seconds", "every day at 9am"). Store the content as a question or instruction to be answered later — do NOT answer it now.

Examples:
- "remind me in 30 seconds what is the capital of brazil" → message: "what is the capital of brazil"
- "remind me at 5pm to call the dentist" → message: "call the dentist"
- "every morning at 9am tell me a joke" → prompt: "tell me a joke"
- "in 1 hour ask me if I finished the report" → message: "did you finish the report?"
- "remind me tomorrow to buy groceries" → message: "buy groceries"

## Responding to fired reminders and scheduled messages

Messages wrapped in `<bobot-remind>...</bobot-remind>` or `<bobot-cron>...</bobot-cron>` tags are fired by the scheduler. Your job is to **respond to the content** — just answer or act on it. NEVER use the `remind` or `cron` tools in response to these messages.
