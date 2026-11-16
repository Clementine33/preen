package httpapi

// Schemas are generated in tests so neither the generator nor the validator is
// part of the server binary. The first step discovers private DTOs in this package.
//go:generate go run ./schemagen
//go:generate env PREEN_GENERATE_SCHEMA=1 go test -run ^TestGenerateWireSchema$ -count=1 .
