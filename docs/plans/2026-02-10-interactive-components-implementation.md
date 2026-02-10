# Interactive Components Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Allow LLMs to embed interactive `<bobot />` buttons in markdown responses that send messages when clicked.

**Architecture:** Client-side only. A post-processing step in `message-renderer.js` transforms `<bobot>` tags (preserved through DOMPurify) into styled `<button>` elements with click handlers. Both `chat.js` and `topic_chat.js` call this after rendering. A new skill file teaches the LLM the syntax.

**Tech Stack:** Vanilla JavaScript, DOMPurify (custom tag config), Catppuccin Latte CSS tokens

**Worktree:** `/Users/nunes/src/github.com/esnunes/bobot-web/.worktrees/feature/interactive-components`

---

### Task 1: Add CSS styles for bobot action buttons

**Files:**
- Modify: `web/static/style.css` (append after line 741, before the closing of the file)

**Step 1: Add button styles**

Append the following CSS section to the end of `web/static/style.css`:

```css
/* ============================================
   BOBOT ACTION BUTTONS
   ============================================ */
.bobot-action-btn {
  display: inline-flex;
  align-items: center;
  padding: var(--space-0) var(--space-1);
  background: var(--colors-border);
  color: var(--colors-text);
  border: var(--borders-thin);
  border-radius: var(--radii-md);
  font-family: var(--fonts-body);
  font-size: var(--font-sizes-1);
  font-weight: var(--font-weights-medium);
  cursor: pointer;
  transition: background-color var(--transitions-fast), color var(--transitions-fast), border-color var(--transitions-fast);
  vertical-align: middle;
  margin: 2px 4px;
  line-height: var(--line-heights-normal);
}

.bobot-action-btn:hover {
  background: var(--colors-border-strong);
}

.bobot-action-btn--confirming {
  background: var(--colors-danger);
  color: var(--colors-background);
  border-color: var(--colors-danger);
}

.bobot-action-btn--confirming:hover {
  background: var(--colors-danger-hover);
  border-color: var(--colors-danger-hover);
}

.bobot-action-btn:disabled {
  background: var(--colors-border);
  color: var(--colors-text-secondary);
  border-color: var(--colors-border);
  opacity: var(--opacities-disabled);
  cursor: default;
}
```

**Step 2: Verify visually (manual)**

No automated test — this is CSS. Visual verification during integration testing.

**Step 3: Commit**

```bash
git add web/static/style.css
git commit -m "feat(ui): add bobot action button styles"
```

---

### Task 2: Add `processBobotTags` to message-renderer.js and configure DOMPurify

**Files:**
- Modify: `web/static/message-renderer.js`

**Step 1: Update `renderMessageContent` to allow `<bobot>` through DOMPurify**

In `message-renderer.js`, change the `DOMPurify.sanitize(html)` call to allow the `<bobot>` tag and its attributes:

Replace line 12:
```javascript
        return DOMPurify.sanitize(html);
```
With:
```javascript
        return DOMPurify.sanitize(html, {
            ADD_TAGS: ['bobot'],
            ADD_ATTR: ['label', 'action', 'message', 'confirm']
        });
```

**Step 2: Add `processBobotTags` function**

Add the following function to the `MessageRenderer` object (after `highlightCodeBlocks`):

```javascript
    processBobotTags(parentEl, sendFn, isHistory) {
        var bobotEls = parentEl.querySelectorAll('bobot');
        bobotEls.forEach(function(el) {
            var label = el.getAttribute('label');
            var action = el.getAttribute('action');
            var message = el.getAttribute('message');
            var needsConfirm = el.hasAttribute('confirm');

            // Validate: remove invalid tags
            if (!label || action !== 'send-message' || !message) {
                el.remove();
                return;
            }

            var btn = document.createElement('button');
            btn.className = 'bobot-action-btn';
            btn.textContent = label;
            btn.setAttribute('data-action', action);
            btn.setAttribute('data-message', message);
            if (needsConfirm) {
                btn.setAttribute('data-confirm', '');
            }
            btn.setAttribute('data-original-label', label);

            if (isHistory) {
                btn.disabled = true;
            } else {
                btn.addEventListener('click', function(e) {
                    e.stopPropagation();

                    if (needsConfirm && !btn.classList.contains('bobot-action-btn--confirming')) {
                        // First click: enter confirm state
                        MessageRenderer.resetConfirmingButtons();
                        btn.classList.add('bobot-action-btn--confirming');
                        btn.textContent = 'Confirm?';
                        return;
                    }

                    // Execute action
                    btn.disabled = true;
                    btn.classList.remove('bobot-action-btn--confirming');
                    btn.textContent = btn.getAttribute('data-original-label');
                    sendFn(message);
                });
            }

            el.replaceWith(btn);
        });
    },

    resetConfirmingButtons() {
        document.querySelectorAll('.bobot-action-btn--confirming').forEach(function(btn) {
            btn.classList.remove('bobot-action-btn--confirming');
            btn.textContent = btn.getAttribute('data-original-label');
        });
    }
```

**Step 3: Commit**

```bash
git add web/static/message-renderer.js
git commit -m "feat(ui): add bobot tag processing to message renderer"
```

---

### Task 3: Integrate `processBobotTags` into chat.js

**Files:**
- Modify: `web/static/chat.js`

**Step 1: Add global click listener for resetting confirm buttons**

In the `setupEventListeners()` method, add a document-level click listener. Add after the logout event listener (after line 156):

```javascript
        // Reset bobot confirm buttons when clicking elsewhere
        document.addEventListener('click', () => {
            MessageRenderer.resetConfirmingButtons();
        });
```

**Step 2: Create a `sendFn` helper and call `processBobotTags` in `addMessage`**

In the `addMessage` method, after `MessageRenderer.highlightCodeBlocks(msgEl)` (line 184), add:

```javascript
            MessageRenderer.processBobotTags(msgEl, (msg) => {
                if (this.wsContainer.send({ content: msg })) {
                    this.showTypingIndicator();
                }
            }, !!id);
```

The `!!id` check: messages loaded from initial data or sync have an `id`, meaning they're history. Live messages from WebSocket events are passed without an `id`.

**Step 3: Call `processBobotTags` in `prependMessage`**

In the `prependMessage` method, after `MessageRenderer.highlightCodeBlocks(msgEl)` (line 206), add:

```javascript
            MessageRenderer.processBobotTags(msgEl, (msg) => {
                if (this.wsContainer.send({ content: msg })) {
                    this.showTypingIndicator();
                }
            }, true);
```

`prependMessage` is only used for history loading, so `isHistory` is always `true`.

**Step 4: Commit**

```bash
git add web/static/chat.js
git commit -m "feat(ui): integrate bobot action buttons into private chat"
```

---

### Task 4: Integrate `processBobotTags` into topic_chat.js

**Files:**
- Modify: `web/static/topic_chat.js`

**Step 1: Add global click listener for resetting confirm buttons**

In the `setupEventListeners()` method, add after the scroll event listener (after line 100):

```javascript
        // Reset bobot confirm buttons when clicking elsewhere
        document.addEventListener('click', () => {
            MessageRenderer.resetConfirmingButtons();
        });
```

**Step 2: Call `processBobotTags` in `addMessage`**

In the `addMessage` method, after `MessageRenderer.highlightCodeBlocks(contentEl)` (line 162), add:

```javascript
            MessageRenderer.processBobotTags(contentEl, (msg) => {
                if (this.wsContainer.send({ content: msg, topic_id: this.topicId })) {
                    // Show typing indicator if message mentions @bobot
                    if (msg.toLowerCase().includes('@bobot')) {
                        this.showTypingIndicator();
                    }
                }
            }, !!id);
```

Note: In `topic_chat.js`, `addMessage` takes a `msg` object. The `id` is accessed as `msg.id || msg.ID`. We need to determine `isHistory` from the `scroll` parameter — if `scroll` is `false`, it's initial load (history). But more reliably: live WebSocket messages don't have an `id` field initially. Use `!!id` (the local variable extracted on line 141).

**Step 3: Call `processBobotTags` in `prependMessage`**

In the `prependMessage` method, after `MessageRenderer.highlightCodeBlocks(contentEl)` (line 252), add:

```javascript
            MessageRenderer.processBobotTags(contentEl, (msg) => {
                if (this.wsContainer.send({ content: msg, topic_id: this.topicId })) {
                    if (msg.toLowerCase().includes('@bobot')) {
                        this.showTypingIndicator();
                    }
                }
            }, true);
```

**Step 4: Commit**

```bash
git add web/static/topic_chat.js
git commit -m "feat(ui): integrate bobot action buttons into topic chat"
```

---

### Task 5: Create the LLM skill file

**Files:**
- Create: `skills/interactive-components.md`

**Step 1: Create the skill file**

Create `skills/interactive-components.md`:

```markdown
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
```

**Step 2: Verify skill loads correctly**

Run existing tests to ensure the skill is parsed:

```bash
go test ./assistant/ -v -run TestLoadSkills
```

If there's no specific test for this, verify manually that the skills directory glob picks it up — `LoadSkills` reads all `.md` files from the embedded FS.

**Step 3: Commit**

```bash
git add skills/interactive-components.md
git commit -m "feat(skills): add interactive-components skill for bobot tags"
```

---

### Task 6: Manual integration test

**No files changed — verification only.**

**Step 1: Run all Go tests**

```bash
go test ./...
```

Expected: All tests pass (same as baseline).

**Step 2: Manual verification checklist**

Start the app and verify:

1. Send a message that triggers the LLM to use `<bobot />` tags (e.g., ask to list ThinQ devices if the tool is configured)
2. Verify buttons render inline with correct styling
3. Click a button without `confirm` — verify message is sent and button disables
4. Click a button with `confirm` — verify it shows "Confirm?" state
5. Click elsewhere — verify confirm state resets
6. Click confirm button twice — verify message is sent and button disables
7. Reload the page — verify old messages show buttons in disabled state
8. Check topic chat — verify same behavior works there
