# Work collections

[API reference](../api.md) · [Common conventions](common.md)

A Work Collection is the single user's ordered set of complete Works, independent
of Libraries, Copies and inventory paths. A Work may belong to several collections,
but appears at most once in each. Losing a Copy, rescanning, moving files or deleting
a Library never removes membership. Selecting a Work uses the existing
[content manifest and file routes](works.md#work-content-manifest); there is no
collection playback endpoint or cross-work sequencing.

## Routes

All routes require bearer authentication. Work numbers use the catalog's
six/eight-digit, case-insensitive normalization and return canonical eight-digit RJs.

| Method | Path | Contract |
| --- | --- | --- |
| GET | `/api/v1/collections` | 200 `{"collections":[…],"total":N}`: one page. |
| POST | `/api/v1/collections` | Body `{"name":"Sleep"}`; 201 the new empty collection. |
| GET | `/api/v1/collections/{id}` | 200 collection fields plus `works:[…]` and `total`: one page of work summaries in saved order, or reversed with `direction=desc`. |
| PATCH | `/api/v1/collections/{id}` | Body `{"name":"Rest"}`; 200 the renamed collection. |
| DELETE | `/api/v1/collections/{id}` | 204; deletes only the collection and its membership. |
| POST | `/api/v1/collections/{id}/works` | Body `{"rjs":["RJ00000002","RJ00000001"]}`; 200 `{"added":[…],"skipped":[…],"work_count":N}`. |
| DELETE | `/api/v1/collections/{id}/works/{rj}` | 204; an already absent member of an existing collection succeeds. |
| PATCH | `/api/v1/collections/{id}/works/{rj}` | Body `{"before_rj":"RJ00000002"}` or `{"before_rj":null}`; 204 after moving before that member or to the end. |

Collection fields are `id`, `name`, `work_count`, `created_at` and `updated_at`.
User collections have UUID IDs. Names are trimmed and must be non-blank; duplicate
names are allowed. There is no separate name-length cap beyond the shared 1 MiB
JSON-body limit. Timestamps use UTC with nine fractional digits and are informational,
not concurrency tokens. Successful changes to membership or order update the timestamp.

Detail returns the [work listing's summary](works.md#listing-filters-ordering-and-paging)
fields, including local covers. Work information is current, not a second title or
cover snapshot. Counts include members without Copies; Copy count does not guarantee
that files are online or playable. Collections do not create Works or rebind Copies.

## Favorites

The built-in collection has ID `favorites` and name `Favorites`, exists on every
fresh database, and always precedes user collections in the list. Its initial
timestamps use the Unix epoch. It supports append, remove and move normally, but
cannot be renamed or deleted: those operations return 409 / `BUILTIN_COLLECTION`.

Membership is the sole source of the catalog's `favorite` flag:

- Collection member edits change summaries, detail and `favorite=true|false` filters.
- `PATCH /works/{rj}/user-data` with `favorite:true` appends if absent; an existing
  favorite keeps its position. `favorite:false` removes it. Omitting the field
  preserves membership. Submitted rating, note and favorite changes remain atomic.
- Release upgrades preserve the old favorite set, ordered by former user-data
  update timestamp then Work Number. The former favorite column is removed.
- Membership changes notify existing `works_changed` subscribers because work flags
  and filters change. No new collection SSE protocol is introduced.

## Paging

Both GET routes accept `limit` and `page`, following the catalog conventions; the detail
route also accepts `direction` (below). Both reject any other key:
absent or empty values use `limit=50`, `page=1`; nonempty limits must be integers
1–200 and pages integers of at least 1. Invalid values return 400 / `INVALID_PAGE`;
unknown query keys return 400 / `INVALID_ARGUMENT`.

Lists order Favorites first, then user collections by creation timestamp and ID.
Details follow saved member order. `direction=desc` on a detail reverses that order, so
the first page holds the most recently appended members (the end of saved order, which
a move also changes); `asc` or absent keeps saved order. Any other value is 400 /
`INVALID_ARGUMENT`, and the collection list refuses `direction` as an unknown key.
`limit` and `page` window the reversed order. Past-end pages contain an empty array and the
current total. The total and window are read together, but separate requests share
no snapshot: an edit can move items across page boundaries. Clients re-read after edits.

## Atomic append and incremental move

Append accepts 1–200 RJs before normalization and deduplication. Request duplicates
collapse to their first occurrence. Existing members appear in `skipped` without
moving; new members append in request order and appear in `added`. Both arrays
contain canonical RJs, and empty results are arrays rather than null. `work_count`
is the whole collection size after the transaction.

All Works are validated before any membership is written:

- An invalid RJ, absent/empty array or more than 200 entries is 400 / `INVALID_ARGUMENT`.
- A well-formed RJ without a Work is 404 / `WORK_NOT_FOUND`.
- A Relation Placeholder never found by a scan is 409 / `WORK_IS_PLACEHOLDER`.
  A Work found by a scan remains eligible after all Copies disappear.
- `details[]` lists every missing or placeholder canonical RJ in request order.
  A mixed batch returns `WORK_NOT_FOUND`. Any refusal writes nothing.

Moves submit an intent, not positions or the whole list. `before_rj` must be present:
omitting it is 400, explicit null means the end. Other non-string values and invalid
RJs are also 400. Moving before itself or to its current place succeeds with unchanged
order. A target without playable Copies remains valid.

Writes apply to current membership inside one protected write transaction. A move
preserves concurrent additions and never re-adds removed members. If its member or
target has been removed, it returns 409 / `COLLECTION_MEMBER_CONFLICT`, writes nothing,
and the client refreshes before retrying. Multiple moves apply in transaction execution
order, not necessarily HTTP arrival order. There is no list replacement endpoint,
expected version, schema version, fractional index or persistent idempotency key.

## Missing resources and deletion protection

An unknown collection is 404 / `COLLECTION_NOT_FOUND`, including member removal.
Repeating member removal in an existing collection is a no-op; repeating deletion
of the collection itself is a 404.

Membership blocks permanent Work deletion with 409 / `WORK_IN_USE`. Each referencing
collection adds a `details[]` string `a collection holds this work: <id> <name>`,
ordered by collection ID. Existing Copy and relationship-evidence blockers remain.
The ID is stable; the name is arbitrary user text. Remove memberships before deleting
a Work, including Favorites membership.

Deletion checks and member writes share transaction guarantees, with foreign keys
as the final protection against dangling members. Deleting a collection leaves
Works, their personal data, other collections, Copies and media files untouched.
