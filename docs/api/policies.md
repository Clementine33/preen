# Classifier tags

[API reference](../api.md) · [JSON schemas and endpoint/event bindings](schema/contracts.json) · [Common conventions](common.md)

These authenticated routes supply reusable values; edits never rewrite saved
operation drafts or revisions.

| Method | Path | Schema | Contract |
| --- | --- | --- | --- |
| GET | `/api/v1/classifier-tags` | [classifierTagLibraryResponse](schema/classifierTagLibraryResponse.json) | Defaults come from `prune.no_effect_tags` and `prune.other_tags` and are read-only here. |
| POST | `/api/v1/classifier-tags` | Req: [classifierTagCreateRequest](schema/classifierTagCreateRequest.json) · Res: [classifierCustomTagItem](schema/classifierCustomTagItem.json) | 201 custom tag object. Empty trimmed tags return `INVALID_TAG` (400). |
| DELETE | `/api/v1/classifier-tags/{id}` | — | 204; invalid IDs return `INVALID_ID` (400), missing tags `NOT_FOUND` (404). |

Conversion policy and draft shapes are described in [Audio conversion](conversion.md#operation-draft-document).

The default groups are separate; custom tags share one reusable string library.
Configuration uses `prune.no_effect_tags` and `prune.other_tags`; the shipped
template places `反転` in `other_tags`, and existing user tags are not
reassigned automatically.
