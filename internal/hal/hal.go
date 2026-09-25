// Package hal turns OpenProject HAL+JSON resources into flat, predictable
// maps that are pleasant for humans (tables) and cheap for LLMs (tokens).
//
// Flattening rules, applied to every resource type alike:
//   - scalar properties are kept as-is;
//   - Formattable values ({format, raw, html}) collapse to their raw text;
//   - each _links entry "foo" with a title becomes "foo": "<title>" plus
//     "fooId": <id parsed from href>; link arrays become lists;
//   - action links (those carrying a "method"), templated links and
//     bookkeeping links (self, schema, ...) are dropped;
//   - _embedded is dropped except for keys listed in keepEmbedded.
package hal

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Links that carry no information for a consumer of the flattened record.
var skipLinks = map[string]bool{
	"self": true, "schema": true, "ancestors": true, "attachments": true,
	"activities": true, "availableWatchers": true, "watchers": true,
	"relations": true, "revisions": true, "timeEntries": true, "fileLinks": true,
	"customActions": true, "availableRelationCandidates": true, "atom": true,
	"pdf": true, "generate_pdf": true, "github_pull_requests": true,
	"gitlab_issues": true, "gitlab_merge_requests": true, "meetings": true,
	"showCosts": true, "previewMarkup": true, "staticDownloadLocation": true,
	"memberships": true, "workPackages": true, "categories": true,
	"versions": true, "types": true, "children": true, "availableProjects": true,
	"availableInProjects": true, "emojiReactions": true,
	"configureForm": true, "copy": true, "customFields": true, "logTime": true,
	"move": true, "showUser": true, "addAttachment": true, "addComment": true,
	"addRelation": true, "addWatcher": true, "removeWatcher": true, "addChild": true,
	"changeParent": true, "watch": true, "unwatch": true, "update": true,
	"updateImmediately": true, "delete": true, "lock": true, "unlock": true,
}

// Flatten converts one HAL resource into a flat map (see package doc).
func Flatten(res map[string]any, keepEmbedded ...string) map[string]any {
	out := make(map[string]any, len(res))
	for k, v := range res {
		switch k {
		case "_links", "_embedded", "_type":
			continue
		}
		out[k] = flattenValue(v)
	}
	if t, ok := res["_type"].(string); ok {
		out["_type"] = t
	}
	if links, ok := res["_links"].(map[string]any); ok {
		for name, l := range links {
			if skipLinks[name] {
				continue
			}
			switch lv := l.(type) {
			case map[string]any:
				if isAction(lv) {
					continue
				}
				addLink(out, name, lv)
			case []any:
				var titles, ids []any
				for _, item := range lv {
					m, ok := item.(map[string]any)
					if !ok || isAction(m) {
						continue
					}
					if t, ok := m["title"].(string); ok {
						titles = append(titles, t)
					}
					if id := IDFromHref(str(m["href"])); id != nil {
						ids = append(ids, id)
					}
				}
				if len(titles) > 0 {
					out[name] = titles
				}
				if len(ids) > 0 {
					out[name+"Ids"] = ids
				}
			}
		}
	}
	if emb, ok := res["_embedded"].(map[string]any); ok {
		for _, key := range keepEmbedded {
			switch ev := emb[key].(type) {
			case map[string]any:
				if els, ok := ev["_embedded"].(map[string]any); ok {
					out[key] = FlattenAll(toMaps(els["elements"]))
				} else {
					out[key] = Flatten(ev)
				}
			case []any:
				out[key] = FlattenAll(toMaps(ev))
			}
		}
	}
	return out
}

// FlattenAll flattens a list of HAL resources.
func FlattenAll(items []map[string]any, keepEmbedded ...string) []map[string]any {
	out := make([]map[string]any, len(items))
	for i, it := range items {
		out[i] = Flatten(it, keepEmbedded...)
	}
	return out
}

func addLink(out map[string]any, name string, l map[string]any) {
	href := str(l["href"])
	if href == "" || l["templated"] == true {
		// A null href means "not set" (e.g. unassigned): expose it explicitly.
		if _, exists := l["href"]; exists && href == "" {
			out[name] = nil
		}
		return
	}
	t, titled := l["title"].(string)
	id := IDFromHref(href)
	if _, numeric := id.(int); !numeric && !titled {
		return // untitled sub-collection link such as .../emoji_reactions
	}
	if titled {
		out[name] = t
	}
	if id != nil {
		out[name+"Id"] = id
	} else if !titled {
		out[name] = href
	}
}

func isAction(l map[string]any) bool {
	m, ok := l["method"].(string)
	return ok && !strings.EqualFold(m, "get")
}

func flattenValue(v any) any {
	if m, ok := v.(map[string]any); ok {
		if raw, ok := m["raw"]; ok {
			if _, fmtOK := m["format"]; fmtOK {
				return raw
			}
		}
	}
	return v
}

// IDFromHref returns the trailing id segment of an API href: an int when
// numeric, the segment string for identifiers like "me" or "2024-01-01",
// or nil for non-resource hrefs.
func IDFromHref(href string) any {
	if href == "" || !strings.HasPrefix(href, "/api/v3/") {
		return nil
	}
	href, _, _ = strings.Cut(href, "?")
	seg := href[strings.LastIndex(href, "/")+1:]
	if seg == "" {
		return nil
	}
	if n, err := strconv.Atoi(seg); err == nil {
		return n
	}
	return seg
}

// Link returns the href of a named _links entry of res.
func Link(res map[string]any, name string) string {
	links, _ := res["_links"].(map[string]any)
	l, _ := links[name].(map[string]any)
	return str(l["href"])
}

// Ref builds a HAL link object for request payloads; an empty href means
// "unset" and serialises as {"href": null}.
func Ref(href string) map[string]any {
	if href == "" {
		return map[string]any{"href": nil}
	}
	return map[string]any{"href": href}
}

// Text builds a Formattable payload.
func Text(raw string) map[string]any {
	return map[string]any{"raw": raw}
}

func toMaps(v any) []map[string]any {
	raw, _ := v.([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// String renders any flattened value as a single-line string for tables.
func String(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case bool:
		return strconv.FormatBool(x)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = String(e)
		}
		return strings.Join(parts, ", ")
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}
