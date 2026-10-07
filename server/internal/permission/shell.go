package permission

import (
	"path/filepath"
	"strings"
	"unicode"
)

// ParseArgv 解析 shell 命令为 argv（处理引号；按 ; && || | 只取第一段用于签名）。
func ParseArgv(cmd string) []string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return nil
	}
	seg := firstSegment(cmd)
	toks := tokenize(seg)
	return stripWrappers(toks)
}

// NormalizeCommand 规范化命令文本（折叠空白、去掉 sudo 包装）。
func NormalizeCommand(cmd string) string {
	return strings.Join(ParseArgv(cmd), " ")
}

func firstSegment(cmd string) string {
	rs := []rune(cmd)
	var b strings.Builder
	inS, inD := false, false
	for i := 0; i < len(rs); i++ {
		ch := rs[i]
		if ch == '\\' && i+1 < len(rs) {
			b.WriteRune(rs[i+1])
			i++
			continue
		}
		if ch == '\'' && !inD {
			inS = !inS
			b.WriteRune(ch)
			continue
		}
		if ch == '"' && !inS {
			inD = !inD
			b.WriteRune(ch)
			continue
		}
		if !inS && !inD {
			if ch == '\n' || ch == ';' {
				break
			}
			if ch == '|' || ch == '&' {
				if i+1 < len(rs) && rs[i+1] == ch {
					break
				}
				if ch == '|' {
					break
				}
			}
		}
		b.WriteRune(ch)
	}
	return strings.TrimSpace(b.String())
}

func tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	inS, inD := false, false
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		out = append(out, cur.String())
		cur.Reset()
	}
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		ch := rs[i]
		if ch == '\\' && i+1 < len(rs) && !inS {
			cur.WriteRune(rs[i+1])
			i++
			continue
		}
		if ch == '\'' && !inD {
			inS = !inS
			continue
		}
		if ch == '"' && !inS {
			inD = !inD
			continue
		}
		if !inS && !inD && unicode.IsSpace(ch) {
			flush()
			continue
		}
		cur.WriteRune(ch)
	}
	flush()
	return out
}

func stripWrappers(toks []string) []string {
	for len(toks) > 0 {
		bin := filepath.Base(strings.Trim(toks[0], `"'`))
		low := strings.ToLower(bin)
		switch low {
		case "sudo":
			toks = dropSudo(toks[1:])
			continue
		case "env", "command", "time", "nice", "nohup":
			toks = toks[1:]
			continue
		}
		if strings.Contains(toks[0], "=") && !strings.HasPrefix(toks[0], "-") {
			toks = toks[1:]
			continue
		}
		break
	}
	return toks
}

func dropSudo(toks []string) []string {
	for len(toks) > 0 && strings.HasPrefix(toks[0], "-") {
		flag := toks[0]
		toks = toks[1:]
		if flag == "-u" || flag == "-g" || flag == "-C" || flag == "--user" || flag == "--group" {
			if len(toks) > 0 {
				toks = toks[1:]
			}
		}
	}
	return toks
}

func binName(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	return strings.ToLower(filepath.Base(argv[0]))
}

func hasPipeToShell(cmd string) bool {
	low := strings.ToLower(cmd)
	// 管道到解释器：解析 | 后第一词
	inS, inD := false, false
	rs := []rune(low)
	for i := 0; i < len(rs); i++ {
		ch := rs[i]
		if ch == '\'' && !inD {
			inS = !inS
			continue
		}
		if ch == '"' && !inS {
			inD = !inD
			continue
		}
		if inS || inD {
			continue
		}
		if ch == '|' && (i+1 >= len(rs) || rs[i+1] != '|') {
			rest := strings.TrimSpace(string(rs[i+1:]))
			fields := strings.Fields(rest)
			if len(fields) == 0 {
				return false
			}
			b := filepath.Base(strings.Trim(fields[0], `"'`))
			switch b {
			case "sh", "bash", "zsh", "ksh", "fish", "dash":
				return true
			}
		}
	}
	return false
}
