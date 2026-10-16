# Onsei Media Organization

Onsei organizes audio collections by comparing observed media with desired audio outputs. A media library IS the workbench: its folders are browsed, organized and converted there, and one shared Workset holds the current folder batch for every Task.

## Language

### Work catalog

**Work**:
One audio work, identified by its work number: the immutable identity, and the
whole reason two directories holding the same work share one entry. It is
discovered by scanning a directory whose *name* carries exactly one work number
— there is no other way to create one. Its metadata comes from a source, its
relations from the source's own report, and the user's rating and tags from
this deployment's single user.
_Avoid_: Treating a directory as the work, or a work as a copy of one.

**Work Number**:
The canonical form of a work's identity: `RJ` plus eight zero-padded digits.
Six-digit and eight-digit spellings of the same number are the same work; a
seven-digit run is not a work number at all.
_Avoid_: Using the source's own spelling as the key.

**Relation Placeholder**:
A work a source's relation report named but no scan has found on disk. It is
reachable through its relations and holds no personal data (writing it is
refused), and it becomes an ordinary entry the moment a scan finds it — its
collection time, not the source's report, is what makes it collected.
_Avoid_: A second way to create a work.

**Copy (Work Binding)**:
One scanned Work Root bound to one work. The binding is maintained by scanning and directory rename
and identified by the library and the relative path, so re-scanning keeps it
and a directory that stops naming a work is simply unbound. It carries no
foreign key to the inventory: a rescan deleting rows must never delete a work
or its personal data.
_Avoid_: Treating a copy as the work, or as a file.

**Source Data Group**:
One independently submittable slice of a work's source metadata — `core`
(the basic description), `statistics` (sales), `relations` (the translation
report) and `editions` (the language edition list). A group is submitted as a
complete snapshot or not at all, because the upstream channels differ: sales
and the translation report come from DLsite's dynamic endpoint, while the
edition list exists only on its static one.
_Avoid_: One combined snapshot that forces a crawler to resubmit what it did
not fetch.

**Age Category**:
The source's explicit age rating: all ages, R15 or R18. Missing source information
stays absent and has no visible UNKNOWN label. The compatibility NSFW flag only
identifies R18; a false flag cannot distinguish R15 from all ages.
_Avoid_: Treating every non-R18 work as all ages.

**Metadata Refresh**:
A request to retrieve source data groups for explicitly selected existing Works,
including Relation Placeholders. Missing-only refresh fills unreported groups;
dynamic refresh renews sales and relation reports; full refresh renews all groups.
Reported Chinese translation ancestors are filled as Relation Placeholders.
_Avoid_: Collecting sibling translations or following every language edition.

**Tag Language**:
The selected language for source tag names. It does not select a Work's title
language or its circle: Chinese edition titles follow their own edition language,
and their circle belongs to the Japanese original under its Japanese name.
_Avoid_: A title translation preference; the translator as the work's circle.

**Chinese Translation Chain**:
The Japanese original, a simplified or traditional Chinese edition page, and
an individual translation Work. The edition page and translation have distinct
Work Numbers even when their titles match. Several translations may share one
edition page; only translations with Copies in the current Library are in-library
lookup candidates.
_Avoid_: Treating the edition page as an alias that replaces a translation's identity.

**Language Edition**:
One language of a work's chain, rooted at its edition page. The Japanese
original is an edition in its own right although the source's edition list
omits it. A Chinese edition page itself is never on disk; the edition is present
through the translations under it. On a translation, its current edition is the
one its resolved parent roots. A work's chain shows the Japanese, simplified
Chinese and traditional Chinese editions; a language-free (NM) work has none.
_Avoid_: Calling an edition a "version", or treating the edition page as the thing that is collected.

**Held**:
A Work that has at least one Copy in any Library right now, online or not. An
edition is held when its root or any translation under it is held. A collected
Work that has lost every Copy is not held; a Relation Placeholder never is.
_Avoid_: Using "collected" (first found by a scan, kept forever) to mean held.

**Relation Evidence / Resolved Relation**:
*Evidence* is what sources said — a work's own relation report, and the child
edges other works declared. *Resolved* is what this build derives from the
evidence: the role, language, original and parent a reader sees. Evidence may
contain a conflict, which is stored and warned about; the resolved value is
recomputed whenever evidence changes, and being derived it never blocks a
deletion.
_Avoid_: Reading the derived relation as a source of truth about references.

### Collections and workbench

**Work Collection**:
A user-curated, ordered set of complete Works, independent of Libraries and physical Copies. A Work may belong to several collections; losing its Copies does not remove its membership. Its **saved order** is the user's arrangement and changes only by moving a member; viewing it sorted by another key never rewrites it.
_Avoid_: A track playlist, a Workset, a Library, or the act of finding a Work on disk during a scan (`collected_at`).

**Favorites**:
The built-in Work Collection representing the user's favorite Works. Its membership is the favorite flag; it can be reordered but cannot be renamed or deleted.
_Avoid_: A second favorite marker synchronized with collection membership.

**Library**:
A named media collection rooted at one directory, whose scanned inventory supplies the member directories and media facts. It owns the workbench: entering a library is entering its overview, its browsing, its conversion record and its file management. Its deletion removes its entry (and that entry's shared Workset) but never the media.

**Work Root**:
The first directory in a Library descendant chain whose name unambiguously carries one Work Number. Its whole subtree belongs to that independent work, even when an internal directory also carries a Work Number.
_Avoid_: Treating every RJ directory inside a work as another copy.

**Classification Directory**:
A directory organizing paths above Work Roots, with no work binding of its own. Its descendants may contain independent Work Roots.
_Avoid_: Selecting a category as one processing member.

**Album Folder / Planning Root**:
A Work Root of a Library, including its descendants. It is listed whether or not it holds audio, and each selected root is an independent boundary for classification, association and planning.

**Workset (shared folder batch)**:
The current, fixed, ordered set of 1–500 member directories selected in one Library. At most one exists per Library; conversion, metadata refresh and directory rename reuse the same members, including directories without audio. Replacing the batch replaces every activity's settings, current plan and execution results. The product entry keeps the name 工作集.
_Avoid_: One batch per Task; multiple named collections per Library; permanent per-record history.

**Member / Member ID**:
A scanned Work Root selected into one processing record. Its library-relative path (`rel_path`) is the durable data identity — a rescan cannot renumber it, and file management addresses items relative to it — `member_id` is the record-scoped identity the conversion routes carry, and `dir_id` is the identity its *page address* carries.
_Avoid_: Scan folder ID, list index, or the absolute path as member identity.

**Directory Identity (`dir_id`)**:
The navigation identity of one member directory, derived by the backend from a fixed version marker, the library, the canonical identity of its root and the directory's stored relative path. Page addresses and the tree routes carry it instead of the path, so an address names no folder. It is stable across rescans and restarts, unknown again once the directory is renamed or the library root changes, and it is never an access credential.
_Avoid_: Treating it as a permission, or confusing it with the data identity `rel_path`.

**Member Files / Current Files**:
What the shared file module shows for one member: the directory's contents as the last scan recorded them, refreshed when the page is entered. It serves every caller — the overview's browsing and a conversion member's page alike — and it reads no draft and interprets no plan.
_Avoid_: Conversion file browser; a page-owned tree.

**Plan Review**:
The read-only view of one member's frozen plan: what will be kept, deleted or generated, and which planned outputs do not exist yet. It is built from the frozen plan, never from the latest scan, and it never manages files.
_Avoid_: Using the current-files tree as the plan's view.

**Direct File Management**:
Renaming one item, moving one item inside its member, and soft-deleting one or more items into `Delete/<work name>/` beside the Work Root, without a plan. It validates its own paths and conflicts, reports per-item results, and never overwrites a destination.
_Avoid_: Treating these writes as a conversion step; disguising them as a plan.

**File Management Admission**:
The process-wide rule that direct file management and the managed task paths (scan, planning, execution) never interleave: one side is refused while the other is in flight, checked and registered atomically. Library root changes and deletions take the same slot.
_Avoid_: Frontend-only disabling; a queue of waiting file operations.

**Workset Operation**:
An independent business activity on a Workset's shared members, owning its draft, participation scope, planning sessions and its current plan. Conversion, metadata refresh and directory rename are the available activities; each reuses the same selected batch and retains its own settings, plan and results.
_Avoid_: Workflow step; filesystem Operation when referring to this activity.

**Operation Draft**:
The saved common settings, member field overrides and participation choices of one Workset Operation.
_Avoid_: Batch Draft, Workset-wide Workflow Draft.

**Common Settings**:
The current values inherited by members without an explicit override for the corresponding setting group.

**Field Override**:
An explicit member-specific replacement of conversion mode, no-effect tags, other-directory tags, matched targets, unmatched targets or isolated-version targets. Other groups keep inheriting; an explicit empty value or a value equal to the common value still counts as an override.

**Effective Settings**:
The settings resolved for a member from common values and its field overrides, together with each group's common or member source.

**Excluded Member**:
A member that does not participate in one Workset Operation but retains its membership and configuration relationships.

**Batch Edit**:
An explicit edit of selected setting groups for a fixed member list. Unedited groups and members outside that list retain their existing settings and inheritance.

**Policy Slot**:
One of three reusable, named policy templates, initially unconfigured. Applying a configured slot copies its policy; later template changes do not alter saved drafts or revisions.

**Classifier Tag Library**:
Reusable literal tags, comprising separate installation defaults for no-effect matching and other-directory isolation, plus shared user-saved custom strings. Drafts carry their own chosen tag values.

### Planning and review

**Task**:
One kind of work a Workset Operation can carry: it owns that work's draft document shape, its planning pass, its input facts and its execution. The generic workset module names the kind and payload schema version but never reads a task's payload; conversion, metadata refresh and directory rename supply their own task content.
_Avoid_: Workflow, job, plugin.

**Directory Rename Proposal**:
A member's proposed directory basename, rendered from one common naming
template and the bound Work's metadata. It preserves that Work Number and records
the raw name, cleaned name and any reason the proposal is blocked.
_Avoid_: Renaming the Work; a member-specific naming override.

**Plan**:
A reviewable proposal of media decisions and required filesystem changes, produced before execution.

**Plan Snapshot**:
The frozen payload of one Plan Revision as its Task wrote it: opaque to the generic workset module, carries `kind` and `schema_version` so a reader knows what it is looking at.

**Plan Revision (current plan)**:
The immutable proposal one Workset Operation currently holds: its draft, member participation, effective settings and sources, input facts and results are frozen. Publishing a new one retires the plan it replaced, with its payload and execution results, in the same transaction — each activity keeps one plan.
_Avoid_: Revision history; browsing an older plan of a record.

**Planning Session**:
An asynchronous attempt to produce a complete Plan Revision from frozen settings and member inputs. Only successful completion replaces the operation's current plan; failure, cancellation or interruption leaves the previous plan exactly as it was.

**Planning State**:
The operation's relationship to planning: unplanned, planning, planned, needs planning or orphaned. It is distinct from input validity and the proposal's results.

**Input Facts**:
What a plan was made from: per Planning Root, the identity, inventory fingerprint and entry count observed at planning time. The Task says how to recompute them and what counts as moved.
_Avoid_: Snapshot token, ETag.

**Execution Session**:
The asynchronous, durable run of one Plan Revision, across the whole revision or the members selected for this run. It is cooperatively cancelable, retains each committed result across interruption and does not resume automatically. Audio execution commits its frozen file decisions; metadata execution refreshes each distinct Work and records any cover demands for the selected Copies. Directory rename execution applies frozen basenames while retaining the Copies and recorded media outcomes.
_Avoid_: Treating a cover delivery's later failure as a change to the execution's final result.

**Metadata Refresh**:
An independent activity that fills missing source groups, refreshes dynamic groups or refreshes all groups for the Works bound to selected member directories. Copies of the same Work share one metadata result; an unbound member is skipped.
_Avoid_: Making a directory scan depend on an upstream metadata request.

**Main Cover**:
The source's main artwork, stored as one `cover.{ext}` in a Copy's directory. It retains the source format and is independent of sample and thumbnail URL fields retained for compatibility.
_Avoid_: Treating scaled variants of the same artwork as separate cover requirements.

**Cover Demand**:
An execution's request to deliver a Main Cover to one selected Copy. It can be stopped or retried independently; demands sharing the same Work and source version share a transfer while each Copy keeps its own publication result. Pending delivery resumes after interruption; a stopped demand requires an explicit retry.
_Avoid_: Extending a metadata Execution Session until its covers finish.

**Obsolete Audio Handling**:
The whole-operation choice of what happens to replaced and obsolete audio at execution: soft (moved to `Delete/<work name>/` in the Work Root's parent directory, preserving the work's internal paths and recoverable) or hard (removed). It is a draft setting, never a per-member override; the plan freezes it and the run uses the frozen value.
_Avoid_: Delete mode as a per-run option.

**Input Validity**:
Whether a revision's recorded inputs still agree with the current observed inventory: valid, stale or unavailable.

**Deleted Library**:
A Library entry the user removed: its processing record, plan and execution results are deleted with it, while the media files and the `Delete/` recovery directory on disk stay untouched. There are no orphaned records left behind to review.

### Audio reconciliation

**Plan Policy**:
A declaration of no-effect tags, other-directory tags, desired audio outputs for each partition and the conversion mode used to reconcile them.
_Avoid_: Slim/prune mode, user-composed conversion and cleanup steps.

**Filter Match / Classifier Partition**:
Whether an audio file's Planning Root-relative path contains a no-effect tag, ignoring case. The partitions are `matched` (UI “无音效”) and `unmatched` (UI “有音效”); these labels describe rule matching, not proof of audio effects.

**Version Scope**:
The main scope of one Planning Root, or the subtree of its outermost directory
segment matching an other-directory tag. Each actual matched root is independent;
files aggregate only inside their scope. No-effect classification still reads
the full original work-relative path. An unknown directory stays in the main
scope until a rule isolates it.
_Avoid_: One combined “other” bucket; isolating no-effect format directories.

**Desired Audio Profile**:
The exact managed audio outputs wanted for one classifier partition: at most one lossless output and one encoded output. It is the partition's final shape — media of an output it does not declare is obsolete once the declared set is reached, and a profile that declares nothing means the partition holds no managed audio at all.
_Avoid_: User-facing generation policy such as always or if missing.

**Target Encode Specification**:
The codec and quality required for an encoded output, such as MP3 at 320 kbps, against which existing encoded media is assessed.

**Media Facts / Observed Inventory**:
The observed files and their properties, including paths, codec, size, modification time and bitrate. These facts describe available inputs rather than desired outputs.

**Media Facts Pass**:
One automatic background walk over every file whose media facts are still unknown, started by a scan, a member refresh or startup. Any other work stops it and it resumes where it left off; it never refuses that work. A file that cannot be measured stays unknown and is tried again on the next pass.
_Avoid_: Media facts task (a Task is something the user starts through the task seam); treating it as a gate holder.

**Component**:
A transitive association of audio files sharing a parent directory or track stem within one Planning Root, Version Scope and classifier partition. It is the safety boundary for blocking changes and, in strict mode, maintaining encoded-output consistency.

**Variant Group**:
The files representing the same logical track within a Component, grouped by stem.

**Qualified Source**:
Observed lossless audio from which a requested output can be generated. Lossy audio is never a conversion source: not for a lossless output, and not for a codec change either, so a 256 kbps MP3 is no source for a 256 kbps AAC target.

**Conversion Mode**:
`strict` enforces Component-wide encoded consistency; `available_sources` preserves satisfied outputs and generates unsatisfied targets where qualified sources exist. Source ambiguity and safety conflicts remain blocking in either mode.

**Unknown Bitrate**:
An observed encoded file whose bitrate cannot be established, so it cannot be assumed to satisfy the target quality.

**Unmet Target**:
A desired output that cannot be supplied from available qualified sources in `available_sources` mode. The Variant Group it belongs to is left exactly as it is — existing media preserved, nothing removed — and the fact is disclosed separately from blocking conflicts; it does not by itself prevent execution.

**Decision**:
A reviewable conclusion such as keep, delete or encode, with its reason. A keep decision requires no filesystem change.

**Operation**:
An executable filesystem change derived from a Plan's Decisions, such as encoding or deletion.
_Avoid_: Workset Operation when referring to a file change.

**Reconciliation**:
Comparison of observed variants with their Desired Audio Profile to produce Decisions and required Operations under the chosen Conversion Mode.
