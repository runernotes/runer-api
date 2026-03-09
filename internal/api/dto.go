package api

type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

type MessageResponse struct {
	Message string `json:"message"`
}
