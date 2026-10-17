# User preferences

[API reference](../api.md) · [Common conventions](common.md)

Authenticated routes for the single user's deployment-wide display choices.
They sync through the server, so every device shares them. There is no ETag or
`If-Match`: the last write wins.

| Method | Path | Contract |
| --- | --- | --- |
| GET | `/api/v1/preferences` | 200 `{"track_formats":["WAV","FLAC"]}`; `[]` when never saved. |
| PUT | `/api/v1/preferences` | Same body; 200 with the normalized object. |

`track_formats` is the ordered Format Preference. A Work's effective format is
the first listed format it ships; a Work shipping none keeps every format. An
empty list means no preference. Labels are uppercase file extensions, the form
the [content manifest](works.md) aggregates by (`MP3`, `WAV`, …).

PUT strictly accepts only `track_formats` (unknown keys return
`INVALID_ARGUMENT`, 400). Each entry is trimmed and uppercased, later duplicates
are dropped keeping order, and the list holds at most 32 entries. A missing
`track_formats` or an empty entry returns `INVALID_PREFERENCES` (400); nothing
is stored on refusal.

The payload is stored in the `common_settings` row `preferences`, which is not
a Task kind, so `/api/v1/tasks/{type}/settings` answers it as an unknown type.
