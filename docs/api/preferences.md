# User preferences

[API reference](../api.md) · [Common conventions](common.md)

Authenticated routes for the single user's deployment-wide choices: the Format
Preference and the Default Library. They sync through the server, so every
device shares them. There is no ETag or `If-Match`: the last write wins, per
field.

| Method | Path | Contract |
| --- | --- | --- |
| GET | `/api/v1/preferences` | 200 `{"track_formats":["WAV","FLAC"],"default_library_id":"…"}`; `track_formats` is `[]` and `default_library_id` is absent when never saved. |
| PUT | `/api/v1/preferences` | A merge: 200 with the whole stored object after it. |

`track_formats` is the ordered Format Preference. A Work's effective format is
the first listed format it ships; a Work shipping none keeps every format. An
empty list means no preference. Labels are uppercase file extensions, the form
the [content manifest](works.md) aggregates by (`MP3`, `WAV`, …).

`default_library_id` is the Default Library: the Library a session with no place
of its own opens first. It names one Library, never `all`. It may name a
Library deleted since; readers treat that as none, and the server does not
rewrite it.

PUT accepts only `track_formats` and `default_library_id` (unknown keys return
`INVALID_ARGUMENT`, 400) and changes only the fields the body carries, so each
settings page writes its own field without overwriting the other. Each
`track_formats` entry is trimmed and uppercased, later duplicates are dropped
keeping order, and the list holds at most 32 entries. `default_library_id`
`null` clears the Default Library. An empty entry, an empty
`default_library_id` or a body carrying neither field returns
`INVALID_PREFERENCES` (400); a `default_library_id` naming no Library returns
`LIBRARY_NOT_FOUND` (404). Nothing is stored on refusal.

The payload is stored in the `common_settings` row `preferences`, which is not
a Task kind, so `/api/v1/tasks/{type}/settings` answers it as an unknown type.
