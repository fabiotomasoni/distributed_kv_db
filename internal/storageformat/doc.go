// Package storageformat implements the stateless v1 frame, entry/digest and
// control-manifest subset of docs/en/canonical-formats.md. It performs no I/O,
// determines no commit, replays no log and changes no state machine. Cross-frame
// vote/commit/suffix invariants require the future WAL owner. Snapshot codecs,
// session application and recovery/truncation are deliberately not implemented.
// Inputs must remain immutable during calls; returned buffers are owned copies.
package storageformat
