# UI verification — screenshot suite, layout and accessibility checks

Date: 2026-11-17. Scope: Phase 7 Task 7.13 (smtp-gateway v2 plan).

## Command

```bash
cd ui && bun run build
SG_CHROMIUM=/home/ubuntu/.cache/ms-playwright/chromium-1243/chrome-linux-arm64/chrome bun run shots
```

**Deviation from the plan:** the plan says `SG_CHROMIUM=/usr/bin/chromium-browser`. That path does
not exist on this host (no system Chromium package installed); only Playwright's own managed
Chromium (`~/.cache/ms-playwright/chromium-1243`, matching the pinned `@playwright/test` 1.65.1)
is present. `playwright.config.ts` was left exactly as the plan specifies (`SG_CHROMIUM` env var,
falls back to `undefined` i.e. Playwright's bundled browser, `--no-sandbox`); only the value of the
env var differs for this run.

## Result

Final run: **184 passed, 0 failed** (`184 passed (3.5m)`). `ls shots-out | wc -l` → **184**, matching
the plan's expected count (18 page states × 5 viewports × 2 themes = 180, plus the filter-sheet
state on 2 phone viewports × 2 themes = 4).

Every test runs all six checks from the plan (no horizontal scroll; 44×44 touch targets below
1024px; no leaked `req-fixture-` string or the route's own numeric id as a whole word; zero axe
violations for `wcag2a`/`wcag2aa`/`wcag21aa`/`wcag22aa`; no console errors or failed requests; one
`h1` and a `document.title` ending in `· SMTP Gateway`).

## Problems found and fixed (fix loop)

1. **Contrast failure on unselected `ToggleButton` text** (axe `color-contrast`, 4.48:1 vs the
   4.5:1 AA minimum). MUI's default `text.secondary` picks up a reduced-opacity blend on the
   unselected state. Fixed in `ui/src/theme/theme.ts`: `MuiToggleButton.styleOverrides.root` now
   sets full-opacity `var(--sg-ink2)` (unselected) / `var(--sg-ink)` (selected) directly.
2. **`ToggleButton` and `Switch` touch targets below 44px** on phone/tablet (`ToggleButton` ~38px
   tall and ~43.6px wide at `size="small"`; the Switch's native `input[role=switch]` hit box was
   38px tall). Fixed in the same theme file: `MuiToggleButton.styleOverrides.root` adds
   `minHeight: 44` / `minWidth: 44`; `MuiSwitch.styleOverrides.input` extends the already-wider
   (300%) hit area vertically too (`height: '300%', top: '-100%'`), matching the pattern MUI
   already uses horizontally, so the drawn switch stays the same size while the tappable area grows.
3. **`vite preview`'s dev proxy swallowed `/logins*` routes.** `server.proxy` matches `/login` by
   string prefix, which also matches `/logins`, `/logins/new`, `/logins/:id` — every one of those
   routes hit `ECONNREFUSED 127.0.0.1:18080` instead of the SPA. Fixed in `ui/vite.config.ts`: the
   `/login` proxy entry now has a `bypass` that only proxies the exact `/login` (and `/login?...`)
   path and otherwise returns the request to Vite's own handling.
4. **A link rendered as `inline-block` with no min-height** ("All messages from this login" on the
   login detail page) measured 24px tall. Fixed in `ui/src/pages/LoginDetail.tsx`: `inline-flex
   min-h-11 items-center`.
5. **Horizontal overflow on tablet landscape (1180px) Overview**, both themes, both the data and
   empty variants. The "Settings for mail clients" Facts row paired a `Mono` hostname with a
   `CopyButton` in a non-wrapping `inline-flex`; at the three-column `lg:grid-cols-3` width each
   column is only ~350px, and the row didn't wrap or shrink. Fixed in `ui/src/pages/Overview.tsx`
   and `ui/src/components/ClientSettings.tsx`: the wrapper is now `flex min-w-0 flex-wrap
   items-center gap-2` and `Mono` gets `min-w-0`.
6. **`/signed-out` and `/signed-out#error=…` never set `document.title`** (they render outside
   `PageHeader`, which is what normally does it). Fixed in `ui/src/pages/SignedOut.tsx`: a small
   `useEffect` sets the title itself.
7. **Credential id 11 (used for the `/logins/11` shot) coincidentally leaked its own id as a
   timestamp.** Its `last_used_at` was `11:50 UTC`, and several of its messages (same login,
   55-minute cadence from a 12:00 anchor) also fell in the 11 o'clock hour, so `formatDateTime`
   rendered a bare "11" that the leaked-id check (correctly) flagged. Fixed in `ui/e2e/fixtures.ts`:
   moved that credential's `last_used_at` off the 11 o'clock hour and added a pass that nudges any
   of its messages whose `created_at` falls in hour 11 to hour 10. This is a fixture artifact of my
   own test data, not a product bug — recorded here per the verification skill's rule to flag that
   explicitly rather than silently drop it.
8. **Cosmetic, not fixed further:** at the narrow end of the three-column Overview layout (roughly
   1024–1366px), long dotted hostnames (`mail.example.com:587`) still wrap mid-segment inside the
   12rem-label `Facts` grid, because each column is only ~350–450px wide. I added break hints after
   `.`/`:`/`@`/`/` in `Mono` (`ui/src/components/Mono.tsx`, via `<wbr>`) and switched from
   `break-all` to `break-words` so the browser prefers those points over an arbitrary mid-character
   break; this measurably improved the wrap (confirmed by re-screenshotting) but a single
   dot-separated segment like `example` can still be wider than the column, in which case
   `break-words` still forces a break inside it. No overflow results (`scrollWidth <=
   innerWidth` holds, confirmed with a DOM probe before and after), so the hard gate passes; this is
   a genuine but minor legibility compromise at that one width band, left as a known limitation
   rather than redesigning the three-column grid.

## Manual pass over the pictures

Per viewport (phone 390, phone-small 360, tablet 820, tablet-landscape 1180, desktop 1366), opened
Overview, Messages, Message detail, Audit (expanded row) and Login detail in both light and dark
and checked against `DESIGN.md`:

- Nothing is a card: every list is flat rows split by hairlines; dialogs/sheets are flat with a
  hairline border, no shadows anywhere.
- The accent red appears only on the primary button ("Test connection", "Renew now", destructive
  actions), the active nav mark (rail left border / tab top border), and text links. Result colours
  (green/amber/red) appear only on the three counts, the hour chart and status marks.
- 16px gutter held on phone; no prose wider than the `max-w-prose` column; long values (the 140-char
  subject, the 12-recipient list, the 64-char address) wrap or truncate without pushing layout,
  confirmed on Messages and the message-number-30/35 fixtures.
- Phone/phone-small: the bottom tab bar sits below the last content row, confirmed by the
  `pb-24`/`pb-[env(safe-area-inset-bottom)]` padding and by the "Show older"/footer text being
  fully visible in the shots (the tab bar's apparent mid-page position in the *full-page* capture
  is the known Playwright artifact — a `fixed`/`sticky` element is frozen at its original viewport
  offset when the page is scrolled and stitched; a viewport-only capture after scrolling to the
  bottom, spot-checked separately, shows it correctly pinned with no overlap).
- Dark theme: background is `#18130e` (not pure black), text `#ede7df` (not pure white), hairlines
  `#3b342e` clearly visible against both `sheet` and `sunken`, the secret-reveal box and raw-header
  `<pre>` block both legible on `sunken`.
- Tablet portrait (820) shows the bottom tab bar (not the rail) and widens `Facts` to the two-column
  (label-beside-value) layout; tablet-landscape (1180) and desktop (1366) both show the 232px left
  rail with all six items and the signed-in name / theme switch / sign-out at the foot.

No problem was found in this manual pass beyond the two already recorded above (the Overview
three-column `Mono` wrapping, and the known full-page-screenshot fixed-element artifact, which is
not a product defect).
