package response

import (
	"encoding/json"
	"net/http"
)

// Response 统一 API 响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// PagedData 分页数据包装
type PagedData struct {
	Items      interface{} `json:"items"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}

// Success 返回成功响应
func Success(w http.ResponseWriter, data interface{}) {
	SuccessWithStatus(w, http.StatusOK, data)
}

// SuccessWithStatus 返回带自定义 HTTP 状态码的成功响应。
func SuccessWithStatus(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

// SuccessPaged 返回分页成功响应
func SuccessPaged(w http.ResponseWriter, items interface{}, total, page, pageSize int) {
	totalPages := (total + pageSize - 1) / pageSize
	Success(w, PagedData{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	})
}

// Error 返回错误响应
func Error(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	httpStatus := http.StatusBadRequest
	if code >= 500 {
		httpStatus = http.StatusInternalServerError
	}
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(Response{
		Code:    code,
		Message: message,
		Data:    nil,
	})
}

// ErrorWithStatus 返回带 HTTP 状态码的错误响应
func ErrorWithStatus(w http.ResponseWriter, httpStatus int, code int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(Response{
		Code:    code,
		Message: message,
		Data:    nil,
	})
}

// ToJSON 返回标准响应的 JSON 字节
func ToJSON(code int, message string, data interface{}) []byte {
	resp := Response{
		Code:    code,
		Message: message,
		Data:    data,
	}
	b, _ := json.Marshal(resp)
	return b
}

// SuccessJSON 返回成功的 JSON 字节
func SuccessJSON(data interface{}) []byte {
	return ToJSON(0, "success", data)
}

// ErrorJSON 返回错误的 JSON 字节
func ErrorJSON(code int, message string) []byte {
	return ToJSON(code, message, nil)
}
