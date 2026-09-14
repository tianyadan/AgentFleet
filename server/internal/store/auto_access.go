package store

// CanControlAuto 判断客户端是否可开关该会话的 AI 自动审核。
// 管理台智能体会话归属 user_ip=admin,与浏览器 IP 不同,仍应允许控制。
func CanControlAuto(ownerIP, clientIP string) bool {
	if ownerIP == "" {
		return false
	}
	return ownerIP == clientIP || ownerIP == "admin"
}
