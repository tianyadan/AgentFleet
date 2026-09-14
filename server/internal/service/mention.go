package service

import (
	"regexp"
	"strconv"
	"strings"
)

// AgentRef 提及解析用的轻量引用。
type AgentRef struct {
	ID   int64
	Name string
}

// Mention 解析出的一次 @ 委托目标。
type Mention struct {
	ID   int64
	Name string
}

var mentionTokenRe = regexp.MustCompile(`@\[([^\]]+)\]\(#agent:(\d+)\)`)

// ParseMentions 解析 @[Name](#agent:ID) 与唯一匹配的 @Name；返回去重后的提及与去掉 token 后的正文。
func ParseMentions(question string, agents []AgentRef) (mentions []Mention, rest string) {
	seen := map[int64]bool{}
	rest = question

	for _, m := range mentionTokenRe.FindAllStringSubmatch(question, -1) {
		id, _ := strconv.ParseInt(m[2], 10, 64)
		name := m[1]
		if id <= 0 || seen[id] {
			continue
		}
		// 校验 id 存在
		ok := false
		for _, a := range agents {
			if a.ID == id {
				ok = true
				if name == "" {
					name = a.Name
				}
				break
			}
		}
		if !ok {
			continue
		}
		seen[id] = true
		mentions = append(mentions, Mention{ID: id, Name: name})
	}
	rest = mentionTokenRe.ReplaceAllString(rest, "")

	// 纯 @Name（词边界），仅当名称唯一命中且未被 token 选中
	byName := map[string][]AgentRef{}
	for _, a := range agents {
		byName[strings.ToLower(a.Name)] = append(byName[strings.ToLower(a.Name)], a)
	}
	words := regexp.MustCompile(`@([^\s@\[\]（）()]+)`).FindAllStringSubmatch(rest, -1)
	for _, w := range words {
		raw := w[1]
		list := byName[strings.ToLower(raw)]
		if len(list) != 1 {
			continue
		}
		a := list[0]
		if seen[a.ID] {
			continue
		}
		seen[a.ID] = true
		mentions = append(mentions, Mention{ID: a.ID, Name: a.Name})
		rest = strings.Replace(rest, w[0], "", 1)
	}

	rest = strings.TrimSpace(regexp.MustCompile(`\s{2,}`).ReplaceAllString(rest, " "))
	return mentions, rest
}

// ContainsAgentID 是否包含某 id。
func ContainsAgentID(ms []Mention, id int64) bool {
	for _, m := range ms {
		if m.ID == id {
			return true
		}
	}
	return false
}

// FormatMentionToken 前端插入用 token。
func FormatMentionToken(id int64, name string) string {
	return "@[" + name + "](#agent:" + strconv.FormatInt(id, 10) + ")"
}
