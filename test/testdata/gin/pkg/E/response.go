package E

// Success 成功响应包装函数
func Success(data interface{}) map[string]interface{} {
	return map[string]interface{}{
		"code": 0,
		"msg":  "success",
		"data": data,
	}
}

// Error 错误响应包装函数
func Error(code int, msg string) map[string]interface{} {
	return map[string]interface{}{
		"code": code,
		"msg":  msg,
		"data": nil,
	}
}