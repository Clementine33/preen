# Classifier tags

[API reference](../api.md) · [Common conventions](common.md)

These authenticated routes supply reusable values; edits never rewrite saved
operation drafts or revisions.

| Method | Path | Contract |
| --- | --- | --- |
| GET | `/api/v1/classifier-tags` | 200 `{"default_no_effect_tags":[…],"default_other_tags":[…],"custom_tags":[{"id":1,"tag":"…","created_at":"…"}]}`. Defaults come from `prune.no_effect_tags` and `prune.other_tags` and are read-only here. |
| POST | `/api/v1/classifier-tags` | Body `{"tag":"…"}`; 201 custom tag object. Empty trimmed tags return `INVALID_TAG` (400). |
| DELETE | `/api/v1/classifier-tags/{id}` | 204; invalid IDs return `INVALID_ID` (400), missing tags `NOT_FOUND` (404). |

Conversion policy and draft shapes are described in [Audio conversion](conversion.md#operation-draft-document).

The default groups are separate; custom tags share one reusable string library.
The old response key `default_tags` is removed. Configuration uses
`prune.no_effect_tags` and `prune.other_tags`. The shipped template places
`反転` in `other_tags`; existing user tags are not reassigned automatically.
