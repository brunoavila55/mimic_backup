# DESIGN.md — Mimic Visual Language ("Schematic")

Single source of truth for Mimic's visual identity. All UI work should derive
its colors, type, spacing and radii from the tokens in `static/css/style.css`
`:root` — never hardcode a new hex value when a token already means the same
thing.

## Why "Schematic"

Mimic isn't a consumer product — it's a compliance/audit tool network admins
open to answer one question: *what changed, and is it safe?* The interface
spends most of its time showing dense tables, config diffs, and SHA-256-backed
version history. The previous theme (`v4.0`) was a generic "near-black
background + single neon-green accent" dark mode — the kind of look most
AI-assisted dark themes converge on regardless of what the product actually
does, and it had a real bug baked in: the brand accent and the "success /
diff-add" color were the *same* green, so a healthy status and a clickable
link looked identical.

"Schematic" grounds the palette and layout language in the artifacts network
engineers actually work with: topology blueprints, patch-panel diagrams, rack
elevation drawings, and the unified-diff (`+`/`-`) convention every admin
already reads instinctively from `git diff` or a router's `show run` output.
The brand color is a cyan "trace on a scope," reserved strictly for
interactive/brand elements — it never doubles as a status color, so status
(success/warning/danger) always means the same thing everywhere.

## Color

Named palette (6 core hues + neutrals):

| Token | Hex | Role |
|---|---|---|
| Ink | `#0B0E13` | Base background — an unlit equipment chassis |
| Slate | `#12161F` / `#171C27` | Card and raised-surface backgrounds |
| Line | `#232838` | Borders — blueprint gridline gray-blue |
| Signal (brand) | `#4FD6EF` | Interactive/brand only: links, primary actions, focus, live indicators — **never** a status color |
| Continuity (success) | `#35C778` | Healthy state, diff-add — matches the diff convention admins already know |
| Fault (danger) | `#F0553D` | Failure, diff-remove |
| Caution (warning) | `#F2A93C` | Degraded / needs-attention |
| Paper (text) | `#ECEEE9` | Primary text — warm off-white, not pure white |

Info badges use a desaturated slate-blue (`--info: #7C93B8`), distinct from
the bright brand cyan, for neutral annotations that aren't a call to action.

Rule of thumb: if a color communicates a *state* (pass/fail/warn), it comes
from the semantic tokens. If it communicates *"this is interactive / this is
Mimic,"* it's `--accent`. Don't reuse one for the other.

## Typography

Three roles, two families:

- **Display** — `Space Grotesk` (headings, eyebrows, big stat numbers). A
  geometric grotesque with distinctive numerals — used with restraint, only
  on `h1`–`h6` and the type scale's largest sizes, so it reads as an accent
  rather than the whole UI's voice.
- **Body/UI** — `Inter` (paragraphs, labels, buttons, nav, table text). Kept
  from the previous system because it's genuinely good at small sizes in
  dense tables — this axis wasn't worth spending novelty on.
- **Mono/data** — `JetBrains Mono` (config snippets, diffs, hashes, IPs,
  timestamps, schedule times). Config data should always look like config
  data.

## Spacing & Radius

A `--space-1` … `--space-8` scale (4px → 48px) is now defined in `:root` for
new work. Existing component CSS keeps its hand-tuned pixel values for now —
retrofitting every rule is out of scope for the token pass and will happen
incrementally as each screen is touched in P1/P2.

Radii stay small and precise (`4–12px`) — this is an instrument panel, not a
soft consumer app, so nothing should feel pillowy.

## States

- **Focus** — every interactive element gets a visible ring via a global
  `:focus-visible` rule (2px accent outline) so keyboard users never lose
  their place; it's suppressed for mouse/touch by design (`:focus-visible`,
  not `:focus`). Form inputs additionally get a `--focus-ring` box-shadow.
- **Hover** — existing `--bg-hover` / `--border-focus` tokens, unchanged in
  behavior, just repainted with the new palette.
- **Disabled** — unchanged: `opacity: .5` + `pointer-events: none`.
- **Motion** — `prefers-reduced-motion` is now respected globally.

## Signature element

A near-invisible 48px blueprint grid (`--grid-line`, ~3.5% opacity cyan)
sits behind every page — a quiet nod to schematic drafting paper that never
competes with content. It's the one deliberate flourish; everything else
stays disciplined so the grid and the diff colors do the storytelling.

## Scope note

This pass (P0) only touches `:root` tokens and truly global chrome (fonts,
focus ring, page background, heading font-family) — it deliberately does not
rewrite any screen's layout or components.

Update (P3): `login.html`/`login.css` and the setup/onboarding flow
(`setup_superuser.html`, `setup_database.html`) have since been reconciled to
the Schematic palette — their brand/interactive elements (buttons, focus
states, stepper, eyebrow text) now use `--accent`, and their status elements
(password-strength meter, setup checklist "ready" state) use `--success`,
matching the rule of thumb above. The leftover hardcoded green values tied to
the *old* accent (dashboard onboarding banner glow, node import "schedule
preview" chip, the SFTP export-destination card border) have also been swept
to the current tokens. The purple ambient glows on the login brand panel are
a separate decorative color, unrelated to this brand-accent cleanup, and were
left untouched.

`estilo.md` still holds valid guidance on logo assets and the login-page
Figma/backend contract; its color values are superseded by this document.
