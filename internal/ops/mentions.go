package ops

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
)

// A plain "@ana" in Markdown is just text to OpenProject: nobody is
// notified. The editor stores mentions as <mention> tags, so opcli rewrites
// @login into one when the login belongs to a user. What does not resolve is
// left as typed and reported, never guessed.

var (
	mentionRe  = regexp.MustCompile(`(^|[^\p{L}\p{N}_@./\\-])@([\p{L}\p{N}_](?:[\p{L}\p{N}_.\-]*[\p{L}\p{N}_])?)`)
	mentionTag = regexp.MustCompile(`(?s)<mention\b.*?</mention>`)
)

type mentions struct {
	Mentioned  []string
	Unresolved []string
	users      map[string]*mentionUser // by lower-case login; nil = not a user
}

type mentionUser struct {
	ID   int
	Name string
}

// report adds what happened to the result of a write.
func (m *mentions) report(out map[string]any) {
	if m == nil || out == nil {
		return
	}
	if len(m.Mentioned) > 0 {
		out["mentioned"] = m.Mentioned
	}
	if len(m.Unresolved) > 0 {
		out["unresolvedMentions"] = m.Unresolved
	}
}

// mentionArgs rewrites the @logins of the given text params in place.
func (e *Env) mentionArgs(ctx context.Context, a Args, params ...string) (*mentions, error) {
	m := &mentions{users: map[string]*mentionUser{}}
	for _, p := range params {
		if !a.Has(p) {
			continue
		}
		s, err := e.linkMentions(ctx, a.String(p), m)
		if err != nil {
			return nil, err
		}
		a[p] = s
	}
	return m, nil
}

func (e *Env) linkMentions(ctx context.Context, text string, m *mentions) (string, error) {
	masked := []byte(maskCode(text))
	for _, loc := range mentionTag.FindAllIndex(masked, -1) {
		blank(masked[loc[0]:loc[1]])
	}
	matches := mentionRe.FindAllSubmatchIndex(masked, -1)
	for i := len(matches) - 1; i >= 0; i-- { // from the end, so offsets stay valid
		at, start, end := matches[i][4]-1, matches[i][4], matches[i][5]
		login := text[start:end]
		u, err := e.mentionUser(ctx, login, m)
		if err != nil {
			return "", err
		}
		if u == nil {
			m.Unresolved = appendOnce(m.Unresolved, "@"+login)
			continue
		}
		name := html.EscapeString(u.Name)
		tag := fmt.Sprintf(`<mention class="mention" data-id="%d" data-type="user" data-text="@%s">@%s</mention>`, u.ID, name, name)
		text = text[:at] + tag + text[end:]
		m.Mentioned = appendOnce(m.Mentioned, u.Name)
	}
	return text, nil
}

// mentionUser finds the one user that @token names. Only admins see logins,
// so besides an exact login it accepts a first name, a last name or the full
// name without spaces (case and accents ignored) when exactly one candidate
// matches: a wrong guess would notify the wrong person.
func (e *Env) mentionUser(ctx context.Context, token string, m *mentions) (*mentionUser, error) {
	key := fold(token)
	if u, ok := m.users[key]; ok {
		return u, nil
	}
	f := NewFilters().Add("type", "=", "User").Add("any_name_attribute", "~", token)
	items, _, err := e.C.Collect(ctx, "/principals", url.Values{"filters": {f.JSON()}}, 50)
	if err != nil {
		return nil, err
	}
	var byLogin, byName []*mentionUser
	for _, it := range items {
		u := &mentionUser{ID: intOf(it["id"]), Name: fmt.Sprint(it["name"])}
		if l, _ := it["login"].(string); l != "" && fold(l) == key {
			byLogin = append(byLogin, u)
			continue
		}
		words := strings.Fields(fold(u.Name))
		if strings.Join(words, "") == key || containsStr(words, key) {
			byName = append(byName, u)
		}
	}
	var found *mentionUser
	switch {
	case len(byLogin) == 1:
		found = byLogin[0]
	case len(byLogin) == 0 && len(byName) == 1:
		found = byName[0]
	}
	m.users[key] = found
	return found, nil
}

var accents = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n", "à", "a", "è", "e", "ò", "o", "ç", "c")

func fold(s string) string { return accents.Replace(strings.ToLower(s)) }

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func appendOnce(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}
