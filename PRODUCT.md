# PRODUCT.md — SMTP Gateway panel

**What it is:** an SMTP relay that gives each app or device its own login and forwards mail through one upstream account. The panel manages those logins and shows what was relayed and who changed what.

**Who uses it:** one or two administrators of a small self-hosted setup. They open it on a phone or tablet when something does not send, and on a laptop to add a login. They are technical but not SMTP experts.

**Top jobs, in order:**

1. "Did that message go out, and if not, why?"
2. "Create a login for a new app and copy its settings."
3. "Turn off or replace a login that leaked."
4. "Who changed this, when, from where?"
5. "Give a script read access to the logs."

**Register:** product UI. Quiet, legible, dense enough to scan a log, never decorative.

**Voice:** plain and direct. Says what happened and what to do next. No exclamation marks, no "Oops", no "successfully".

**Never:** show internal ids; hide a failure behind a toast that disappears; ask for confirmation of harmless actions; use colour alone.
