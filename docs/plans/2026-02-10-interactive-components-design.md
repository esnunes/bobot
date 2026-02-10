# Interactive Components Design

## Overview

Allow LLMs to embed interactive UI components (buttons) in their markdown responses using a custom `<bobot />` HTML tag. When clicked, buttons send messages to the chat on behalf of the user, enabling one-click actions like toggling smart home devices.

## Use Case

When listing ThinQ devices, the LLM can render action buttons inline:

```
- Living Room Light — ON <bobot label="Turn off" action="send-message" message="/thinq power living-room off" confirm />
- Kitchen Light — OFF <bobot label="Turn on" action="send-message" message="/thinq power kitchen on" />
```

## Tag Syntax

```html
<bobot label="Button text" action="send-message" message="/command args" />
<bobot label="Button text" action="send-message" message="/command args" confirm />
```

### Attributes

| Attribute | Required | Description |
|-----------|----------|-------------|
| `label`   | yes      | Button text shown to the user |
| `action`  | yes      | Must be `send-message` |
| `message` | yes      | The message sent to the chat when clicked |
| `confirm` | no       | Requires two clicks to execute (prevents accidental actions) |

## Rendering Pipeline

1. LLM response arrives as text containing markdown and `<bobot />` tags
2. `marked.parse()` runs — passes through unknown HTML tags untouched
3. `DOMPurify.sanitize()` runs — configured with `ADD_TAGS: ['bobot']` and `ADD_ATTR: ['label', 'action', 'message', 'confirm']`
4. A new **post-processing step** (`processBobotTags`) walks the DOM, finds all `<bobot>` elements, validates them, and replaces each with a styled `<button>` element
5. For **history messages**, buttons are rendered in a **disabled** state

## Button States

| State | Appearance | Behavior |
|-------|-----------|----------|
| **Default** | `--color-surface0` bg, `--color-text` text | Clickable |
| **Hover** | `--color-surface1` bg | Visual feedback |
| **Confirm-pending** | `--color-red` bg, `--color-base` text | Waiting for second click |
| **Disabled** | `--color-surface0` bg, `--color-overlay0` text, reduced opacity | Not clickable (after execution or history) |

## Button Behavior

### `send-message` action
- On click, sends `data-message` value through the existing chat submit path (same as pressing Enter in the input field)
- After sending, the button becomes **disabled**

### `confirm` attribute
- First click: button changes to confirm-pending state (red, text becomes "Confirm?")
- Second click: executes the action, button becomes disabled
- Clicking **anywhere else** on the page: global click listener resets **all** confirm-pending buttons to their original state

## Error Handling

- **Missing `label`, `action`, or `message`**: tag is silently removed
- **Unknown `action` value**: tag is silently removed (forward-compatible for future actions)
- **Multiple buttons in confirm state**: clicking elsewhere resets all of them

## Styling

Uses Catppuccin Latte design tokens from `tokens.css`. Buttons render inline within the message flow, sitting naturally next to text.

## LLM Skill

A new skill file `skills/interactive-components.md` instructs the LLM when and how to use `<bobot />` tags. Follows the same YAML frontmatter pattern as existing skills.

## Files Changed

### Modified
1. **`web/static/message-renderer.js`** — add `processBobotTags(container, isHistory)` function
2. **`web/static/chat.js`** — call `processBobotTags()` after rendering messages; add global click listener for confirm reset; expose submit function for button use
3. **`web/static/topic_chat.js`** — same changes as `chat.js`
4. **`web/static/style.css`** — add `.bobot-action-btn` styles for all states

### New
5. **`skills/interactive-components.md`** — LLM skill teaching `<bobot />` usage

### No Changes
- Server-side Go code
- Database schema
- Message storage format (`<bobot />` tags stored as-is in `Content`)
