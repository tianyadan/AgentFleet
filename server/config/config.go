package config

import (
	"os"
	"strings"
	"time"
)

// Config 汇总后端所需配置。全部用环境变量覆盖,可用 .env 或 scripts/start.sh 注入。
type Config struct {
	Addr          string   // HTTP 监听地址
	DBDSN         string   // MySQL DSN(本地 colleague_avatar)
	DataDBDSN     string   // 数据查询库 DSN;为空时禁用 /api/db 查询能力
	ClaudeBin     string   // claude CLI 路径
	AllowedIPs    []string // 局域网 IP 白名单
	WorkspaceRoot string   // 授权工作区根(用于 resolve 绝对路径)
	TimeoutSec    int      // 分身单次回答超时(秒)

	PermissionEnabled bool   // 是否启用工具授权代理(默认 true)
	PermissionWaitSec int    // 等待用户点授权的秒数
	ReviewTimeoutSec  int    // AI 自动审核单次调用超时(秒)
	HookBin           string // PreToolUse hook 可执行文件绝对路径
	BackendURL        string // hook 回调的后端地址(供 CLI 子进程使用)
	NotifyScript      string // 待授权 Bark 脚本(scripts/notify-permission.sh)
	BarkNotify        bool   // 是否推送 Bark(默认 true;AVATAR_BARK_NOTIFY=0 关闭)

	SecretKey                string // 测试服务器密码 AES 密钥
	SSHInsecureIgnoreHostKey bool   // 开发环境忽略 SSH host key

	AdminUser   string // 管理台账号
	AdminPass   string // 管理台密码(明文比对)
	JWTSecret   string // 管理台 JWT HS256 密钥
	JWTTTLHours int    // JWT 有效小时数

	// 数字员工头像 OSS（未配齐则上传接口不可用）
	OSSEndpoint        string // 如 oss-cn-qingdao.aliyuncs.com
	OSSAccessKeyID     string
	OSSAccessKeySecret string
	OSSBucket          string
	OSSPrefix          string // object 前缀，如 avatars/
	OSSPublicBase      string // 公网访问根，如 https://digital-employee-qd.cn-qingdao.taihangcda.cn
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Load 从环境变量加载配置,带默认值。
func Load() Config {
	ips := []string{}
	if raw := os.Getenv("AVATAR_ALLOWED_IPS"); raw != "" {
		ips = strings.Split(raw, ",")
	}
	return Config{
		Addr:          getenv("AVATAR_ADDR", ":8080"),
		DBDSN:         getenv("AVATAR_DB_DSN", "root:root@tcp(127.0.0.1:3307)/colleague_avatar?parseTime=true&charset=utf8mb4&loc=Local"),
		DataDBDSN:     getenv("AVATAR_DATA_DB_DSN", ""),
		ClaudeBin:     getenv("AVATAR_CLAUDE_BIN", "claude"),
		AllowedIPs:    ips,
		WorkspaceRoot: getenv("AVATAR_WORKSPACE_ROOT", "/Users/tianhaowen/Desktop/code_work_space"),
		TimeoutSec:    clampTimeoutSec(atoi(getenv("AVATAR_TIMEOUT_SEC", "28800"))),

		PermissionEnabled: getenv("AVATAR_PERMISSION_ENABLED", "1") != "0",
		PermissionWaitSec: atoi(getenv("AVATAR_PERMISSION_WAIT_SEC", "120")),
		ReviewTimeoutSec:  atoi(getenv("AVATAR_REVIEW_TIMEOUT_SEC", "20")),
		HookBin:           getenv("AVATAR_HOOK_BIN", ""),
		BackendURL:        getenv("AVATAR_BACKEND_URL", "http://127.0.0.1:8080"),
		NotifyScript:      getenv("AVATAR_NOTIFY_SCRIPT", ""),
		BarkNotify:        getenv("AVATAR_BARK_NOTIFY", "1") != "0",

		SecretKey:                getenv("AVATAR_SECRET_KEY", "colleague-avatar-dev-secret-change-me"),
		SSHInsecureIgnoreHostKey: getenv("AVATAR_SSH_INSECURE_IGNORE_HOSTKEY", "1") != "0",

		AdminUser:   getenv("AVATAR_ADMIN_USER", "tianhaowen"),
		AdminPass:   getenv("AVATAR_ADMIN_PASS", "12345678"),
		JWTSecret:   getenv("AVATAR_JWT_SECRET", "colleague-avatar-jwt-dev-secret-change-me"),
		JWTTTLHours: atoi(getenv("AVATAR_JWT_TTL_HOURS", "168")),

		OSSEndpoint:        strings.TrimSpace(getenv("AVATAR_OSS_ENDPOINT", "")),
		OSSAccessKeyID:     strings.TrimSpace(getenv("AVATAR_OSS_ACCESS_KEY_ID", "")),
		OSSAccessKeySecret: strings.TrimSpace(getenv("AVATAR_OSS_ACCESS_KEY_SECRET", "")),
		OSSBucket:          strings.TrimSpace(getenv("AVATAR_OSS_BUCKET", "")),
		OSSPrefix:          strings.Trim(strings.TrimSpace(getenv("AVATAR_OSS_PREFIX", "avatars/")), "/") + "/",
		OSSPublicBase:      strings.TrimRight(strings.TrimSpace(getenv("AVATAR_OSS_PUBLIC_BASE", "")), "/"),
	}
}

// OSSConfigured 头像上传所需 OSS 是否齐全。
func (c Config) OSSConfigured() bool {
	return c.OSSEndpoint != "" && c.OSSAccessKeyID != "" && c.OSSAccessKeySecret != "" &&
		c.OSSBucket != "" && c.OSSPublicBase != ""
}

// JWTTTL 返回管理员 JWT 有效期(至少 1 小时)。
func (c Config) JWTTTL() time.Duration {
	h := c.JWTTTLHours
	if h < 1 {
		h = 168
	}
	return time.Duration(h) * time.Hour
}

// ReviewTimeout 返回 AI 审核单次调用超时(至少 5s)。
func (c Config) ReviewTimeout() int {
	if c.ReviewTimeoutSec < 5 {
		return 20
	}
	return c.ReviewTimeoutSec
}

// PermissionWaitSeconds 返回等待用户授权的时长(至少 30s)。
func (c Config) PermissionWaitSeconds() int {
	if c.PermissionWaitSec < 30 {
		return 120
	}
	return c.PermissionWaitSec
}

func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0
		}
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// MaxTimeoutSec 单次问答最长 8 小时。
const MaxTimeoutSec = 8 * 60 * 60

func clampTimeoutSec(n int) int {
	if n <= 0 {
		return MaxTimeoutSec
	}
	if n > MaxTimeoutSec {
		return MaxTimeoutSec
	}
	return n
}

// AskTimeout 返回分身单次超时 Duration。
func (c Config) AskTimeout() time.Duration {
	return time.Duration(clampTimeoutSec(c.TimeoutSec)) * time.Second
}
