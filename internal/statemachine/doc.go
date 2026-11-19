// Package statemachine implements QuorumKV's sequential, in-memory key-value
// transitions. The caller supplies the operation order and must not access a
// Machine concurrently. There is no consensus, persistence or session handling.
//
// Commands and results carry raw bytes, not JSON or Base64. The future wire
// adapter is responsible for validating its own representation. Input and output
// buffers never alias the machine's stored values.
package statemachine
