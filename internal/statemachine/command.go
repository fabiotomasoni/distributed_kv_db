package statemachine

// Operation identifies one local state transition.
type Operation string

const (
	Put    Operation = "put"
	Get    Operation = "get"
	Delete Operation = "delete"
	CAS    Operation = "cas"
)

// Limits are the fixed logical limits in the approved data model.
const (
	MaxKeyBytes     = 256
	MaxValueBytes   = 16 * 1024
	MaxKeys         = 10_000
	MaxLogicalBytes = 4 * 1024 * 1024
)

// Expectation distinguishes an absent key from a present value, including an
// empty value. Value must be nil when Exists is false.
type Expectation struct {
	Exists bool
	Value  []byte
}

// Command is a typed, local command rather than a wire or persistence format.
// Value is used only by Put and CAS; a nil value denotes an empty byte sequence.
// Expected is required by CAS and forbidden for every other operation.
type Command struct {
	Operation Operation
	Key       string
	Value     []byte
	Expected  *Expectation
}

// Code is a deterministic domain outcome, using the planned API's error names.
type Code string

const (
	OK              Code = "OK"
	InvalidArgument Code = "INVALID_ARGUMENT"
	KeyNotFound     Code = "KEY_NOT_FOUND"
	CompareFailed   Code = "COMPARE_FAILED"
	ResourceLimit   Code = "RESOURCE_LIMIT"
)

// Result reports one command's outcome. Value and Exists describe a successful
// Get; Applied describes a successful Put or CAS; Deleted describes Delete.
// A failed command returns only its Code and leaves the machine unchanged.
type Result struct {
	Code    Code
	Value   []byte
	Exists  bool
	Applied bool
	Deleted bool
}
