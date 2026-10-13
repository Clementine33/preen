# Work catalog

[API reference](../api.md) · [Common conventions](common.md)

The catalog is the logical layer above the inventory: works identified by their
work number, the source metadata imported for them, the directories bound to
them, and the single user's data about them. The directory name is the only
authority on which work a directory holds, so there is no route that creates a
work or rebinds a copy — scanning does that (ADR 0009). Every work number in
a path or body is accepted in either case and in its 6- or 8-digit spelling, and
resolves to the canonical 8-digit form; `RJ123456` and `RJ00123456` are the same
work, while `RJ1657200` (7 digits) is refused.

| Method | Path | Contract |
| --- | --- | --- |
| GET | `/api/v1/works` | 200 `{"works":[…],"total":N}`: one numbered page and how many works the whole filter lists. Filters and paging below. |
| GET | `/api/v1/works/{rj}` | 200 the work in full: metadata, personal data, resolved relation, language editions and every copy. A work declared by a relation but never found on disk answers `"placeholder":true` and is absent from the listing. |
| GET | `/api/v1/inventory/media-facts/events` | Bearer-authenticated SSE; `changed` with `{}` after a committed measurement batch, and once on connection. See below. |
| GET | `/api/v1/works/{rj}/content` | 200 the work's file manifest (ADR 0013): every bound copy's tree, below. |
| GET | `/api/v1/works/{rj}/content/{copyId}/{path...}` | 200 one manifest file's bytes (ADR 0013): `path` is the copy-root-relative manifest path. See below. |
| GET | `/api/v1/works/{rj}/thumbs/{copyId}/{path...}?h=240&v=…` | 200 one manifest image's thumbnail (ADR 0013). See below. |
| PUT | `/api/v1/works/{rj}/metadata` | Body: up to four independent data groups (below). 200 `{"applied":[…],"ignored":[…],"warnings":[…],"created_placeholders":[…]}`. |
| PATCH | `/api/v1/works/{rj}/user-data` | Body `{"rating":8,"favorite":true,"note":"…"}`; only the fields present are applied, and `"rating":null` clears it. 200 the resulting state. |
| PUT | `/api/v1/works/{rj}/user-tags` | Body `{"tag_ids":["…"]}`; replaces the work's tag set with exactly these. 200 `{"user_tags":[…]}`. Unknown ids are `USER_TAG_NOT_FOUND` (404). |
| GET | `/api/v1/circles` | 200 `{"circles":[{"id":"…","name":"…","work_count":3}]}`: every circle a listed work carries, ordered by name. Takes the listing's filter parameters (below) and counts only the works they list, so `library` (comma-separated ids) scopes the circles and their counts to works with a copy there, and any other filter narrows them the same way; a caller offering alternatives to its own pick leaves that dimension out. `sort`, `direction`, `limit`, `page` and any unknown parameter are `INVALID_ARGUMENT` (400). |
| GET | `/api/v1/voice-actors` | 200 `{"voice_actors":[…]}`, the same shape and scoping for voice actors. Each `id` is what the listing's `circle` / `voice_actor` parameter takes. |
| GET | `/api/v1/tags` | 200 `{"source_tags":[…],"user_tags":[…]}`: the same shape and scoping for both tag vocabularies at once (the wall's tag index). A `source_tags` id is an `s:` ref of the listing's `tag` parameter (and a `source_tag` id), a `user_tags` id a `u:` ref. A user tag no listed work carries is absent here; `GET /user-tags` is the whole vocabulary. |
| GET | `/api/v1/user-tags` | 200 `{"user_tags":[{"id":"…","name":"…"}]}`. |
| POST | `/api/v1/user-tags` | Body `{"name":"…"}`; 200 the tag, **creating it if new and returning the existing one otherwise** — creating by name is idempotent. |
| PATCH | `/api/v1/user-tags/{id}` | Body `{"name":"…"}`; 200 the tag, `USER_TAG_EXISTS` (409) on a name another tag holds. |
| DELETE | `/api/v1/user-tags/{id}` | 204; the tag's relations go with it, no work is touched. |
| DELETE | `/api/v1/works/{rj}` | 204; `WORK_IN_USE` (409) with `details[]` while a copy is bound or another work's evidence points at it. |

Ratings are integers 1–10 (`null` means unrated, which is not 0). A work that
exists only as a relation placeholder holds no personal data: writing it is
`WORK_IS_PLACEHOLDER` (409). User tag names are trimmed, must be non-empty, and
are unique as exact text — case is not folded and source tags are a separate
thing.

The `favorite` flag is membership in the built-in
[Favorites collection](collections.md#favorites), shared by user-data writes,
work summaries, detail and the listing filter. Setting it appends the Work if
absent without moving existing favorites; clearing it removes membership.
Ratings and notes retain their existing behavior. Favorites and other collection
memberships now block permanent Work deletion with `WORK_IN_USE`; `details[]`
names each referencing collection. Removing a Copy or Library preserves membership.

## Work content manifest

`GET /api/v1/works/{rj}/content` answers the work's files from the **stored
inventory** — never a live walk — as one tree per bound copy (ADR 0013):

```json
{
  "rj": "RJ01000001",
  "copies": [
    { "copy_id": "…", "library_id": "…", "dir_id": "…", "online": true,
      "tree": { "name": "RJ01000001", "rel_path": "", "children": [
        { "name": "表紙", "rel_path": "表紙", "children": [
          { "name": "cover_main.jpg", "rel_path": "表紙/cover_main.jpg",
            "kind": "img", "size": 4096, "format": null, "bitrate": null,
            "width": 1200, "height": 900, "duration_sec": null,
            "thumb_v": "9f86d081884c7d65" } ] },
        { "name": "01_導入.mp3", "rel_path": "01_導入.mp3",
          "kind": "aud", "size": 5120, "format": "MP3", "bitrate": 320,
          "width": null, "height": null, "duration_sec": 185.5,
          "thumb_v": null } ] } }
  ]
}
```

`dir_id` is the copy root's navigation identity (ADR 0003), resolved
server-side. A copy whose member directory no longer resolves on disk answers
`"online": false` with no `tree` — stale is worse than absent. A placeholder
work answers an empty `copies` array, not 404. `kind` classifies each file
(`aud`, `img`, `txt`, `lrc`, `bin`); directories carry `children` (empty
included) and files carry no `children`, always carry `size`, and carry
`format` only for audio, as the container name the track list aggregates by.
`width`/`height`/`duration_sec`/`bitrate` are the inventory's facts (ADR 0013): `null` means unmeasured, not zero. Dimensions and durations are measured
in the background after a scan completes, so a freshly scanned work can answer
`null` for a while. `thumb_v` is an image's opaque
thumbnail version (`null` for every other kind), the `v` its thumbnail address
carries.

`GET /api/v1/works/{rj}/content/{copyId}/{path...}` serves one manifest
file's bytes. The route is read-only and takes no admission slot: only paths
the manifest holds are served, resolved inside the member under the same
containment checks [direct file management](libraries.md#direct-file-management) applies (out-of-bounds segments and
symlink traversal is refused; an unscanned file on disk is 404, not a guess).
Directories are refused, and the opened object must still be a regular file.

The `lrc` kind includes timed lyrics with `.lrc` or `.vtt` extensions
(case-insensitive). The detail page pairs these by audio basename, preferring
`.vtt` and falling back to `.lrc`. A manual pairing takes precedence. It reads
their bytes through the content route and follows the playing track's audio position. WebVTT cue end times
clear the highlight during gaps; clicking a line seeks only its matching track.

- Besides the `Authorization` header, this route (and only this one) accepts
  the token as `?access_token=…`, so an `<img>` or `<audio>` element can load
  it by address. Only a completely absent Authorization header permits this
  fallback: malformed, empty or incorrect header credentials are refused.
- `ETag` keys on `(size, mtime, content_rev)`; `Cache-Control: no-cache`
  revalidates before reuse, so a rescan that changed the bytes changes the
  tag and a match answers 304. Original image addresses may carry optional
  `v=thumb_v` to reload a changed path; this does not change file cache semantics.
- `Accept-Ranges: bytes` with single-range 206 (`Range: bytes=0-99`). A
  multi-range request is answered with the whole file; an unsatisfiable range
  answers 416 with `Content-Range: bytes */<size>`.
- Images carry the type recognized from their bytes; unrecognized image
  bytes use `application/octet-stream`, regardless of extension. Audio can
  fall back to the container extension when byte sniffing is inconclusive.
  Text (`txt`/`lrc`) declares the character set the server
  sniffed (`UTF-8` → `Shift_JIS` → `GB18030` candidates) and the body is
  served exactly as stored — decoding happens once, at the client, per the
  header, using `arrayBuffer()` and `TextDecoder` (not `Response.text()`).
  A multibyte character cut off at the sample boundary is not an invalid
  encoding; truncated characters at actual EOF remain invalid.
  WebVTT files use `text/vtt`; other text uses `text/plain`.

`GET /api/v1/works/{rj}/thumbs/{copyId}/{path...}?h=…&v=…` serves one
manifest image scaled to a height (ADR 0013). The source goes through the
same copy binding, containment and whitelist as the file route; a non-image
path is `NOT_AN_IMAGE` (404). Read-only, no admission slot.

- `h` (required, positive) snaps up to a served height: 120, 240, 480 or 960.
  A source shorter than that is never upscaled.
- `v` is the image's `thumb_v` from the manifest, a digest of the file's size,
  mtime, `content_rev` and the engine version. Successful responses and their
  matching 304 at the current version have
  `Cache-Control: private, max-age=31536000, immutable`; any other
  `v` (or none) still answers, but `no-cache`, so a stale address never pins
  old bytes. `ETag` is `"<v>-<height>"`; a match answers 304.
- Opaque sources answer `image/jpeg`, sources with transparency `image/png`.
- `BUSY` (503) with `Retry-After` when the render queue is full — back off and
  retry. `IMAGE_TOO_LARGE` (422) over the pixel limit (about 100M pixels,
  checked from the header), `IMAGE_UNDECODABLE` (422) for bytes that do not
  decode; the page draws its hatched placeholder for both. Errors (including
  500) use `Cache-Control: no-store` and carry no successful-content ETag.
  Allowed cross-origin responses expose `Retry-After` to the client.

## Background media facts notifications

`GET /api/v1/inventory/media-facts/events` is a bearer-authenticated SSE
connection; query-token authentication is not accepted. It responds with
`Content-Type: text/event-stream` and `Cache-Control: no-cache`.

```text
event: changed
data: {}

```

A successful batch commit sends one notification; failed writes send none.
Each subscriber has only one pending notification, so slow clients coalesce
changes without blocking measurement. Every connection receives an initial
notification to cover changes while disconnected. A `: keep-alive` comment
is sent every 15 seconds; disconnection unregisters the subscriber. There are
no event IDs, replay history or persistent event fields.

The same connection also carries `event: works_changed` with `{}`: one is sent
on every connection, registered together with the subscription so no commit
between a reader's list and its connection is lost, and one follows each commit
that can change what the works pages show (metadata import, scan and inventory
merges, observed inventory such as a published cover, directory relocation, and
library removal or re-rooting). The root application refreshes work lists,
details, facets and manifests on it, debounced by 1 second with a 5 second
ceiling.

The root application maintains one connection, reconnecting after 1, 2, 4
seconds up to 30 seconds; 401 stops retries, and unmount cancels both the stream
and pending wait. Notifications cancel any older in-flight reads, refresh
active work-content and member-tree queries, and remove inactive caches.
Scanning and file/library mutations also refresh or remove work lists, details
and manifests. Rebinding a manifest preserves reading, lyric pairing and
playback. Thumbnail blobs are keyed by copy, path and `thumb_v`; changed
versions revoke the old blob and reload both thumbnail and original image.

## Source metadata import

The body carries up to four **independent** groups; a group that is absent is
left exactly as it was, and a group that is present is a complete snapshot of
that group — an unknown scalar is an explicit `null`, and an empty array means
the set is empty. A submission with no group at all is refused.

```json
{
  "core": {
    "source": "dlsite",
    "fetched_at": "2026-09-27T10:00:00.000000000Z",
    "title": "…",
    "language": "NM",
    "circle": { "source_circle_id": "RG01038998", "name": "mete0r" },
    "nsfw": true,
    "age_category": 3,
    "release_date": "2025-06-25",
    "source_url": "https://…",
    "cover_url": "…", "thumbnail_url": "…", "sample_cover_url": "…",
    "series_id": null, "series_name": null,
    "voice_actors": [{ "source_voice_actor_id": "17180", "name": "伊ヶ崎綾香" }],
    "tags": [{ "source_tag_id": "497", "name": "ASMR" }]
  },
  "statistics": { "fetched_at": "2026-09-27T10:00:00.000000000Z", "dl_count": 12 },
  "relations": {
    "source": "dlsite",
    "fetched_at": "2026-09-27T10:00:00.000000000Z",
    "role": "child",
    "language": "CHI_HANT",
    "original_rj": "RJ01276487",
    "parent_rj": "RJ01412616",
    "child_rjs": []
  },
  "editions": {
    "source": "dlsite",
    "fetched_at": "2026-09-27T10:00:00.000000000Z",
    "edition_id": "51904",
    "items": [{ "version_root_rj": "RJ01506610", "language": "JPN", "label": "日本語", "display_order": 1 }]
  }
}
```

Work detail presents both `original_rj` and `parent_rj` when available; they are different hops, not alternative labels. On the library wall, Enter in FIND opens an exact in-library number first, otherwise the sole in-library Chinese translation; multiple candidates remain on the wall. Typing alone does not navigate.

`relations` and `editions` are separate groups on purpose: `translation_info`
is available on DLsite's dynamic endpoint, `language_editions` only on its
static one, so a crawler refreshing sales cheaply must not be forced to
resubmit — or worse, silently blank — the group it did not fetch
(ADR 0009). `statistics` carries no `source`: sales are only ever reported
by DLsite. `fetched_at` must be UTC with nine fractional digits; other shapes
are `INVALID_TIMESTAMP` (400).

Within a group, a higher-priority source (DLsite) replaces a lower-priority one
(ASMR.one) outright, and the same source only moves forward: an equal or older
fetch is skipped, reported in `ignored[]` with `STALE_FETCHED_AT` or
`LOWER_PRIORITY_SOURCE`. `warnings[]` is a different thing — those groups
*were* written, and carry `RELATION_CONFLICT` or `RELATION_AMBIGUOUS` when a
declaration disagrees with the work's own relation report (the work's own
report always wins). Anything structurally illegal — a self-reference, an
original chain, a cycle among effective parents, a malformed number — refuses
the **whole** submission as a 400 and writes nothing. An import never touches
personal data or copy bindings, and a work it names but the catalog has not
seen on disk is created as a relation placeholder, listed in
`created_placeholders[]`.

## Listing filters, ordering and paging

Each work in a list or detail response carries `local_cover`: `null`, or
`{"copy_id", "rel_path", "version"}` naming a `cover.{webp,jpg,jpeg,png,gif}`
the stored inventory holds in one of the work's copies (the first copy by
library and `rel_path`, then by name). Its bytes are served by the content route;
`version` is `<content_rev>-<mtime>-<size>`, so the address changes when the
inventory observes new bytes. Clients show it ahead of `thumbnail_url` and fall
back to `thumbnail_url` when the image fails to load.

`GET /api/v1/works` accepts exactly these parameters; anything else is
`INVALID_ARGUMENT` (400), so a typo cannot silently widen the answer.

| Parameter | Meaning |
| --- | --- |
| `q` | Literal substring of the work number or the title. `%` and `_` are ordinary characters, not wildcards. A query that *is* a work number matches that canonical number or Chinese translations whose effective parent is that number and which have a current copy; other filters, including library scope, still apply. The short spelling finds the same identities. Such a query does not search title substrings. |
| `circle`, `language`, `library` | Comma-separated id lists; a work matches if it has any of them. |
| `voice_actor` / `source_tag` / `user_tag` | Comma-separated id lists, each **requiring** its `…_match=all\|any` companion (`MATCH_MODE_REQUIRED`, 400, if absent). |
| `tag` | Both tag vocabularies in one filter: comma-separated **groups**, each group `\|`-joined refs `s:<source tag id>` or `u:<user tag id>`. A group is one picked tag and matches a work carrying **any** of its ids (a name spelled by several entities is one group); **requires** `tag_match=all\|any`, which combines the groups. A malformed ref is `INVALID_ARGUMENT` (400). Intersects every other filter, `source_tag` and `user_tag` included. |
| `nsfw` | Compatibility adult flag: `true`, `false` or `unknown` (not reported). `false` includes both all ages and R15. |
| `age_category` | Exact age rating: `1` (all ages), `2` (R15), `3` (R18); intersects other filters. |
| `released_from` / `released_to` | `YYYY-MM-DD`, inclusive; works with an unknown release date are not in the range. |
| `rating_from` / `rating_to` | Integer 1–10, inclusive. |
| `unrated` | `true`/`false`. It **unions** with the rating range inside that dimension: `rating_from=8&unrated=true` means "8 or more, or not rated". |
| `favorite`, `has_copy` | `true`/`false`. |
| `sort` / `direction` | `rj` / `release_date` / `collected_at` / `rating`, and `asc` / `desc`. |
| `limit`, `page` | Page size and 1-based page number (below). |

Different dimensions intersect. An absent `limit` is 50; a supplied one must be
an integer 1–200, so `limit=abc`, `0`, `-1`, `201` and `1.5` are all
`INVALID_PAGE` (400) rather than being replaced or clamped. An absent `page` is
the first; a supplied one must be an integer of at least 1, so `page=0`, `-1`,
`abc` and `1.5` are `INVALID_PAGE` too. A page past the end answers an empty
`works` with the same `total`, which is what a caller corrects against.

Pages are offsets into one total order: unknown release dates and unrated works
always sort last, in either direction, and equal sort values are broken by work
number, so consecutive pages of an unchanged catalog neither repeat nor skip a
row. There is no cross-request snapshot: a concurrent change can move a work
across a page boundary, and `total` is counted with each page.

The listing returns the work number, title, thumbnail, circle, language, adult
flag, exact age category, release date, **sales**, personal rating, favorite flag, user tags,
whether a note exists, when it was first collected, and its copy count. It does
not return the note text, the full edition list, relation details or the file
tree.

作品 Core、列表与详情的 `age_category` 使用 `1`（全年龄）、`2`（R15）、`3`（R18），
未报告时为 `null`。导入拒绝其他数字及与 `nsfw` 冲突的值；指定分级时自动同步兼容
`nsfw`（仅 R18 为 true）。只有旧 `nsfw=true` 可确认为 R18，`false` 不推断为全年龄。
DLsite 保存其原始分级，ASMR.one 仅提供布尔值时遵循同样规则。界面年龄筛选只有
三档和全部；未报告的年龄不显示标签，也不显示 UNKNOWN，仍可在全部中找到作品。
旧库升级保留所有作品和个人数据；非 R18 作品需全部刷新才能恢复准确年龄档位。

Metadata refresh and copy cover delivery are described in [Rename and metadata refresh tasks](tasks.md#metadata-refresh-task-schema-version-1).

作品 Core 的 `language` 声明当前 RJ 自身的语言，列表和详情顶层的 `language`
及列表 `language` 筛选优先使用它；为空时兼容回落到已解析的翻译关系语言。
`relations.language` 和 `editions.items[].language` 仍描述翻译关系与语言版，含义不变。
DLsite 从现有 product.json 的 `options` 按 `#` 分隔精确提取语言码，多语言优先
`CHI_HANS` → `CHI_HANT` → `JPN`，其后按 ENG、CHI、KO_KR、SPA、GER、FRE、IND、ITA、
POR、SWE、THA、VIE、RUS、OTL 的顺序选择；仅有 `NM` 时表示“无语言限制”。
`DOT` 是官方翻译标记，不作为语言；没有已识别的语言码保持 `null`，不能推断为 NM 或日语。
Core 导入仅接受上述语言码或 `null`，遵循完整快照和来源优先级规则。
界面语言筛选提供简中、繁中、日语、无语言限制及全部；详情显示当前语言。
旧库升级不猜测语言，需刷新元数据补齐原作的 JPN 与 NM；开发库按开发库重建流程更新结构。
