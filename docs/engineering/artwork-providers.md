# Artwork providers

Media Core discovers covers only when the operator explicitly asks for them:
Quick Add cover search or the Library action **Find missing covers**. No page
render opens an outbound connection. Saved libraries render from SQLite and
the local image cache, with a neutral placeholder when a cover is missing.

Research date: 2026-11-24. Each endpoint below was checked with a live,
anonymous, read-only request. Terms and limits can change; a provider that
starts failing is isolated and never blocks the primary task.

## Verified providers

| Provider | Formats | API and docs | Live check | Limits and conditions | Attribution |
| --- | --- | --- | --- | --- | --- |
| Kitsu | anime, manga | `https://kitsu.io/api/edge/anime` and `/manga`, JSON:API, [docs](https://hummingbird-me.github.io/api-docs/) | `filter[text]`, `page[limit]` returned titles and `posterImage.medium` at `media.kitsu.app` | No published fixed rate limit; public GET needs no token; 429 is possible. Client uses one request/second, no background polling. | Link to the Kitsu work URL stored as `source_url`; provider shown in the picker and selection. |
| Shikimori | anime, manga | GraphQL at `https://shikimori.one/api/graphql`, [docs](https://shikimori.one/api/doc) | Introspection, an `animes(search: …)` query and the `mangas(search: …)` field return ids, names, `kind`, URLs and poster URLs | 5 requests/second and 90/minute; descriptive `User-Agent` required, no browser mimic. New posters are only in GraphQL, so REST v1/v2 is not used. | `source_url` is the Shikimori work page; provider shown in the picker. |
| MangaDex | manga | `https://api.mangadex.org/manga?includes[]=cover_art`, [docs](https://api.mangadex.org/docs/) | Search returned manga ids and `cover_art` relationships with `fileName` | ~5 requests/second per IP; TLS and non-spoofed `User-Agent` required; mandatory credit, no ads/paid access, and image requests must be proxied rather than hotlinked. Media Core credits MangaDex and proxies every image through the server-side allowlist. | Work URL stored as `source_url`; MangaDex is named in the picker and in this document. |
| Open Library | books | `https://openlibrary.org/search.json`, [docs](https://openlibrary.org/developers/api), [covers](https://openlibrary.org/dev/docs/api/covers) | Search returned work keys and `cover_i` values | 1 request/second unidentified, 3 identified with `User-Agent` and contact; no HTML scraping, no crawling covers, no bulk backend use. Client identifies itself and caches selected images locally. | Courtesy link to the Open Library work page stored as `source_url` and shown in the picker. |

## Optional, not enabled

- **TMDB** (`https://api.themoviedb.org/3`, [terms](https://www.themoviedb.org/api-terms-of-use)) requires an API key, a visible attribution notice and logo, and forbids caching content longer than six months. It remains a documented extension for movie/TV works; it is not wired without an operator-provided key and an explicit decision.
- Other sources require a fresh terms/rate-limit review and a new ADR before an adapter is added.

## Explicitly excluded

- **AniList** is not used: its rules restrict use by competing trackers.
- The former public **Jikan** endpoint is not used; no unofficial wrapper around a site's HTML is acceptable.
- No HTML scraping, no Google Images, no generic web search and no inference of cover data from arbitrary URLs.
- The server never fetches a URL supplied by a user. Only provider responses can name an image host, and only the exact allowlisted hosts above are accepted over HTTPS with bounded redirects, timeouts, bytes, dimensions and MIME types.

## What is stored

Each selection records provider, external id, source URL, attribution URL,
media kind, explicit user-selection flag, provenance and a local cache
reference in `work_artwork` (migration 0014). Image bytes live in
`<state-dir>/artwork-cache/` with a hard 64 MiB total cap, including every
protected selection, and a 2 MiB per-image cap. Unreferenced files are
evicted first; when protected selections plus a new image would exceed the
cap, the selection fails with a stable `cache_full` error instead of
deleting another cover. Selected bytes travel in v2 backup archives (see
[backup and restore](backup-restore.md)). `DELETE /api/v1/artwork/cache/clear`
removes bytes without touching selections; a missing file renders the
neutral placeholder. Re-selecting a cover rebuilds the cache entry.

## Resources

One request per provider per second, at most five provider results, at most
three image previews per search, at most five works per explicit batch, and
no background workers. Provider searches and preview fetches run in bounded
request-scoped goroutines (at most three at once); find-missing works are
processed in keyset order and return partial results with a timeout status
when the per-request budget expires. The budget is below the server's 30
second WriteTimeout, and the browser cancels the in-flight request with an
AbortController. Only candidates with a local preview are returned, so every
listed candidate is selectable. There is no image resizing or re-encoding:
the server decodes only the image header for dimension and decompression-bomb
checks and writes the original bounded bytes. This stays within the existing
systemd CPU, memory and I/O limits.
