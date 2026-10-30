// Package workset owns one shared folder batch per library and its ordered members, operation
// drafts and revisions, the current plan, and planning/execution sessions. It
// coordinates identity, ownership, versions, idempotency and worker scheduling.
// Tasks may declare light planning for one extra worker without partitioning
// the general FIFO queue or exposing their business payloads to the scheduler.
// The dispatcher owns worker/session lifetimes: Stop interrupts active sessions
// and joins their child work before the application closes storage.
//
// Tasks provide operation-specific policy and opaque payloads through Task and
// the execution and member-conclusion seams. Workset owns member input health
// and component ownership while task conclusions stay opaque; a task may declare
// that read-time input freshness needs only the frozen roots (InputValidationTask),
// so summary reads skip component payloads; workset does not
// depend on a concrete task kind. Store
// expresses the atomic persistence operations this lifecycle needs, while
// inventory refresh and process admission are supplied by the caller.
package workset
