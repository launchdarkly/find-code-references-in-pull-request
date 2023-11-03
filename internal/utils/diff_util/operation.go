package diff_util

import "strings"

// Operation defines the operation of a diff item.
type Operation int

const (
	// OperationEqual item represents an equals diff.
	OperationEqual Operation = iota
	// OperationAdd item represents an insert diff.
	OperationAdd
	// OperationDelete item represents a delete diff.
	OperationDelete
)

func LineOperation(line string) Operation {
	if strings.HasPrefix(line, OperationAdd.String()) {
		return OperationAdd
	}
	if strings.HasPrefix(line, OperationDelete.String()) {
		return OperationDelete
	}

	return OperationEqual
}

func (o Operation) String() string {
	switch o {
	case OperationAdd:
		return "+"
	case OperationDelete:
		return "-"
	case OperationEqual:
		return ""
	}

	return ""
}
