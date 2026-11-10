# Web UI design

First-class MVP interface: React + TypeScript + Vite, same-origin Go API, no production Node server. Lightweight feature components and typed fetch client, route state and local React state; no mandatory global state framework or large design system. Basic CSS, semantic HTML and accessible controls. API contract: [application-contract.md](application-contract.md).

## Screens and ownership

| Screen | Content and daily actions | Task |
| --- | --- | --- |
| Library | Responsive list/card toggle; title/alias search, medium, consumption, intention, tags, rating; sort and count; matching edition badges distinguish completed from saved (advanced intention/tag/rating filters added in 0035) | 0031, 0035 |
| Quick add | Title, optional medium/language/URL/provider ID; preview candidates; choose existing, new edition under work, or confirmed distinct; title-only manual fallback | 0032 |
| Work details / related media | Titles/aliases, editions with individual states, notes; explicit source/adaptation/translation rows with direction, provenance and confirmation state; navigate source; propose/confirm/revoke/origin forms | 0033 |
| Edition details | Kind/language/publication fact and source, title correction/archive, per-unit progress, consumption, historical observations/corrections | 0034 |
| Edition personal panels | Intention, tags, references, notes (including work-scoped note selection), rating history and corrections, add-source link | 0035 |
| Recent activity | Personal changes and imported publication events with distinct labels, observed/recorded time, pending import binding and release list | 0036, extended by 0043 |

Routes: `/`, `/add`, `/works/:id`, `/editions/:id`, `/activity`, `/login`. Related media is a readable list in work/edition details, not a separate graph application. Before advanced work-detail task 0033 lands, library cards navigate directly to edition details and quick add shows a compact work/edition result.

Library filter state lives in URL parameters for refresh/back; debounce search without changing semantics, cancel obsolete requests. Counts reflect current filters before pagination. Show missing metadata as “Unknown”/“No rating”, not fabricated covers or source facts. There is no cover fetch in MVP; a neutral placeholder can be omitted entirely. Search matches aliases and labels the match.

## Interactions

- Quick add is title-first; optional edition details are inline. A URL-only or ID-only entry asks for a manual title if no local identity exists, without pretending to fetch it. Duplicate candidates appear before committing. An exact reference opens the existing edition; conflicting fields require correction. Creation retries keep the request_id until a definitive result.
- Status/position are separate controls on the edition page and compact card where useful. Updating chapter 42 to 48 appends history. No implicit completed transition. Saved/planned is visibly distinct from in-progress/dropped.
- All forms keep user input on failure, show field errors, and handle stale revisions with reload/compare; never silently overwrite. Buttons are disabled while a request is pending; idempotency remains necessary for lost responses.
- A history drawer shows observed time and recorded time, old/current/superseded states, and explicit “Correct mistake” versus “Record new update”. No raw observation deletion button.
- Relation forms identify both editions and direction (“Anime is adaptation of manga”). Suggestions show “Proposed”; source claims show evidence. Confirm/revoke are deliberate actions with a reason for corrections.
- Release history and personal progress are separate. Binding an import selects a concrete edition and previews source identity. A release never marks a chapter read.

## Shared quality requirements

Every screen provides loading, empty, not-found, unauthorized/session-expired, retryable storage-lock and conflict states. Keyboard navigation, labels/focus and error associations are required; mobile at 360px must not overflow; desktop supports efficient list scanning. No screen requires a remote provider or LLM. Render notes as plain text, safe external links only. Responsive and error states are acceptance criteria in each UI task rather than a late cosmetic cleanup.

Session login and logout are in task 0030. Keep secret and CSRF out of persisted browser storage and build files; see [architecture security](architecture.md). The API client is the sole fetch wrapper and translates common errors. It does not reimplement domain decisions.
