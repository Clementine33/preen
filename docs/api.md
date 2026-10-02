# Onsei Backend HTTP / SSE API

The backend runs a single net/http listener (the Vue web client) over
loopback. It binds `127.0.0.1` only and owns the one SQLite writer, the scan
service and the workset service.

HTTP endpoints live under `/api/v1`. Machine-checked contract coverage lives
in the Go tests (`backend/go/tests/e2e/*_test.go` and the
`backend/go/internal/adapters/httpapi` handler tests); this document is the human
reference.

## Contract documents

All endpoint documents use the shared authentication, error and path conventions.

| Document | Scope |
| --- | --- |
| [Common conventions](api/common.md) | Startup handshake, auth, CORS, error envelope, paths, health and shutdown |
| [Libraries, scans and file management](api/libraries.md) | Library CRUD, scan SSE, directories, member trees and direct file operations |
| [Processing records and operations](api/worksets.md) | Current records, scope, versions, drafts, generation and operation routes |
| [Rename and metadata refresh tasks](api/tasks.md) | Task payloads, rename proposals, metadata refresh and asynchronous copy covers |
| [Operation execution](api/executions.md) | Execution guards, scope, results, SSE, cancellation and inventory sync |
| [Audio conversion](api/conversion.md) | Sparse conversion drafts, common settings, frozen revisions and conformance rules |
| [Reusable policy slots and classifier tags](api/policies.md) | Policy slots and classifier tag vocabulary |
| [Work catalog](api/works.md) | Works, personal data, tags, content manifests, thumbnails, media facts, metadata imports and listing filters |
