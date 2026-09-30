package model

type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (err *Error) Error() string { return err.Code + ": " + err.Message }

func NewError(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

func Invalid(message string) *Error { return NewError(400, "INVALID_REQUEST", message) }
func Conflict(code string) *Error {
	return NewError(409, code, "Конфликт состояния или уникальности")
}
func NotFound() *Error { return NewError(404, "RESOURCE_NOT_FOUND", "Ресурс не найден") }

type ErrorResponse struct {
	Error ErrorDetails `json:"error"`
}

type ErrorDetails struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details"`
	RequestID string         `json:"request_id"`
}

func ErrorBody(err *Error, requestID string) ErrorResponse {
	return ErrorResponse{Error: ErrorDetails{
		Code:      err.Code,
		Message:   err.Message,
		Details:   map[string]any{},
		RequestID: requestID,
	}}
}
