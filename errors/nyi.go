package errors

import "fmt"

// ErrNotYetImplemented is fired when the Action is impossible to on the given Target;
// If the action is not possible on the target given its type, then IsTypeError should be true
type ErrNotYetImplemented struct {
	Action      string
	Target      interface{}
	IsTypeError bool
}

func (e *ErrNotYetImplemented) Error() string {
	if e.IsTypeError {
		return fmt.Sprintf("%s not yet implemented for %T", e.Action, e.Target)
	}
	return fmt.Sprintf("%s not yet implemented for %v", e.Action, e.Target)
}

// ErrDoFail is fired at runtime when a do operation fails
type ErrDoFail struct {
	NestedError error
	IsUnsafe    bool
}

func (e *ErrDoFail) Error() string {
	if e.IsUnsafe {
		return fmt.Sprintf("cannot perform unsafe do: %+v", e.NestedError)
	}
	return fmt.Sprintf("cannot perform do: %+v", e.NestedError)
}
