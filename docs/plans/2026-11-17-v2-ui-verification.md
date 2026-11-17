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

## Update 2026-11-17 (review fix-up): result-colour discipline, formatSize, AuditFilter, new shots

Scope: fixing four review findings on top of the run above — result colours appearing on
non-result Alerts, a `formatSize` rounding bug at the KB/MB boundary, a missing `AuditFilter`
field, and five screenshot states that existed in the app but were never shot. No change to the
decisions already recorded above; this section only adds to them.

### Result-colour discipline

`theme.ts` maps MUI's `success`/`warning`/`error` palette slots to the `sent`/`rejected`/`failed`
design tokens, so any `<Alert severity="success"|"warning"|"error">` picks up a result colour even
when it is not showing a delivery result. Nine call sites did this: `LoginDetail.tsx` ("Saved."),
`SecretReveal.tsx` (the once-only warning), `Shell.tsx` (sign-out error), `LoginForm.tsx` (the
non-field form error), `ConfirmDialog.tsx` (a failed destructive action), `SignedOut.tsx` (OIDC
error), and three in `Overview.tsx` (the upstream connectivity test result/error, its network
error, the certificate `last_error`) plus a fourth or-of-kind one not flagged by the review but
in the same file (the ACME "Renewal started" `severity="info"` note, swapped for consistency).

**Decision — what counts as "a result":** DESIGN.md's rule is narrow: "the three result colours
appear only on a result (**a status mark, a chart bar, a count**)". None of the nine sites above
render a status mark, a chart bar or a count — they are all `Alert` boxes with a sentence in them.
The one borderline case, called out explicitly in the task, is the Overview upstream-test Alert:
tempting to read as "a delivery-ish result" because it reports ok/error for an action, but it is
text in a box, not one of the three named shapes, and a failed *connectivity test* is not the same
thing as a *failed message*. Decided: none of the nine qualify, all nine move to a new shared
`components/Notice.tsx` (ink text on `sunken`, 1px `hairline` border, an icon + words, no colour
keyed to severity — not even red for errors, since DESIGN.md reserves `failed` for a delivery
result and for destructive-action buttons specifically, both already handled elsewhere: the
confirm button in a destructive `ConfirmDialog` is already `color="error"`). `MessageDetail.tsx`
keeps its `Alert severity={failed|rejected}` unmodified: that one **is** showing the colour of the
message's own delivery result, which the review correctly left out of the finding list. This
distinction is now spelled out in DESIGN.md's colour paragraph and guarded by
`src/test/resultColours.test.ts`, a vitest that greps every non-test `.ts(x)` file under `src/`
(skipping `MessageDetail.tsx`) for `severity="success"` or `severity="warning"` and fails if either
appears. `severity="error"` isn't grepped for, since after this fix the only remaining
`severity={...}` in the whole app is `MessageDetail.tsx`'s own (verified: `grep -rn "severity=" src`
returns exactly that one file).

### formatSize boundary

`formatSize` rounded kilobytes with `Math.round`, which can round up to `1024` (e.g. 1,048,575
bytes → 1023.999… KB → rounds to 1024), then returned `"1024 KB"` instead of carrying into the MB
branch below it. Fixed by checking the rounded KB string against 1024 and falling through to the
existing MB formatting when it would read 1024. New cases added to `format.test.ts`: 1023 → "1023
B", 1024 → "1.0 KB", 1,047,552 → "1023 KB", 1,048,575 → "1.0 MB" (the bug case), 1,048,576 → "1.0
MB" (consistent with the fixed boundary).

### AuditFilter.target_type

Added the optional `target_type` field to `AuditFilter` (plan section 2.3, `GET /audit` query
params) and to `Audit.tsx`'s `apiFilter` object, which previously built its query object by
explicitly naming each known filter key and would have silently dropped `target_type` even once
present in the raw URL-backed filter. `src/api/hooks.test.tsx` (new) exercises `useAudit` directly
with `{ target_type: 'credential' }` and asserts the fetched URL contains
`target_type=credential`, and a second case asserts it is omitted when unset. No filter UI control
was added for `target_type` — the review asked only for the type and the query builder, and there
is currently no in-app control or link that sets it (a future "view this login's audit" link could
use it without further plumbing).

### New screenshot states

Added `e2e/shots.spec.ts` → `notice and error states`: five interaction-gated states, shot at
phone (390) and desktop (1366), both themes (20 new shots): `login-saved` (edit a login, save,
see the neutral "Saved." notice), `secret-reveal-new` (create a login, see the once-only secret),
`secret-reveal-rotate` (replace a login's password through the confirm dialog), `form-validation-
error` (a non-field 409 from `POST /credentials`, shown as the shared `Notice`, not a per-field
helper text), and `page-error-state` (a 500 from `GET /credentials`, the plain `Failure` component
— "Could not load this." + "Try again" — which intentionally renders no `PageHeader`/`h1`, so this
one test runs its own light checks — overflow, axe — instead of the shared `checkPage` helper that
asserts exactly one `h1`). `shots-out` now has **204** files (184 from the run above + 20 new).

**Bug found and fixed while writing these:** the first version nested all five tests' `for`
loops directly inside one `test.describe('notice and error states')` and called `test.use({
viewport })` once per loop iteration at that single describe's scope. Playwright does not
re-scope `test.use` per loop iteration that way — the last call in the block wins for *every* test
registered in that describe, so every "phone" shot silently rendered at the last-set viewport
(desktop, 1366×900) instead of 390 (caught by checking PNG `IHDR` width/height with a one-line
Python script before the manual visual pass, since a phone screenshot with a desktop left rail
is an easy thing to miss at a glance). Fixed by giving each (viewport, theme) pair its own nested
`test.describe`, exactly matching the pattern the existing suite already uses for the main route
matrix — re-ran afterwards and confirmed `login-saved-phone-light.png` is 390px wide and
`secret-reveal-rotate-desktop-light.png` is 1366px wide.

### Manual pass over the ten new pictures (five states × phone + desktop, both themes checked)

- `login-saved`: the "Saved." notice sits as a flat `sunken` box with a hairline border, a check
  icon and the word — ink-coloured in both themes, not green. The real result colours below it
  (the "Sent" status marks in Recent messages) are untouched.
- `secret-reveal-new` / `secret-reveal-rotate`: the once-only warning is the same neutral
  treatment — a warning-triangle icon, no amber. Phone width (390) shows the bottom tab bar; the
  secret value and copy button are not truncated or wrapped awkwardly at either width.
- `form-validation-error`: the conflict message renders in the shared `Notice`, not under a field
  helper text (confirming the no-`details.field` path), with a grey error-circle icon, not red, in
  both themes; "Create login" is still the accent-red primary button (not `color="error"` — that is
  just the brand stamp-red used for every primary action), unrelated to this fix.
- `page-error-state`: "Could not load this." renders with no page header, plain ink text, a
  "Try again" outlined button in the default accent-red used by every primary/outlined action
  (not a destructive signal — it is just the normal button colour), no result-colour styling on
  the message itself. Phone and desktop both show the known `#main`
  programmatic-focus outline along the content column's edge (Shell.tsx focuses `#main` on every
  route change for screen-reader users) — confirmed present on baseline shots too (e.g.
  `overview-desktop-light.png` shows the same thin blue rule along `main`'s left edge), so this is
  pre-existing app behaviour, not a regression or a shot artifact.

No problem found in this second pass beyond the viewport-scoping bug already recorded and fixed
above.

### Gate status (re-run after all four fixes)

`bun run typecheck`, `bun run lint` (`--max-warnings 0`), `bun run test` (**157 passed**, 36 files,
including the two new test files `Notice.test.tsx` and `src/test/resultColours.test.ts` and the new
`src/api/hooks.test.tsx`), `bun run build`, and `bun run shots` (**204 passed**, 4.1m) are all
green.
