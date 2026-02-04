# Design System Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement a Theme UI-inspired design system with "Soft Stone" palette, replacing current iOS-blue styling with warm neutrals and sage green accent.

**Architecture:** Create `tokens.css` with all design tokens, then refactor `style.css` to use tokens. Update HTML templates to use new class structure. Work incrementally, keeping the app functional at each step.

**Tech Stack:** CSS custom properties, Inter font (Google Fonts), vanilla HTML/CSS

---

## Task 1: Create Design Tokens File

**Files:**
- Create: `web/static/tokens.css`

**Step 1: Create tokens.css with all design tokens**

Create the file `web/static/tokens.css`:

```css
:root {
  /* ============================================
     COLORS - Palette "Soft Stone"
     ============================================ */
  --colors-background: #F7F6F3;
  --colors-surface: #FFFFFF;
  --colors-text: #1A1A1A;
  --colors-text-secondary: #6B6B6B;
  --colors-accent: #4A7C6F;
  --colors-accent-hover: #3d6b5f;
  --colors-accent-light: #E8F0EE;
  --colors-border: #E5E4E1;
  --colors-border-accent: #4A7C6F;
  --colors-error: #D64545;
  --colors-danger: #D64545;
  --colors-danger-hover: #B83B3B;
  --colors-overlay: rgba(0, 0, 0, 0.3);

  /* ============================================
     TYPOGRAPHY
     ============================================ */
  /* Fonts */
  --fonts-body: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  --fonts-heading: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  --fonts-mono: 'SF Mono', SFMono-Regular, Consolas, 'Liberation Mono', Menlo, monospace;

  /* Font Sizes */
  --font-sizes-0: 12px;
  --font-sizes-1: 14px;
  --font-sizes-2: 16px;
  --font-sizes-3: 18px;
  --font-sizes-4: 20px;
  --font-sizes-5: 24px;
  --font-sizes-6: 32px;

  /* Font Weights */
  --font-weights-normal: 400;
  --font-weights-medium: 500;
  --font-weights-bold: 600;

  /* Line Heights */
  --line-heights-tight: 1.25;
  --line-heights-normal: 1.5;
  --line-heights-relaxed: 1.75;

  /* Letter Spacing */
  --letter-spacings-tight: -0.02em;
  --letter-spacings-normal: 0;
  --letter-spacings-wide: 0.02em;

  /* ============================================
     SPACING - 8px Base
     ============================================ */
  --space-0: 0;
  --space-1: 8px;
  --space-2: 16px;
  --space-3: 24px;
  --space-4: 32px;
  --space-5: 48px;
  --space-6: 64px;
  --space-7: 96px;
  --space-8: 128px;

  /* ============================================
     SIZES
     ============================================ */
  --sizes-input-height: 44px;
  --sizes-button-height: 44px;
  --sizes-icon: 24px;
  --sizes-menu-width: 240px;
  --sizes-modal-min-width: 280px;
  --sizes-max-message-width: 80%;

  /* ============================================
     BORDER RADII
     ============================================ */
  --radii-none: 0;
  --radii-sm: 4px;
  --radii-md: 8px;
  --radii-lg: 12px;
  --radii-xl: 16px;
  --radii-pill: 9999px;

  /* ============================================
     BORDERS
     ============================================ */
  --borders-none: none;
  --borders-thin: 1px solid var(--colors-border);
  --borders-accent: 1px solid var(--colors-border-accent);
  --borders-dashed: 1px dashed var(--colors-border);

  /* ============================================
     SHADOWS - 3-Level Elevation
     ============================================ */
  --shadows-none: none;
  --shadows-low: 0 1px 3px rgba(0, 0, 0, 0.04);
  --shadows-medium: 0 2px 8px rgba(0, 0, 0, 0.06);
  --shadows-high: 0 4px 16px rgba(0, 0, 0, 0.08), 0 2px 4px rgba(0, 0, 0, 0.04);

  /* ============================================
     TRANSITIONS
     ============================================ */
  --transitions-fast: 150ms ease;
  --transitions-normal: 250ms ease;

  /* ============================================
     Z-INDICES
     ============================================ */
  --z-indices-base: 0;
  --z-indices-header: 10;
  --z-indices-menu: 100;
  --z-indices-modal: 200;

  /* ============================================
     OPACITY
     ============================================ */
  --opacities-disabled: 0.5;
  --opacities-muted: 0.7;
}
```

**Step 2: Verify file was created correctly**

Run: `cat web/static/tokens.css | head -20`
Expected: Shows the first 20 lines with color tokens

**Step 3: Commit**

```bash
git add web/static/tokens.css
git commit -m "feat(design): add design tokens file

Theme UI-inspired tokens for colors, typography, spacing,
radii, shadows, transitions, and z-indices."
```

---

## Task 2: Add Inter Font and Link Tokens

**Files:**
- Modify: `web/templates/layout.html`

**Step 1: Update layout.html to include Inter font and tokens.css**

In `web/templates/layout.html`, replace the `<head>` section (lines 1-17) with:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="mobile-web-app-capable" content="yes">
    <meta name="apple-mobile-web-app-capable" content="yes">
    <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
    <meta name="apple-mobile-web-app-title" content="Bobot">
    <link rel="apple-touch-icon" href="/static/icon-512x512.png" sizes="512x512">
    <meta name="format-detection" content="telephone=no">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no, viewport-fit=cover">
    <title>{{.Title}} - bobot</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&display=swap" rel="stylesheet">
    <link rel="stylesheet" href="/static/tokens.css">
    <link rel="stylesheet" href="/static/style.css">
    <link ref="manifest" href="/static/manifest.json">
    <script src="https://unpkg.com/htmx.org@2.0.4"></script>
    <script src="/static/ws-manager.js" defer></script>
</head>
```

**Step 2: Verify changes**

Run: `grep -n "Inter\|tokens.css" web/templates/layout.html`
Expected: Shows lines with Inter font and tokens.css links

**Step 3: Commit**

```bash
git add web/templates/layout.html
git commit -m "feat(design): add Inter font and link tokens.css"
```

---

## Task 3: Refactor Base Styles and Reset

**Files:**
- Modify: `web/static/style.css`

**Step 1: Replace root variables and base styles**

Replace lines 1-23 of `web/static/style.css` with:

```css
/* ============================================
   RESET & BASE
   ============================================ */
* {
  box-sizing: border-box;
  margin: 0;
  padding: 0;
}

body {
  font-family: var(--fonts-body);
  font-size: var(--font-sizes-2);
  line-height: var(--line-heights-normal);
  color: var(--colors-text);
  background-color: var(--colors-background);
  height: 100dvh;
  overflow: hidden;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

.hidden {
  display: none !important;
}
```

**Step 2: Verify the app still loads**

Run: `go build ./... && echo "Build OK"`
Expected: "Build OK"

**Step 3: Commit**

```bash
git add web/static/style.css
git commit -m "refactor(design): update base styles to use tokens"
```

---

## Task 4: Refactor Login/Signup Page Styles

**Files:**
- Modify: `web/static/style.css`

**Step 1: Replace login container styles**

Find and replace the Login Page section (approximately lines 25-79) with:

```css
/* ============================================
   LOGIN / SIGNUP PAGE
   ============================================ */
.login-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100dvh;
  padding: var(--space-3);
  gap: var(--space-2);
}

.login-container h1 {
  font-size: var(--font-sizes-6);
  font-weight: var(--font-weights-bold);
  color: var(--colors-text);
  margin-bottom: var(--space-2);
}

.login-form {
  width: 100%;
  max-width: 320px;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.login-form input {
  padding: var(--space-1) var(--space-2);
  height: var(--sizes-input-height);
  border: var(--borders-thin);
  border-radius: var(--radii-pill);
  font-family: var(--fonts-body);
  font-size: var(--font-sizes-2);
  background: var(--colors-surface);
  color: var(--colors-text);
  outline: none;
  transition: border-color var(--transitions-fast);
}

.login-form input:focus {
  border-color: var(--colors-accent);
}

.login-form input::placeholder {
  color: var(--colors-text-secondary);
}

.login-form button {
  height: var(--sizes-button-height);
  background-color: var(--colors-accent);
  color: var(--colors-surface);
  border: none;
  border-radius: var(--radii-pill);
  font-family: var(--fonts-body);
  font-size: var(--font-sizes-2);
  font-weight: var(--font-weights-medium);
  cursor: pointer;
  transition: background-color var(--transitions-fast);
}

.login-form button:hover {
  background-color: var(--colors-accent-hover);
}

.error {
  color: var(--colors-error);
  text-align: center;
  font-size: var(--font-sizes-1);
}
```

**Step 2: Test login page renders**

Run: `go build ./... && echo "Build OK"`
Expected: "Build OK"

**Step 3: Commit**

```bash
git add web/static/style.css
git commit -m "refactor(design): update login/signup styles to use tokens"
```

---

## Task 5: Refactor Chat Container and Header

**Files:**
- Modify: `web/static/style.css`

**Step 1: Replace chat container and header styles**

Find and replace the Chat Page section (approximately the .chat-container and .chat-header blocks) with:

```css
/* ============================================
   CHAT CONTAINER
   ============================================ */
.chat-container {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background-color: var(--colors-background);

  &:has(input:focus) {
    height: 100dvh;
  }
}

/* ============================================
   HEADER
   ============================================ */
.chat-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: calc(var(--space-2) + env(safe-area-inset-top)) calc(var(--space-3) + env(safe-area-inset-right)) var(--space-2) calc(var(--space-3) + env(safe-area-inset-left));
  background-color: var(--colors-background);
  position: sticky;
  top: 0;
  z-index: var(--z-indices-header);
}

.chat-header h1 {
  font-size: var(--font-sizes-3);
  font-weight: var(--font-weights-medium);
  color: var(--colors-text);
}

.header-nav {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.header-btn {
  background: none;
  border: none;
  color: var(--colors-accent);
  font-size: var(--font-sizes-1);
  font-weight: var(--font-weights-medium);
  padding: var(--space-1);
  cursor: pointer;
  transition: opacity var(--transitions-fast);
}

.header-btn:hover {
  opacity: var(--opacities-muted);
}

.nav-link {
  color: var(--colors-accent);
  text-decoration: none;
  font-size: var(--font-sizes-1);
  font-weight: var(--font-weights-medium);
  padding: var(--space-1);
  transition: opacity var(--transitions-fast);
}

.nav-link:hover {
  opacity: var(--opacities-muted);
}

.back-link {
  color: var(--colors-accent);
  text-decoration: none;
  font-size: var(--font-sizes-3);
  padding: var(--space-1);
  transition: opacity var(--transitions-fast);
}

.back-link:hover {
  opacity: var(--opacities-muted);
}

.menu-btn {
  background: none;
  border: none;
  color: var(--colors-accent);
  font-size: var(--font-sizes-4);
  cursor: pointer;
  padding: var(--space-1);
  transition: opacity var(--transitions-fast);
}

.menu-btn:hover {
  opacity: var(--opacities-muted);
}
```

**Step 2: Verify build**

Run: `go build ./... && echo "Build OK"`
Expected: "Build OK"

**Step 3: Commit**

```bash
git add web/static/style.css
git commit -m "refactor(design): update chat header styles to use tokens

Header now blends with background, uses accent color for interactive elements."
```

---

## Task 6: Refactor Message Styles

**Files:**
- Modify: `web/static/style.css`

**Step 1: Replace message styles**

Find and replace the message-related styles with:

```css
/* ============================================
   MESSAGES
   ============================================ */
.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-3) calc(var(--space-3) + env(safe-area-inset-right)) var(--space-3) calc(var(--space-3) + env(safe-area-inset-left));
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.message {
  display: flex;
  flex-direction: column;
  max-width: var(--sizes-max-message-width);
}

.message.user {
  align-self: flex-end;
  align-items: flex-end;
}

.message.assistant,
.message.system {
  align-self: flex-start;
  align-items: flex-start;
}

.message-meta {
  font-size: var(--font-sizes-0);
  color: var(--colors-text-secondary);
  margin-bottom: var(--space-0);
  padding: 0 var(--space-1);
}

.message-bubble {
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radii-lg);
  box-shadow: var(--shadows-low);
  background: var(--colors-surface);
  line-height: var(--line-heights-normal);
  word-wrap: break-word;
}

.message.user .message-bubble {
  background: var(--colors-accent-light);
  border: var(--borders-accent);
}

/* Legacy message styling (for messages without bubble wrapper) */
.message:not(:has(.message-bubble)) {
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radii-lg);
  box-shadow: var(--shadows-low);
  background: var(--colors-surface);
  line-height: var(--line-heights-normal);
  word-wrap: break-word;
}

.message.user:not(:has(.message-bubble)) {
  background: var(--colors-accent-light);
  border: var(--borders-accent);
}

.message-sender {
  font-size: var(--font-sizes-0);
  font-weight: var(--font-weights-medium);
  color: var(--colors-text-secondary);
  margin-bottom: var(--space-0);
}

.message-content {
  line-height: var(--line-heights-normal);
}
```

**Step 2: Verify build**

Run: `go build ./... && echo "Build OK"`
Expected: "Build OK"

**Step 3: Commit**

```bash
git add web/static/style.css
git commit -m "refactor(design): update message styles to use tokens

User messages now have sage accent tint, assistant messages are neutral white."
```

---

## Task 7: Refactor Input Area Styles

**Files:**
- Modify: `web/static/style.css`

**Step 1: Replace input area styles**

Find and replace the chat input styles with:

```css
/* ============================================
   INPUT AREA
   ============================================ */
.chat-input {
  padding: var(--space-2) calc(var(--space-3) + env(safe-area-inset-right)) calc(var(--space-2) + env(safe-area-inset-bottom)) calc(var(--space-3) + env(safe-area-inset-left));
  background-color: var(--colors-background);
  position: sticky;
  bottom: 0;

  &:has(input:focus) {
    padding-bottom: var(--space-2);
  }
}

.chat-input form {
  display: flex;
  align-items: flex-end;
  background: var(--colors-surface);
  border: var(--borders-thin);
  border-radius: var(--radii-pill);
  padding: var(--space-1);
  box-shadow: var(--shadows-low);
}

.chat-input input {
  flex: 1;
  border: none;
  outline: none;
  background: transparent;
  font-family: var(--fonts-body);
  font-size: var(--font-sizes-2);
  padding: var(--space-1) var(--space-2);
  color: var(--colors-text);
  line-height: var(--line-heights-normal);
}

.chat-input input::placeholder {
  color: var(--colors-text-secondary);
}

.chat-input button {
  width: var(--sizes-button-height);
  height: var(--sizes-button-height);
  background: none;
  border: none;
  color: var(--colors-border);
  font-size: var(--font-sizes-3);
  cursor: pointer;
  padding: var(--space-1);
  border-radius: var(--radii-pill);
  transition: color var(--transitions-fast);
  display: flex;
  align-items: center;
  justify-content: center;
}

.chat-input button:hover,
.chat-input button.active {
  color: var(--colors-accent);
}
```

**Step 2: Verify build**

Run: `go build ./... && echo "Build OK"`
Expected: "Build OK"

**Step 3: Commit**

```bash
git add web/static/style.css
git commit -m "refactor(design): update input area to pill style

Input now uses pill shape with integrated send button."
```

---

## Task 8: Refactor Menu Overlay Styles

**Files:**
- Modify: `web/static/style.css`

**Step 1: Replace menu overlay styles**

Find and replace the menu overlay styles with:

```css
/* ============================================
   MENU OVERLAY
   ============================================ */
.menu-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: var(--colors-overlay);
  display: flex;
  justify-content: flex-end;
  z-index: var(--z-indices-menu);
}

.menu {
  background-color: var(--colors-surface);
  width: var(--sizes-menu-width);
  padding: calc(var(--space-3) + env(safe-area-inset-top)) calc(var(--space-3) + env(safe-area-inset-right)) calc(var(--space-3) + env(safe-area-inset-bottom)) var(--space-3);
  box-shadow: var(--shadows-high);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.menu-item {
  padding: var(--space-2);
  color: var(--colors-text);
  border-radius: var(--radii-md);
  cursor: pointer;
  font-size: var(--font-sizes-2);
  transition: background-color var(--transitions-fast);
  text-align: left;
  background: none;
  border: none;
  width: 100%;
}

.menu-item:hover {
  background-color: var(--colors-background);
}

.menu button:not(.menu-item) {
  width: 100%;
  padding: var(--space-2);
  border: none;
  border-radius: var(--radii-md);
  font-family: var(--fonts-body);
  font-size: var(--font-sizes-2);
  font-weight: var(--font-weights-medium);
  cursor: pointer;
  transition: background-color var(--transitions-fast);
}

.members-list {
  padding: var(--space-2) 0;
}

.members-list strong {
  font-size: var(--font-sizes-1);
  color: var(--colors-text-secondary);
  text-transform: uppercase;
  letter-spacing: var(--letter-spacings-wide);
}

.member {
  padding: var(--space-1) 0;
  font-size: var(--font-sizes-2);
}

.danger-btn {
  background-color: var(--colors-danger);
  color: var(--colors-surface);
}

.danger-btn:hover {
  background-color: var(--colors-danger-hover);
}
```

**Step 2: Verify build**

Run: `go build ./... && echo "Build OK"`
Expected: "Build OK"

**Step 3: Commit**

```bash
git add web/static/style.css
git commit -m "refactor(design): update menu overlay styles to use tokens"
```

---

## Task 9: Refactor Groups Page Styles

**Files:**
- Modify: `web/static/style.css`

**Step 1: Replace groups page styles**

Find and replace the groups page styles with:

```css
/* ============================================
   GROUPS PAGE
   ============================================ */
.groups-container {
  max-width: 600px;
  margin: 0 auto;
  padding: var(--space-2);
  padding-top: calc(var(--space-2) + env(safe-area-inset-top));
  padding-bottom: calc(var(--space-2) + env(safe-area-inset-bottom));
}

.groups-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--space-2) 0;
}

.groups-header h1 {
  font-size: var(--font-sizes-3);
  font-weight: var(--font-weights-medium);
  color: var(--colors-text);
}

.groups-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.group-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--space-2) var(--space-3);
  background: var(--colors-surface);
  border-radius: var(--radii-lg);
  box-shadow: var(--shadows-low);
  text-decoration: none;
  color: inherit;
  transition: box-shadow var(--transitions-fast);
}

.group-item:hover {
  box-shadow: var(--shadows-medium);
}

.group-name {
  font-size: var(--font-sizes-2);
  font-weight: var(--font-weights-medium);
  color: var(--colors-text);
}

.group-members {
  font-size: var(--font-sizes-0);
  color: var(--colors-text-secondary);
}

.create-btn {
  font-size: var(--font-sizes-4);
  padding: var(--space-1) var(--space-2);
  background: var(--colors-accent);
  color: var(--colors-surface);
  border: none;
  border-radius: var(--radii-pill);
  cursor: pointer;
  transition: background-color var(--transitions-fast);
}

.create-btn:hover {
  background-color: var(--colors-accent-hover);
}

.card-action {
  background: transparent;
  border: var(--borders-dashed);
  border-radius: var(--radii-lg);
  padding: var(--space-2) var(--space-3);
  color: var(--colors-text-secondary);
  text-align: center;
  cursor: pointer;
  font-family: var(--fonts-body);
  font-size: var(--font-sizes-2);
  transition: border-color var(--transitions-fast), color var(--transitions-fast);
}

.card-action:hover {
  border-color: var(--colors-accent);
  color: var(--colors-accent);
}
```

**Step 2: Verify build**

Run: `go build ./... && echo "Build OK"`
Expected: "Build OK"

**Step 3: Commit**

```bash
git add web/static/style.css
git commit -m "refactor(design): update groups page styles to use tokens"
```

---

## Task 10: Refactor Modal Styles

**Files:**
- Modify: `web/static/style.css`

**Step 1: Replace modal styles**

Find and replace the modal styles with:

```css
/* ============================================
   MODAL
   ============================================ */
.modal {
  position: fixed;
  inset: 0;
  background: var(--colors-overlay);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: var(--z-indices-modal);
  padding: var(--space-3);
}

.modal-content {
  background: var(--colors-surface);
  padding: var(--space-3);
  border-radius: var(--radii-xl);
  box-shadow: var(--shadows-high);
  min-width: var(--sizes-modal-min-width);
  max-width: 90%;
}

.modal-content h2 {
  font-size: var(--font-sizes-4);
  font-weight: var(--font-weights-medium);
  color: var(--colors-text);
  margin-bottom: var(--space-2);
}

.modal-content input {
  width: 100%;
  padding: var(--space-1) var(--space-2);
  height: var(--sizes-input-height);
  border: var(--borders-thin);
  border-radius: var(--radii-pill);
  font-family: var(--fonts-body);
  font-size: var(--font-sizes-2);
  background: var(--colors-surface);
  color: var(--colors-text);
  outline: none;
  margin-bottom: var(--space-2);
  transition: border-color var(--transitions-fast);
}

.modal-content input:focus {
  border-color: var(--colors-accent);
}

.modal-content input::placeholder {
  color: var(--colors-text-secondary);
}

.modal-actions {
  display: flex;
  gap: var(--space-2);
  justify-content: flex-end;
}

.modal-actions button {
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radii-pill);
  cursor: pointer;
  font-family: var(--fonts-body);
  font-size: var(--font-sizes-1);
  font-weight: var(--font-weights-medium);
  transition: background-color var(--transitions-fast);
}

.modal-actions button[type="button"] {
  background: transparent;
  color: var(--colors-accent);
  border: none;
}

.modal-actions button[type="button"]:hover {
  opacity: var(--opacities-muted);
}

.modal-actions button[type="submit"] {
  background: var(--colors-accent);
  color: var(--colors-surface);
  border: none;
}

.modal-actions button[type="submit"]:hover {
  background-color: var(--colors-accent-hover);
}
```

**Step 2: Verify build**

Run: `go build ./... && echo "Build OK"`
Expected: "Build OK"

**Step 3: Commit**

```bash
git add web/static/style.css
git commit -m "refactor(design): update modal styles to use tokens"
```

---

## Task 11: Refactor Utility Styles and Animations

**Files:**
- Modify: `web/static/style.css`

**Step 1: Replace typing indicator and utility styles**

Find and replace the remaining utility styles (typing indicator, empty/loading states, authenticated container, loading spinner) with:

```css
/* ============================================
   TYPING INDICATOR
   ============================================ */
.typing-indicator {
  display: flex;
  gap: var(--space-1);
  padding: var(--space-1) var(--space-2);
  align-self: flex-start;
  background: var(--colors-surface);
  border-radius: var(--radii-lg);
  box-shadow: var(--shadows-low);
}

.typing-indicator span {
  width: 8px;
  height: 8px;
  background-color: var(--colors-text-secondary);
  border-radius: 50%;
  animation: typing 1.4s infinite ease-in-out;
}

.typing-indicator span:nth-child(2) {
  animation-delay: 0.2s;
}

.typing-indicator span:nth-child(3) {
  animation-delay: 0.4s;
}

@keyframes typing {
  0%, 60%, 100% {
    transform: translateY(0);
  }
  30% {
    transform: translateY(-4px);
  }
}

/* ============================================
   EMPTY & LOADING STATES
   ============================================ */
.empty,
.loading {
  text-align: center;
  padding: var(--space-5);
  color: var(--colors-text-secondary);
  font-size: var(--font-sizes-2);
}

/* ============================================
   AUTHENTICATED TRANSITION
   ============================================ */
.authenticated-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100dvh;
  gap: var(--space-3);
}

.authenticated-container h1 {
  font-size: var(--font-sizes-6);
  font-weight: var(--font-weights-bold);
  color: var(--colors-text);
}

.loading-spinner {
  width: 40px;
  height: 40px;
  border: 3px solid var(--colors-border);
  border-top-color: var(--colors-accent);
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
```

**Step 2: Verify build**

Run: `go build ./... && echo "Build OK"`
Expected: "Build OK"

**Step 3: Commit**

```bash
git add web/static/style.css
git commit -m "refactor(design): update utility styles and animations to use tokens"
```

---

## Task 12: Final Cleanup and Verification

**Files:**
- Modify: `web/static/style.css`

**Step 1: Remove any remaining old variables**

Search for any remaining hardcoded colors or old CSS variables:

Run: `grep -n "#007AFF\|#ff3b30\|#e9e9eb\|--primary-color\|--user-msg\|--assistant-msg" web/static/style.css`
Expected: No output (all old references removed)

If any remain, replace them with appropriate token references.

**Step 2: Verify the complete style.css structure**

Run: `wc -l web/static/style.css`
Expected: File should be organized with clear section headers

**Step 3: Run the app and manually verify**

Run: `go run main.go`
Open browser to http://localhost:8080 (or configured port)

Verify:
- Login page shows warm gray background, sage green button
- Chat page has minimal header blending with background
- Messages have subtle shadows, user messages have sage tint
- Input is pill-shaped
- Groups page cards have proper styling
- Modal has rounded corners and proper button styles

**Step 4: Final commit**

```bash
git add web/static/style.css
git commit -m "refactor(design): complete design system migration

All components now use design tokens from tokens.css.
Palette changed from iOS blue to 'Soft Stone' with sage green accent."
```

---

## Summary

After completing all tasks:

- `web/static/tokens.css` - New file with all design tokens
- `web/static/style.css` - Refactored to use tokens, organized by component
- `web/templates/layout.html` - Updated with Inter font and tokens.css link

The design system is now in place with:
- Warm neutral "Soft Stone" palette
- Inter typography
- 8px spacing scale
- Pill-shaped inputs and buttons
- 3-level shadow elevation
- Structure ready for future dark mode
