// Package librarypath defines library-root identity, stored POSIX paths and
// lexical containment without consulting the host filesystem. Windows drive,
// UNC and device syntax is understood on every host; native resolution and
// filesystem indirection belong to the filesystem adapter. A lexical check is
// not sufficient to authorize a read or write on disk.
package librarypath
