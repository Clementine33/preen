//go:build windows

package filesystem //nolint:testpackage // Narrow Windows-only test seam; external tests exercise behavior.

var ConversionMoveForTest = conversionMove
