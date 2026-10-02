# Audio conversion

[API reference](../api.md) · [Common conventions](common.md)

## Operation draft document

The draft is one sparse document. The four common setting groups are the base;
a member record overrides only the units it explicitly replaces. The
operation-wide `delete_mode` (obsolete-audio handling: `soft` — the default —
or `hard`) is a common-level field, never a member override, and freezes into
every revision the draft produces; the execution uses the frozen value.

```json
{
  "schema_version": 1,
  "mode": "available_sources",
  "delete_mode": "soft",
  "classifier_tags": ["SEなし"],
  "matched":   {"lossless": {"codec": "wav"}, "encoded": {"codec": "mp3", "quality": {"kind": "bitrate", "bitrate": 320}}},
  "unmatched": {"lossless": {"codec": "wav"}, "encoded": {"codec": "mp3", "quality": {"kind": "bitrate", "bitrate": 320}}},
  "members": [
    {"member_id": "m-…", "overrides": {"matched": {"lossless": {"codec": "flac"}}}},
    {"member_id": "m-…", "excluded": true}
  ]
}
```

A profile names a codec per lane: `lossless` is `wav` or `flac` and carries no
quality; `encoded` is `mp3`, `aac` (an `.m4a`) or `opus` (an `.opus`) and
requires a positive `quality.bitrate`. The settings offer three named pairs —
Opus 160, AAC 256, MP3 320 — but a preset writes that same codec + bitrate pair,
so nothing about it is stored or validated differently from a hand-made one.

- **Absence means inheritance.** A unit missing from `overrides` follows the
  common value, whatever that value is now. Null is never the way to express
  "clear": `"classifier_tags": []` is an explicit empty set, and removing the
  key restores inheritance. The two behave differently after later common
  changes.
- **Sparse storage.** The server normalizes on save: a member record with
  neither an exclusion nor any override is dropped, records are stored in
  member order, and a missing `classifier_tags` on the common level is stored
  as `[]`. `GET` returns that canonical document, so a save/read round trip
  never materializes unmodified units.
- **Structural vs business validation.** Save rejects unknown JSON fields,
  wrong types, unknown/duplicate `member_id`, unknown `mode` or `delete_mode`
  values and unknown codec or quality literals. It accepts structurally valid
  but incomplete settings (empty tags, undeclared outputs, missing quality).
  Generation requires every participating member's effective settings to pass
  the full reconcile policy validation (`INVALID_POLICY`).
- **Exclusion** changes participation only: overrides and inheritance survive
  it, and restoring participation restores the previous relationships.
  Excluding every member saves but cannot generate (`NO_ACTIVE_MEMBERS`).
- **Common settings are deployment-wide** (ADR 0010). The five common values
  are stored once per operation kind, not per record: saving them from any
  record's draft pushes them to every other editable record's draft in the same
  transaction, and a record created afterwards seeds them. The PUT draft body
  may include `"apply_common_settings":true` to publish the current record's
  common values even when they did not change; this lets an older record
  establish the global value explicitly. Without a stored global value, a
  replacement keeps the replaced record's own common values, while a new
  record in another library starts from the built-in seed. A save that changes
  only member records stays inside its own record; a generated plan is never
  rewritten. A record whose common fields moved reports `needs_planning` and must be re-generated before
  it can run. Orphaned (read-only) records never take part, and a record whose
  session is queued/running refuses the whole save with `COMMON_SETTINGS_BUSY`
  — as does one whose stored draft cannot be read
  (`COMMON_SETTINGS_UNREADABLE`, which names the record to recreate) — instead
  of leaving some records updated.

## Frozen revisions

`GET .../revisions/{planId}` resolves the revision's frozen draft snapshot into
per-member effective settings and per-unit sources (`common` | `member`), so a
historical revision always reports the configuration it was planned with —
equal values with different provenance stay distinguishable, and later common
changes never rewrite history. `counts` reports independent facts
(`members`, `changed`, `unmet_targets`, `blocked`, `unchanged`) that may
overlap; no exclusive status label is derived from them.

Conversion modes (policy `mode`, applies to both partitions): `strict` (the
historical rules; missing mode = strict) and `available_sources` (relaxed per
Variant Group). A partition's declared profile is its final audio set in either
mode: missing declared outputs are materialized from the stem's qualified
lossless source, and files the profile does not declare — including the source
a declared output was encoded from — are removed once those replacements
commit. A profile that declares no output means the partition holds no managed
audio: every observed file in it is removed. `available_sources` differs in
`strict`'s failure behavior: a stem whose declared shape cannot be reached is
left exactly as it is and recorded as `UNMET_TARGET` (no generation, no
cleanup), where strict blocks the whole Component with zero operations.
Ambiguity and path conflicts block in both.
An encoded output is accepted by two entries, in this order:

1. **Its own measured rate**, compared per codec — a file that already measures
   its target is not touched, whoever wrote it:

   | Codec | Accepted when |
   | --- | --- |
   | MP3 | probed bitrate at or above the target (1 kbps tolerance for probe under-reporting) |
   | AAC | inside `[0.95, 1.10]x` of a target up to 192k, inside `[0.60, 1.10]x` above it |
   | Opus | probed average at or above the target |

   MP3 is written CBR and lands exactly on target — the number leaves nothing to
   ask, so MP3 is judged by it alone. AAC is exact while the encoder can honour
   the target and saturates near 222 kbps for stereo 44.1 kHz above it (measured:
   1.00x at 192k, 0.86x for a 256k target, 0.65x for 320k), so above that the
   floor admits what can be produced instead of rebuilding its own output
   forever; the ceiling replaces a file well above the declared shape like one
   below it. A bitrate no probe can establish never accepts.
2. **A generation record.** An execution writes one per committed encoded output
   — codec, encoder, target bitrate, rate-control mode (CBR/VBR), the encoder
   version, the output's size and mtime, and the SHA-256 of its bytes — and only
   after the stream check and the full decode passed, so the record's presence is
   the verification. It accepts the file only when it names exactly the current
   codec, bitrate and mode *and* its size and mtime still match what the
   inventory observes (the same content proxy the scan and the inventory
   fingerprint use; the hash is what keeps a deeper re-check possible later).

The second entry is what settles the codecs a measurement cannot judge: Opus is
written VBR, where the same 160k setting averages 0.01x on silence, 0.68x on
dense noise and 1.13x on sparse material, and AAC saturates above 192k as above.
A file neither entry accepts is **unconfirmed**, never silently adequate — and no
measured rate is evidence of quality either, since a low-bitrate file re-encoded
to a high target would not regain what it never had. Where a qualified lossless
source exists the target is regenerated once and recorded; where none exists the
file is kept as it is and the stem counts as `UNMET_TARGETS` (relaxed mode marks
that kept file `CONFORMANCE_UNCONFIRMED`; strict mode blocks the component with
the same code and zero operations). Lossless targets carry no record: a lossless
file is accepted on its codec.

`StepSummary.unmet_targets` counts kept-but-unsatisfied stems; `summary_reason`
may be `UNMET_TARGETS`. New operation drafts seed `mode: "available_sources"`.

A record's scope is fixed once created: it is replaced, never re-scanned in
place. `folder_paths` come from `GET /api/v1/libraries/:id/dirs` of the owning
library, as library-relative paths.

Use the shared [operation routes and versions](worksets.md) and [execution sessions](executions.md#execution-sessions). Reusable policy values are supplied by [Policy slots and classifier tags](policies.md).
