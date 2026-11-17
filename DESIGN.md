# DESIGN.md — "Sorting office"

*Concept.* A postal sorting office: paper, ink, a rubber stamp. Lists read like a manifest: flat rows split by hairlines. There are no cards, no shadows on resting surfaces, no gradients, no rounded pills.

*Colour tokens.* Contrast ratios were computed for text on `paper` / `sheet` / `sunken`; all body text pairs pass WCAG AA (4.5:1).

| Token | Light | Dark | Use |
|---|---|---|---|
| `paper` | `#faf5ef` | `#18130e` | page background |
| `sheet` | `#fefcfa` | `#201a16` | raised surface: dialogs, bottom sheets, nav bars, inputs |
| `sunken` | `#f3ede5` | `#120d09` | inset areas: raw headers, code, secret box, table head |
| `hairline` | `#ddd4cc` | `#3b342e` | 1px dividers between rows and sections (decorative, 1.35:1) |
| `control` | `#90847a` | `#7e7269` | input and switch outlines (3.4:1 or better, passes 3:1 for controls) |
| `ink` | `#281c17` | `#ede7df` | primary text (15:1) |
| `ink2` | `#584a43` | `#c5bcb3` | secondary text, labels (7.3:1 or better) |
| `muted` | `#73665f` | `#a69d94` | tertiary text, timestamps (4.76:1 or better) |
| `accent` | `#c22823` | `#f68678` | the stamp: primary button fill, active nav mark, links |
| `accentHover` | `#a91515` | `#ff9f92` | hover and pressed state of `accent` |
| `onAccent` | `#fefbf8` | `#1a0e0d` | text on an `accent` fill (5.65:1 light, 7.73:1 dark) |
| `sent` | `#206b38` | `#76cf8a` | result: sent |
| `rejected` | `#84550f` | `#edbb64` | result: rejected by this gateway |
| `failed` | `#a51d2b` | `#fe8a88` | result: failed at the upstream; also destructive actions |
| `focus` | `#0053a5` | `#87bafd` | focus ring, 2px, offset 2px. Blue on purpose: it must not be confused with the accent |

The three result colours appear only on a result (a status mark, a chart bar, a count). Nothing else is green or amber.

*Type.* `Atkinson Hyperlegible Next Variable` for everything; `Atkinson Hyperlegible Mono Variable` for addresses, IP addresses, host names, ports, secrets, raw headers and the times in log rows. Sizes (rem): 0.8125 caption, 0.875 secondary, 1 body, 1.125 section heading, 1.5 page title (1.75 from `lg`). Weights: 400 body, 600 headings and the active nav item, 700 only for the three big counts on Overview. Line height 1.5 body, 1.25 headings. Numbers in tables use `font-variant-numeric: tabular-nums` (Tailwind `tabular-nums`). Line length of prose at most 68 characters (`max-w-prose`).

*Space and shape.* 4px grid; the spacing steps used are 4, 8, 12, 16, 24, 32, 48. Page gutter 16px on phone, 24px on tablet, 32px from `lg`. Content column at most 1120px. Corner radius 2px on controls and dialogs, 0 on rows and sections. Rows are at least 56px tall on phone and 44px from `lg`.

*Elevation.* None at rest. Dialogs and bottom sheets sit on `sheet` with a 1px `hairline` border and the browser's backdrop; no drop shadows.

*Motion.* 120ms ease-out on colour and opacity only. No movement on load. Everything is disabled under `prefers-reduced-motion`.

*Navigation.* Below `lg`: a top bar with the page title and a bottom tab bar with five items (Overview, Logins, Messages, Audit, More). From `lg`: a 232px left rail with Overview, Logins, Messages, Audit, API tokens, and at its foot the signed-in name, the theme switch and "Sign out". The active item carries a 3px `accent` bar (top edge of the tab, left edge of the rail item) and weight 600.

*Components.* Page header (title, optional one-line description, optional primary action). Section (heading + hairline + content). Manifest row (the list unit). Status mark (icon + word). Definition list (label over value on phone, label beside value from `sm`). Empty state (one sentence + one action). Error state (what failed + "Try again"). Confirm dialog (destructive actions only). Secret reveal (shown once).

*Bans.* Card grids, drop shadows, gradients, pill badges, emoji, centred body text, icon-only buttons without an accessible name, skeleton shimmer animation, toasts for errors (errors stay on the page until dismissed or fixed), placeholder text used as a label, disabled buttons without a visible reason.
