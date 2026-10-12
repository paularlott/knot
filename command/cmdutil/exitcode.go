package cmdutil

// ExitCodeError ends the CLI with a specific exit status, such as that of a
// command run in a space, after printing its message.
type ExitCodeError struct {
	Code int
	Err  error
}

func (e *ExitCodeError) Error() string { return e.Err.Error() }
func (e *ExitCodeError) Unwrap() error { return e.Err }
