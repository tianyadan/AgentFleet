package permission

import (
	"path/filepath"
	"strings"
)

var safeBins = map[string]bool{
	"ls": true, "pwd": true, "cat": true, "head": true, "tail": true,
	"less": true, "more": true, "grep": true, "rg": true, "ag": true,
	"find": true, "wc": true, "stat": true, "file": true, "which": true,
	"basename": true, "dirname": true, "echo": true, "printf": true,
	"date": true, "true": true, "false": true, "test": true,
	"awk": true, "cut": true, "tr": true, "sort": true, "uniq": true, "jq": true,
}

var gitRead = map[string]bool{
	"status": true, "log": true, "diff": true, "show": true, "blame": true,
	"branch": true, "ls-files": true, "ls-tree": true, "rev-parse": true,
	"describe": true, "shortlog": true, "remote": true,
}

var diskBins = map[string]bool{
	"mkfs": true, "wipefs": true, "fdisk": true, "parted": true, "sfdisk": true,
}

// ClassifyShellAction 由 argv 映射平台 ActionType。
func ClassifyShellAction(argv []string, raw string) string {
	if hasPipeToShell(raw) {
		return ActionShellExec
	}
	bin := binName(argv)
	if bin == "" {
		return ActionShellExec
	}
	if strings.HasPrefix(bin, "mkfs") {
		return ActionSystemConfig
	}
	switch bin {
	case "rm", "unlink", "rmdir":
		return ActionFileDelete
	case "chmod", "chown", "chgrp":
		return ActionPermissionChange
	case "systemctl", "service":
		if looksRestart(argv) {
			return ActionServiceRestart
		}
		if looksStop(argv) {
			return ActionProcessStop
		}
		return ActionProcessStart
	case "docker", "kubectl", "podman":
		if looksRestart(argv) {
			return ActionServiceRestart
		}
		if looksStop(argv) {
			return ActionProcessStop
		}
		return ActionShellExec
	case "kill", "killall", "pkill":
		return ActionProcessStop
	case "curl", "wget":
		return ActionNetworkWrite
	case "psql", "mysql", "sqlite3":
		joined := strings.ToLower(strings.Join(argv, " "))
		if strings.Contains(joined, "drop database") || strings.Contains(joined, "drop schema") || strings.Contains(joined, "drop table") {
			return ActionDBDelete
		}
		if strings.Contains(joined, "insert") || strings.Contains(joined, "update") || strings.Contains(joined, "delete ") || strings.Contains(joined, "alter ") {
			return ActionDBWrite
		}
		return ActionDBRead
	case "git":
		sub := gitSub(argv)
		if gitRead[sub] {
			return ActionShellRead
		}
		return ActionShellExec
	case "dd":
		return ActionSystemConfig
	}
	if safeBins[bin] {
		if bin == "find" && findMutates(argv) {
			return ActionFileDelete
		}
		if bin == "sed" && sedInPlace(argv) {
			return ActionFileWrite
		}
		return ActionShellRead
	}
	return ActionShellExec
}

func looksRestart(argv []string) bool {
	for _, t := range argv {
		if strings.EqualFold(t, "restart") || strings.EqualFold(t, "reload") {
			return true
		}
	}
	return false
}

func looksStop(argv []string) bool {
	for _, t := range argv {
		if strings.EqualFold(t, "stop") || strings.EqualFold(t, "kill") || strings.EqualFold(t, "down") {
			return true
		}
	}
	return false
}

func gitSub(argv []string) string {
	for i := 1; i < len(argv); i++ {
		if !strings.HasPrefix(argv[i], "-") {
			return strings.ToLower(argv[i])
		}
	}
	return ""
}

func findMutates(argv []string) bool {
	for _, t := range argv {
		l := strings.ToLower(t)
		if l == "-delete" || l == "-exec" || strings.HasPrefix(l, "-exec") {
			return true
		}
	}
	return false
}

func sedInPlace(argv []string) bool {
	for _, t := range argv {
		if t == "-i" || strings.HasPrefix(t, "--in-place") {
			return true
		}
		if strings.HasPrefix(t, "-") && !strings.HasPrefix(t, "--") && strings.Contains(t, "i") {
			return true
		}
	}
	return false
}

// IsHardDeny 灾难性操作：会话授权与 JEVOS 均不可覆盖。
func IsHardDeny(a ToolAction) bool {
	if hasPipeToShell(a.Command) {
		return true
	}
	argv := a.argv
	if len(argv) == 0 {
		argv = ParseArgv(a.Command)
	}
	bin := binName(argv)
	if strings.HasPrefix(bin, "mkfs") || diskBins[bin] {
		return true
	}
	if bin == "dd" && ddWritesDisk(argv) {
		return true
	}
	if bin == "rm" && rmTargetsRoot(argv) {
		return true
	}
	if (bin == "chmod" || bin == "chown") && recursiveRoot(argv) {
		return true
	}
	joined := strings.ToLower(strings.Join(argv, " "))
	if strings.Contains(joined, "drop database") || strings.Contains(joined, "drop schema") {
		return true
	}
	return false
}

func ddWritesDisk(argv []string) bool {
	for _, t := range argv {
		if strings.HasPrefix(strings.ToLower(t), "of=/dev/") {
			rest := strings.ToLower(strings.TrimPrefix(t, "of="))
			if strings.HasPrefix(rest, "/dev/sd") || strings.HasPrefix(rest, "/dev/nvme") || strings.HasPrefix(rest, "/dev/disk") || rest == "/dev/sda" {
				return true
			}
		}
	}
	return false
}

func rmTargetsRoot(argv []string) bool {
	forceRec := false
	var paths []string
	for i := 1; i < len(argv); i++ {
		t := argv[i]
		if strings.HasPrefix(t, "-") {
			if strings.Contains(t, "r") || strings.Contains(t, "R") || t == "--recursive" {
				forceRec = true
			}
			continue
		}
		paths = append(paths, t)
	}
	if !forceRec {
		return false
	}
	for _, p := range paths {
		clean := filepath.Clean(p)
		if clean == "/" || clean == "/*" || p == "/*" || p == "/" {
			return true
		}
		// rm -rf /*  token is /*
		if strings.TrimSpace(p) == "/*" {
			return true
		}
	}
	return false
}

func recursiveRoot(argv []string) bool {
	rec := false
	var paths []string
	for i := 1; i < len(argv); i++ {
		t := argv[i]
		if t == "-R" || t == "-r" || t == "--recursive" {
			rec = true
			continue
		}
		if strings.HasPrefix(t, "-") {
			continue
		}
		paths = append(paths, t)
	}
	if !rec {
		return false
	}
	for _, p := range paths {
		if filepath.Clean(p) == "/" {
			return true
		}
	}
	return false
}

// IsSafeAllow 确定性只读安全命令。
func IsSafeAllow(a ToolAction) bool {
	if IsHardDeny(a) {
		return false
	}
	if a.ActionType == ActionFileRead {
		return true
	}
	argv := a.argv
	if len(argv) == 0 {
		argv = ParseArgv(a.Command)
	}
	bin := binName(argv)
	if bin == "git" {
		return gitRead[gitSub(argv)] && !gitDangerFlags(argv)
	}
	if bin == "find" && findMutates(argv) {
		return false
	}
	if bin == "sed" && sedInPlace(argv) {
		return false
	}
	if !safeBins[bin] {
		return false
	}
	joined := strings.Join(argv, " ")
	if strings.Contains(joined, ">") {
		return false
	}
	return a.ActionType == ActionShellRead || a.ActionType == ActionFileRead
}

func gitDangerFlags(argv []string) bool {
	for _, t := range argv {
		if t == "-c" || strings.HasPrefix(t, "-c") {
			return true
		}
	}
	return false
}

func staticRuleID(a ToolAction, deny bool) string {
	if deny {
		return "hard_deny:" + a.ActionType
	}
	return "safe_allow:" + a.ActionType
}
