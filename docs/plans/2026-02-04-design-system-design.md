# Bobot Design System

A Theme UI-inspired design system for bobot-web, implementing a "Soft Stone" aesthetic with warm neutrals and a sage green accent.

## Goals

- **Consistency** — Unified look and feel across all pages
- **Future scaling** — Foundation for adding features and pages
- **Theming support** — Structure that supports dark mode later
- **Developer experience** — Clear mental model when writing CSS

## Design Direction

**Soft modern** — Subtle shadows, muted colors, generous spacing. Warm without being playful, minimal without being cold.

Reference characteristics:
- Warm neutral background (not stark white)
- Minimal header that blends with content
- Subtle message bubbles with light shadow
- Pill-shaped inputs and buttons
- Generous whitespace

## File Structure

```
web/static/
├── tokens.css      # Design tokens (new)
├── style.css       # Component styles (refactored)
└── ...
```

## Design Tokens

### Colors — Palette "Soft Stone"

| Token | Value | Usage |
|-------|-------|-------|
| `--colors-background` | `#F7F6F3` | Page background |
| `--colors-surface` | `#FFFFFF` | Cards, bubbles, inputs |
| `--colors-text` | `#1A1A1A` | Primary text |
| `--colors-text-secondary` | `#6B6B6B` | Metadata, hints |
| `--colors-accent` | `#4A7C6F` | Buttons, links, interactive |
| `--colors-accent-hover` | `#3d6b5f` | Accent hover state |
| `--colors-accent-light` | `#E8F0EE` | User message background |
| `--colors-border` | `#E5E4E1` | Subtle borders |
| `--colors-border-accent` | `#4A7C6F` | User message border |

### Typography

**Font family:** Inter (with system fallbacks)

```css
--fonts-body: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif;
--fonts-heading: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif;
--fonts-mono: 'SF Mono', SFMono-Regular, Consolas, monospace;
```

**Font sizes:**

| Token | Value |
|-------|-------|
| `--font-sizes-0` | `12px` |
| `--font-sizes-1` | `14px` |
| `--font-sizes-2` | `16px` |
| `--font-sizes-3` | `18px` |
| `--font-sizes-4` | `20px` |
| `--font-sizes-5` | `24px` |
| `--font-sizes-6` | `32px` |

**Font weights:**

| Token | Value |
|-------|-------|
| `--font-weights-normal` | `400` |
| `--font-weights-medium` | `500` |
| `--font-weights-bold` | `600` |

**Line heights:**

| Token | Value |
|-------|-------|
| `--line-heights-tight` | `1.25` |
| `--line-heights-normal` | `1.5` |
| `--line-heights-relaxed` | `1.75` |

### Spacing — 8px Base

| Token | Value |
|-------|-------|
| `--space-0` | `0` |
| `--space-1` | `8px` |
| `--space-2` | `16px` |
| `--space-3` | `24px` |
| `--space-4` | `32px` |
| `--space-5` | `48px` |
| `--space-6` | `64px` |
| `--space-7` | `96px` |
| `--space-8` | `128px` |

### Border Radii

| Token | Value | Usage |
|-------|-------|-------|
| `--radii-none` | `0` | Sharp corners |
| `--radii-sm` | `4px` | Subtle rounding |
| `--radii-md` | `8px` | General rounding |
| `--radii-lg` | `12px` | Messages, cards |
| `--radii-xl` | `16px` | Modals |
| `--radii-pill` | `9999px` | Inputs, buttons |

### Shadows — 3-Level Elevation

| Token | Value | Usage |
|-------|-------|-------|
| `--shadows-none` | `none` | Flat elements |
| `--shadows-low` | `0 1px 3px rgba(0,0,0,0.04)` | Messages, cards |
| `--shadows-medium` | `0 2px 8px rgba(0,0,0,0.06)` | Dropdowns, menus |
| `--shadows-high` | `0 4px 16px rgba(0,0,0,0.08), 0 2px 4px rgba(0,0,0,0.04)` | Modals |

### Transitions

| Token | Value |
|-------|-------|
| `--transitions-fast` | `150ms ease` |
| `--transitions-normal` | `250ms ease` |

### Z-Indices

| Token | Value |
|-------|-------|
| `--z-indices-base` | `0` |
| `--z-indices-header` | `10` |
| `--z-indices-menu` | `100` |
| `--z-indices-modal` | `200` |

## Component Designs

### Page Layout

```
┌─────────────────────────────────────────┐
│  Header (contextual)                    │  ← blends with background
├─────────────────────────────────────────┤
│                                         │
│  Content area                           │  ← scrollable
│  (messages, groups list)                │
│                                         │
├─────────────────────────────────────────┤
│  Input area (chat pages only)           │  ← fixed bottom
└─────────────────────────────────────────┘
```

### Header

Contextual header that blends with background (no border/shadow).

| Page | Left | Center | Right |
|------|------|--------|-------|
| Private chat | Groups | "bobot" | Menu (⋮) |
| Groups list | ← Back | "Groups" | Menu (⋮) |
| Group chat | ← Back | Group name | Menu (⋮) |
| Login/Signup | — | "bobot" | — |

### Messages

- **User messages:** Right-aligned, light sage background (`--colors-accent-light`), accent border
- **Assistant messages:** Left-aligned, white background, neutral
- **Metadata:** Above bubble, small text with sender name and timestamp
- **Bubble shape:** `--radii-lg` (12px), `--shadows-low`

```
                         Alice · 23:23:54
                      ┌──────────────────┐
                      │ Hello world      │  ← user (accent tint)
                      └──────────────────┘

bobot · 23:23:56
┌───────────────────────────────────────┐
│ Hi! How can I help you today?         │  ← assistant (neutral)
└───────────────────────────────────────┘
```

### Input Area

Expandable pill-shaped input with integrated send icon.

- Single line by default, expands up to ~4 lines
- Send icon inside pill, right side
- Send icon muted when empty, accent color when content exists

### Buttons

Three variants, all pill-shaped:

| Variant | Style | Usage |
|---------|-------|-------|
| Primary | Filled accent | Main actions (Submit, Create) |
| Secondary | Outlined | Secondary actions |
| Ghost | Text only | Tertiary actions (Cancel) |

### Cards

Used for group list items.

- White background
- `--radii-lg` border radius
- `--shadows-low` elevation
- Title + subtitle layout

"Create new group" uses dashed border style to differentiate from content cards.

### Modal

Centered overlay with high elevation.

- `--radii-xl` border radius
- `--shadows-high` elevation
- Semi-transparent backdrop (`rgba(0,0,0,0.3)`)

### Menu Overlay

Slide-in panel from right.

- Full height, 240px width
- `--shadows-high` elevation
- Semi-transparent backdrop
- Menu items with hover state

## Implementation

### Font Loading

Add to `layout.html` head:

```html
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&display=swap" rel="stylesheet">
<link rel="stylesheet" href="/static/tokens.css">
```

### Migration Approach

1. Create `tokens.css` with all design tokens
2. Refactor `style.css` incrementally, replacing hardcoded values with token references
3. Update HTML templates to use new class names where needed
4. Keep existing functionality working throughout

### Future: Dark Mode

The token structure supports dark mode via CSS custom property overrides:

```css
@media (prefers-color-scheme: dark) {
  :root {
    --colors-background: #1A1A1A;
    --colors-surface: #2A2A2A;
    --colors-text: #F7F6F3;
    --colors-text-secondary: #9A9A9A;
    /* ... other overrides */
  }
}
```

## Component Class Reference

| Component | Classes |
|-----------|---------|
| Layout | `.page`, `.header`, `.content`, `.input-area` |
| Header | `.header`, `.header-title`, `.header-btn` |
| Messages | `.message`, `.message-user`, `.message-assistant`, `.message-meta`, `.message-bubble` |
| Input | `.input-container`, `.input-field`, `.input-send` |
| Buttons | `.btn`, `.btn-primary`, `.btn-secondary`, `.btn-ghost` |
| Cards | `.card`, `.card-title`, `.card-subtitle`, `.card-action` |
| Forms | `.form`, `.form-input`, `.form-label` |
| Modal | `.overlay-backdrop`, `.modal`, `.modal-title`, `.modal-actions` |
| Menu | `.menu-panel`, `.menu-item` |
| Utilities | `.hidden` |
