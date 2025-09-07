package payments

import "fmt"

type Reason string

const (
	ErrorReasonSignatureValidation           Reason = "SignatureValidationError"
	ErrorReasonFailedToCreateCheckoutSession Reason = "FailedToCreateCheckoutSession"
	ErrorReasonInvalidWebhookEventData       Reason = "InvalidWebhookEventData"
	ErrorReasonNotCheckoutConfirmedEvent     Reason = "NotCheckoutConfirmedEvent"
	ErrorReasonNotPaid                       Reason = "PaymentStatusIsNotPaid"
)

var _ error = &Error{}

type Error struct {
	Message string
	Reason  Reason
	Cause   error
}

func (e *Error) Error() string {
	s := fmt.Sprintf("%s: %s.", e.Reason, e.Message)
	if e.Cause != nil {
		s += fmt.Sprintf(" Cause: %s", e.Cause)
	}
	return s
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func newError(reason Reason, message string, cause error) *Error {
	return &Error{
		Reason:  reason,
		Message: message,
		Cause:   cause,
	}
}

func NewSignatureValidationError(message string, cause error) *Error {
	return newError(ErrorReasonSignatureValidation, message, cause)
}

func NewFailedToCreateCheckoutSessionError(message string, cause error) *Error {
	return newError(ErrorReasonFailedToCreateCheckoutSession, message, cause)
}

func NewInvalidWebhookEventDataError(message string, cause error) *Error {
	return newError(ErrorReasonInvalidWebhookEventData, message, cause)
}

func NewNotCheckoutConfirmedEventError(message string) *Error {
	return newError(ErrorReasonNotCheckoutConfirmedEvent, message, nil)
}

func NewNotPaidError(message string) *Error {
	return newError(ErrorReasonNotPaid, message, nil)
}
