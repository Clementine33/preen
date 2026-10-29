# Audio conversion

[API reference](../api.md) · [Common conventions](common.md)

## Operation draft document

The draft is one sparse document. The six common setting groups are the base;
a member record overrides only the units it explicitly replaces. The
operation-wide `delete_mode` (obsolete-audio handling: `soft` — the default —
or `hard`) is a common-level field, never a member override, and freezes into
every revision the draft produces; the execution uses the frozen value.

```json
{
  "schema_version": 1,
  "mode": "available_sources",
  "delete_mode": "soft",
  "no_effect_tags": ["SEなし"],
  "other_tags": ["反転"],
  "matched":   {"encoded": {"codec": "mp3", "quality": {"kind": "bitrate", "bitrate": 320}}},
  "unmatched": {"encoded": {"codec": "mp3", "quality": {"kind": "bitrate", "bitrate": 320}}},
  "other":     {"encoded": {"codec": "mp3", "quality": {"kind": "bitrate", "bitrate": 320}}},
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
  "clear": `"no_effect_tags": []` is an explicit empty set, and removing the
  key restores inheritance. The two behave differently after later common
  changes.
- **Sparse storage.** The server normalizes on save: a member record with
  neither an exclusion nor any override is dropped, records are stored in
  member order, and missing `no_effect_tags` and `other_tags` on the common level are stored
  as `[]`. `GET` returns the stored document, so a save/read round trip
  never materializes unmodified units.
- **Structural vs business validation.** Save rejects unknown JSON fields,
  wrong types, unknown/duplicate `member_id`, unknown `mode` or `delete_mode`
  values, unknown codec or quality literals, and `/` or `\` in any
  `other_tags` value (including excluded member overrides). It accepts structurally valid
  but incomplete settings (empty tags, undeclared outputs, missing quality).
  Generation requires every participating member's effective settings to pass
  the full reconcile policy validation (`INVALID_POLICY`).
- **Exclusion** changes participation only: overrides and inheritance survive
  it, and restoring participation restores the previous relationships.
  Excluding every member saves but cannot generate (`NO_ACTIVE_MEMBERS`).
- **Common settings are deployment-wide** (ADR 0008). The seven common values
  are stored once per operation kind, not per record, and are written only
  through [`PUT /api/v1/tasks/conversion/settings`](tasks.md#task-settings),
  which also rewrites every editable record's draft in the same transaction. A
  record created afterwards seeds them; without a stored value a replacement
  keeps the replaced record's own common values, while a new record in another
  library starts from the built-in seed. The draft PUT refuses a common section
  that differs from them (`COMMON_SETTINGS_MISMATCH`), so a save through it only
  moves member records and stays inside its own record. A generated plan is
  never rewritten; a record whose common fields moved reports `needs_planning`
  and must be re-generated before it can run. The settings save refuses as a
  whole (`COMMON_SETTINGS_BUSY`, `COMMON_SETTINGS_UNREADABLE`) rather than leave
  some records updated.

## Version matching and isolation

`no_effect_tags` contains literal substrings matched case-insensitively against
an audio file's full original Planning Root-relative path, including its filename.
In the main scope it selects the `matched` (无音效) or `unmatched` (有音效) target profile.

`other_tags` contains literal substrings matched case-insensitively against each
directory segment only. The outermost matching directory starts an independent
scope containing its whole subtree; nested matches do not create another scope.
Every actual matching directory is independent, even when the same tag matches
several directories. An empty list disables isolation. Rules cannot contain path
separators. Scope selection runs before no-effect classification; classification
still uses the original full relative path within every scope. Remaining files
share the main scope. Same-parent/same-stem component association then runs
separately for each scope and classifier partition. Every isolated scope uses the
independent `other` target profile for both partitions. The main targets do not
affect it. New drafts seed all three profiles as MP3 320 with no lossless output;
restoring defaults uses the same profiles. Stored profiles are not reset. Older
drafts and frozen revisions without `other` retain their partition targets until
an explicit other target is saved.

Thus `WAV_NoSE` and `MP3_NoSE` can aggregate in the main matched partition, while
`revert` and `wav/xxx` aggregate only within their respective subtrees. An unknown
version stays in the main scope until its directory tag is registered. Resolving
it uses the existing flow: filter 源竞争, add a common or batch `other_tags` rule,
save, regenerate and inspect the scopes in member review.

The six independent override units are `mode`, `no_effect_tags`, `other_tags`,
`matched`, `unmatched` and `other`. Each tag list supports explicit `[]` to clear the unit.
An empty no-effect list remains saveable; generation requires at least one tag.

New HTTP draft and slot writes reject the old `classifier_tags` key, including
`null` and `[]`. Stored schema-1 drafts, common settings and policies remain
readable: the old key supplies `no_effect_tags` only when the new value is absent
or null, so an explicit new `[]` wins. Stored draft and slot GET payloads and
frozen policy payloads may retain the old spelling; the web API client normalizes
them before editing or comparing drafts. No read rewrites a frozen payload or
changes its hash. Effective member settings and their sources use the new keys.

## Frozen revisions

`GET .../revisions/{planId}` resolves the revision's frozen draft snapshot into
per-member effective settings and per-unit sources (`common` | `member`), so a
historical revision always reports the configuration it was planned with —
equal values with different provenance stay distinguishable, and later common
changes never rewrite history. In member review, main components show only
有音效 or 无音效; isolated components show only 其他版本, while the file rows
continue to show their directories. `counts` reports independent facts
(`members`, `changed`, `unmet_targets`, `blocked`, `unchanged`) that may
overlap; no exclusive status label is derived from them.

Every revision member also carries a server-computed `conclusion` (see
[Member Conclusion](worksets.md#routes)). Its conversion
`task` payload contains `status`, `matched` (无音效), and `unmatched` (有音效).
Each partition contains independent boolean facts `applicable`, `blocked`,
`unmet`, and `changes`, so an unmet target can coexist with changes.
The status is the first matching result in this order: `blocked` (a blocker
other than source ambiguity), `source_ambiguous`, `partially_unmet` / `unmet`,
`changed`, `not_applicable` (no components), `unchanged`. Unmet classification
counts this member's components, including multiple components in the same
partition: one unmet component is `unmet`; an unmet component plus an unchanged
component is `partially_unmet`. Excluded, no-input, and missing-input member
states take precedence over this task status. The browser maps these results
to labels, filters, and explanations without deriving them from components.

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
Ambiguity and path conflicts block in both. Qualified source priority is the
declared lossless codec, then WAV, then FLAC. Multiple candidates at the selected
priority remain `SOURCE_AMBIGUOUS` when an output needs generating; their actual
paths appear in file decisions. `ComponentOutcome.scope` is the matched directory's
Planning Root-relative path; absent or `""` means the main scope. Component identity
continues to hash its partition and observed files; scope adds no hash salt.

### FLAC sample representation

Floating-point WAV sources (32-bit or 64-bit) are converted by FFmpeg to 24-bit
integer FLAC, preserving sample rate and channel count. This quantizes the source
samples: no dithering, normalization or peak pre-scan is applied, and samples
outside the integer full-scale range are hard-clipped. The lossless target class
names the codec family, not a guarantee of sample-exact preservation of floating
point sources. Integer sources above 24 bits remain unsupported for FLAC targets.

After successful validation and publication, obsolete source WAV files follow the
frozen soft/hard handling setting like other source files: soft moves them to the
recovery directory, while hard permanently removes the original floating-point
data. Output validation checks the stream properties and full decode; it does not
check for quantization loss or clipping.

### Encoded output acceptance

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

Use the shared [operation routes and versions](worksets.md) and [execution sessions](executions.md#execution-sessions). Reusable classifier tags are supplied by the [classifier tag library](policies.md).
