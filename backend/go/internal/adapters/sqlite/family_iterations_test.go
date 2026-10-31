//go:build !race

package sqlite_test

// familyIterations is the property run's length. The race build runs fewer:
// the plain suite already covers the full run, and the race job only needs the
// code paths exercised.
const familyIterations = 300
