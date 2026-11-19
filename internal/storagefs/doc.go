// Package storagefs provides isolated Darwin/APFS locking, barriers and a small
// CURRENT replacement primitive. It is not a WAL, database or generation manager.
// A caller must validate/synchronize referenced generations before publication.
// All writers must cooperate with the stable LOCK; private local directories and
// no external mutation are preconditions. Contexts cannot interrupt OS syscalls.
package storagefs
