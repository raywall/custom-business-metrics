// Package agent contains the UDP metric agent.
//
// The agent listens for newline-delimited JSON metric events over UDP,
// buffers them in small batches, and forwards them to the service HTTP API.
package agent
