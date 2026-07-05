package response

type apiError struct {
	Code    string              `json:"code"`
	Message string              `json:"message"`
	Details []ValidationDetail  `json:"details,omitempty"`
}

type ValidationDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type pagination struct {
	Page      int   `json:"page"`
	PerPage   int   `json:"per_page"`
	Total     int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}

type Envelope struct {
	Status     string          `json:"status"`
	Data       interface{}     `json:"data,omitempty"`
	Error_     *apiError       `json:"error,omitempty"`
	Pagination *pagination     `json:"pagination,omitempty"`
}

func SuccessCreated(data interface{}) Envelope {
	return Envelope{Status: "success", Data: data}
}

func SuccessOK(data interface{}) Envelope {
	return Envelope{Status: "success", Data: data}
}

func SuccessPaginated(data interface{}, page, perPage int, total int64) Envelope {
	var totalPages int64
	if perPage > 0 {
		totalPages = (total + int64(perPage) - 1) / int64(perPage)
	}
	return Envelope{
		Status: "success",
		Data:   data,
		Pagination: &pagination{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	}
}

func Error(status int, code, message string) Envelope {
	return Envelope{
		Status: "error",
		Error_: &apiError{Code: code, Message: message},
	}
}

func ValidationError(message string, details []ValidationDetail) Envelope {
	return Envelope{
		Status: "fail",
		Error_: &apiError{Code: "VALIDATION_ERROR", Message: message, Details: details},
	}
}
