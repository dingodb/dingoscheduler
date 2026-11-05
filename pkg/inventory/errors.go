package inventory

// Rejection identifies a report that cannot succeed by resending the same bytes.
type Rejection struct {
	Code    string
	Message string
}

func (e *Rejection) Error() string { return e.Message }

func Reject(code, message string) error { return &Rejection{Code: code, Message: message} }
