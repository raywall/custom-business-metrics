// Package service contains the Custom Business Metrics HTTP service.
//
// The service is organized around a small hexagonal architecture:
// core domain types, application use cases, HTTP adapters, and an
// in-memory repository for the MVP. The repository can be replaced by
// DynamoDB, Timestream, PostgreSQL, or another adapter without changing
// the ingest and query use cases.
package service
