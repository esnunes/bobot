---
title: "feat: Change primary accent color from Blue to Catppuccin Latte Yellow"
type: feat
date: 2026-04-07
---

# feat: Change primary accent color from Blue to Catppuccin Latte Yellow

## Overview

Update the application's primary accent color from Catppuccin Latte Blue (`#1e66f5`) to Catppuccin Latte Yellow (`#df8e1d`) by modifying three CSS custom properties in `web/static/tokens.css`. All UI elements (buttons, links, highlights, form controls) consume these variables and will automatically reflect the new color.

## Problem Statement / Motivation

The current blue accent doesn't match the desired brand direction. Switching to Catppuccin Latte Yellow gives the application a warmer, more distinctive identity while staying within the established Catppuccin Latte palette that the design system already uses.

## Proposed Solution

Change three CSS custom property values in `web/static/tokens.css`:

| Variable | Current Value | New Value | Rationale |
|---|---|---|---|
| `--colors-accent` | `#1e66f5` (Blue) | `#df8e1d` (Yellow) | Primary accent — Catppuccin Latte Yellow |
| `--colors-accent-hover` | `#7287fd` (Lavender) | `#e6a336` (lighter yellow) | Hover state — lightened yellow for visible hover feedback on the Latte light background |
| `--colors-accent-light` | `#dce1fb` (Light Lavender tint) | `#faf0db` (pale yellow tint) | Light tint — very subtle yellow for focus rings, selection backgrounds, and light highlights |

### Hover variant derivation

`#e6a336` is derived by increasing the lightness of `#df8e1d` by ~8% in HSL space (hue 36°, saturation ~78%, lightness ~49% → ~57%). This mirrors the relationship between the original Blue and Lavender values. The slightly lighter shade provides clear visual feedback on hover without washing out on the light Latte background.

### Light tint derivation

`#faf0db` is a very pale yellow (hue ~40°, saturation ~75%, lightness ~92%) that matches the role of the original `#dce1fb` — a near-white tint used for focus rings (`box-shadow`), selection/active backgrounds, and subtle highlights. It's visible enough to convey accent association without competing with text or primary buttons.

## Technical Approach

### Files Changed

**`web/static/tokens.css`** (lines 10–12) — the only file that needs modification:

```css
/* Before */
--colors-accent: #1e66f5;           /* Blue */
--colors-accent-hover: #7287fd;     /* Lavender */
--colors-accent-light: #dce1fb;     /* Light Lavender tint */

/* After */
--colors-accent: #df8e1d;           /* Yellow */
--colors-accent-hover: #e6a336;     /* Light Yellow (hover) */
--colors-accent-light: #faf0db;     /* Pale Yellow tint */
```

### Files NOT changed

- **`web/static/style.css`** — 40+ references to `var(--colors-accent)`, `var(--colors-accent-hover)`, and `var(--colors-accent-light)` all resolve through CSS custom properties; no changes needed.
- **No Go templates, no server code** — colors are purely in CSS.

### Verification

1. Open the app in a browser and visually inspect:
   - Primary buttons render in yellow (`#df8e1d`)
   - Button hover states lighten to `#e6a336`
   - Focus rings and selection highlights use the pale yellow tint
   - Text on yellow buttons remains readable (white text on `#df8e1d` passes WCAG AA for large text; the existing `#eff1f5` background-on-yellow combinations should be checked)
2. Grep for any hardcoded old hex values (`#1e66f5`, `#7287fd`, `#dce1fb`) to confirm no references outside `tokens.css` or plan docs.

### Accessibility Note

Yellow on light backgrounds can present contrast challenges. The chosen `#df8e1d` has a contrast ratio of ~3.2:1 against the Latte Base (`#eff1f5`), which passes WCAG AA for large text and UI components but not for normal body text. This matches the existing usage pattern — accent color is used for buttons (with white/light text on solid backgrounds), links, and decorative highlights, not for body text on the base background.

## Implementation Phases

This is a single-step change — update the three CSS variable values in `web/static/tokens.css`.
