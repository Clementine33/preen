# Reusable policy slots and classifier tags

[API reference](../api.md) · [Common conventions](common.md)

These authenticated routes supply reusable values; edits never rewrite saved
operation drafts or revisions.

| Method | Path | Contract |
| --- | --- | --- |
| GET | `/api/v1/policy-slots` | 200 `{"slots":[{"slot":1,"name":"…","policy":null},…]}`; exactly three slots, initially unconfigured. |
| PUT | `/api/v1/policy-slots/{slot}` | Body `{"name":"…","policy":{…}}`; 200 slot object. Slot must be 1–3, trimmed name 1–120 characters, policy complete and valid. Errors: `INVALID_SLOT`, `INVALID_SLOT_NAME`, `INVALID_POLICY` (400). |
| GET | `/api/v1/classifier-tags` | 200 `{"default_tags":[…],"custom_tags":[{"id":1,"tag":"…","created_at":"…"}]}`. Defaults come from `prune.literal_tags` and are read-only here. |
| POST | `/api/v1/classifier-tags` | Body `{"tag":"…"}`; 201 custom tag object. Empty trimmed tags return `INVALID_TAG` (400). |
| DELETE | `/api/v1/classifier-tags/{id}` | 204; invalid IDs return `INVALID_ID` (400), missing tags `NOT_FOUND` (404). |

Applying a slot is a client edit that copies policy values into the operation
draft and uses the normal version-guarded save. There is no server-side live
slot reference, slot creation or slot deletion endpoint.

Conversion policy and draft shapes are described in [Audio conversion](conversion.md#operation-draft-document).
