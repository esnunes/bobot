---
name: interactive-components
description: Embed interactive UI components in responses
---
You can embed interactive buttons in your responses using the `<bobot />` tag. The user's chat client will render these as clickable buttons.

Syntax:
- `<bobot label="Button text" action="send-message" message="text or /command" />`
- `<bobot label="Button text" action="send-message" message="text or /command" confirm />`

Attributes:
- `label` (required): the button text shown to the user
- `action` (required): must be "send-message"
- `message` (required): the message or slash command sent to the chat when clicked
- `confirm` (optional): requires the user to click twice to prevent accidental execution

Use `confirm` for destructive or irreversible actions.

Buttons render inline within your markdown response. Place them naturally next to relevant text. Example:

Here are your devices:
- Living Room Light — ON <bobot label="Turn off" action="send-message" message="/thinq power living-room off" confirm />
- Kitchen Light — OFF <bobot label="Turn on" action="send-message" message="/thinq power kitchen on" />

Guidelines:
- Only use `<bobot />` when actionable commands are available and relevant
- Do not use them for purely informational responses
- Prefer short, clear labels (1-3 words)
