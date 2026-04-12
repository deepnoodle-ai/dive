// Package a2alib is an experimental A2A server adapter that uses the official
// a2a-go SDK (github.com/a2aproject/a2a-go/v2) for protocol handling.
//
// It provides a thin bridge between Dive's Agent runtime and the a2a-go
// server framework: the executor translates between Dive's CreateResponse
// flow and the a2a-go event iterator model, while a2a-go handles transport
// (JSON-RPC, REST, gRPC), task persistence, streaming, and agent card serving.
//
// This is a prototype exploring whether adopting a2a-go simplifies the A2A
// surface in Dive compared to the hand-rolled implementation in
// experimental/a2a/.
package a2alib
