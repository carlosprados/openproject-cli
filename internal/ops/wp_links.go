package ops

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/carlosprados/openproject-cli/internal/hal"
)

// Relations, watchers and attachments of work packages.

func init() {
	registerGroup(&Group{Name: "wp relation", Aliases: []string{"relations", "rel"}, Short: "Relations between work packages (follows, blocks, relates...)"})
	registerGroup(&Group{Name: "wp watcher", Aliases: []string{"watchers"}, Short: "Users watching a work package"})
	registerGroup(&Group{Name: "wp attachment", Aliases: []string{"attachments", "att"}, Short: "Files attached to a work package"})

	register(relList, relCreate, relDelete, watchList, watchAdd, watchRemove, attList, attUpload, attDownload, attDelete)
}

var relationTypes = []string{"relates", "duplicates", "duplicated", "blocks", "blocked", "precedes", "follows", "includes", "partof", "requires", "required"}

var relList = &Op{
	Tool:    "op_list_relations",
	Cmd:     []string{"wp", "relation", "list"},
	Aliases: []string{"ls"},
	Short:   "List relations of a work package",
	Example: `  opcli wp relation list 42`,
	Params:  []Param{pID("Work package")},
	Columns: []Column{{Header: "ID", Key: "id"}, {Header: "TYPE", Key: "type"}, {Header: "WP", Key: "workPackageId"}, {Header: "SUBJECT", Key: "subject"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id, err := WorkPackageID(a.String("id"))
		if err != nil {
			return nil, err
		}
		f := NewFilters().Add("involved", "=", id)
		items, total, err := e.C.Collect(ctx, "/relations", url.Values{"filters": {f.JSON()}}, 0)
		if err != nil {
			return nil, err
		}
		return NewList(compactRelations(hal.FlattenAll(items), id), total), nil
	},
}

var relCreate = &Op{
	Tool:    "op_create_relation",
	Cmd:     []string{"wp", "relation", "add"},
	Aliases: []string{"create"},
	Short:   "Relate two work packages",
	Long: `Create a relation "<id> <type> <to>". For example "12 follows 10" means #12
starts after #10 finishes; "12 blocks 10" means #10 cannot be closed before
#12. For parent/child use 'opcli wp update <child> --parent <id>'.`,
	Example: `  opcli wp relation add 12 --type follows --to 10
  opcli wp relation add 12 --type relates --to 30 --description "same root cause"
  opcli wp relation add 12 --type precedes --to 13 --lag 2`,
	Params: []Param{
		pID("Work package (from)"),
		{Name: "to", Kind: String, Required: true, Desc: "Related work package id"},
		{Name: "type", Kind: String, Default: "relates", Enum: relationTypes, Desc: "Relation type: " + strings.Join(relationTypes, ", ")},
		{Name: "lag", Kind: Int, Desc: "Working days between the two (follows/precedes only)"},
		{Name: "description", Kind: String, Desc: "Optional description"},
	},
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		from, err := WorkPackageID(a.String("id"))
		if err != nil {
			return nil, err
		}
		to, err := WorkPackageID(a.String("to"))
		if err != nil {
			return nil, err
		}
		body := map[string]any{
			"type":   strings.ToLower(a.String("type")),
			"_links": map[string]any{"to": hal.Ref(href("work_packages", to))},
		}
		if a.Has("lag") {
			body["lag"] = a.Int("lag")
		}
		if d := a.String("description"); d != "" {
			body["description"] = d
		}
		res, err := e.C.Post(ctx, fmt.Sprintf("/work_packages/%d/relations", from), body)
		if err != nil {
			return nil, err
		}
		return hal.Flatten(res), nil
	},
}

var relDelete = &Op{
	Tool:    "op_delete_relation",
	Cmd:     []string{"wp", "relation", "delete"},
	Aliases: []string{"rm"},
	Short:   "Delete a relation by its id (see 'wp relation list')",
	Example: `  opcli wp relation delete 7`,
	Params:  []Param{pID("Relation")},
	Safety:  Destructive,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id := a.String("id")
		if err := e.C.Delete(ctx, "/relations/"+url.PathEscape(id)); err != nil {
			return nil, err
		}
		return &Message{OK: true, Message: "Deleted relation " + id}, nil
	},
}

var watchList = &Op{
	Tool:    "op_list_watchers",
	Cmd:     []string{"wp", "watcher", "list"},
	Aliases: []string{"ls"},
	Short:   "List watchers of a work package",
	Example: `  opcli wp watcher list 42`,
	Params:  []Param{pID("Work package")},
	Columns: []Column{{Header: "ID", Key: "id"}, {Header: "NAME", Key: "name"}, {Header: "LOGIN", Key: "login"}, {Header: "EMAIL", Key: "email"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id, err := WorkPackageID(a.String("id"))
		if err != nil {
			return nil, err
		}
		return e.list(ctx, fmt.Sprintf("/work_packages/%d/watchers", id), nil, Args{})
	},
}

var watchAdd = &Op{
	Tool:  "op_add_watcher",
	Cmd:   []string{"wp", "watcher", "add"},
	Short: "Add a watcher to a work package",
	Example: `  opcli wp watcher add 42 --user me
  opcli wp watcher add 42 --user jane.doe`,
	Params: []Param{
		pID("Work package"),
		{Name: "user", Kind: String, Required: true, Desc: "me, or login/email/name/id"},
	},
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id, err := WorkPackageID(a.String("id"))
		if err != nil {
			return nil, err
		}
		uid, err := e.UserID(ctx, a.String("user"))
		if err != nil {
			return nil, err
		}
		if _, err := e.C.Post(ctx, fmt.Sprintf("/work_packages/%d/watchers", id), map[string]any{"user": hal.Ref(href("users", uid))}); err != nil {
			return nil, err
		}
		return &Message{OK: true, Message: fmt.Sprintf("User %d now watches #%d", uid, id)}, nil
	},
}

var watchRemove = &Op{
	Tool:    "op_remove_watcher",
	Cmd:     []string{"wp", "watcher", "remove"},
	Aliases: []string{"rm"},
	Short:   "Remove a watcher from a work package",
	Example: `  opcli wp watcher remove 42 --user me`,
	Params: []Param{
		pID("Work package"),
		{Name: "user", Kind: String, Required: true, Desc: "me, or login/email/name/id"},
	},
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id, err := WorkPackageID(a.String("id"))
		if err != nil {
			return nil, err
		}
		uid, err := e.UserID(ctx, a.String("user"))
		if err != nil {
			return nil, err
		}
		if err := e.C.Delete(ctx, fmt.Sprintf("/work_packages/%d/watchers/%d", id, uid)); err != nil {
			return nil, err
		}
		return &Message{OK: true, Message: fmt.Sprintf("User %d no longer watches #%d", uid, id)}, nil
	},
}

var attList = &Op{
	Tool:    "op_list_attachments",
	Cmd:     []string{"wp", "attachment", "list"},
	Aliases: []string{"ls"},
	Short:   "List attachments of a work package",
	Example: `  opcli wp attachment list 42`,
	Params:  []Param{pID("Work package")},
	Columns: []Column{{Header: "ID", Key: "id"}, {Header: "FILE", Key: "fileName"}, {Header: "SIZE", Key: "fileSize"}, {Header: "TYPE", Key: "contentType"}, {Header: "AUTHOR", Key: "author"}, {Header: "CREATED", Key: "createdAt"}},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id, err := WorkPackageID(a.String("id"))
		if err != nil {
			return nil, err
		}
		l, err := e.list(ctx, fmt.Sprintf("/work_packages/%d/attachments", id), nil, Args{})
		if err != nil {
			return nil, err
		}
		l.Items = compactAttachments(l.Items)
		return l, nil
	},
}

var attUpload = &Op{
	Tool:  "op_upload_attachment",
	Cmd:   []string{"wp", "attachment", "upload"},
	Short: "Attach a local file to a work package",
	Long: `Upload a file. With MCP the path is read on the machine running the
opcli MCP server, not on the client.`,
	Example: `  opcli wp attachment upload 42 --file ./screenshot.png
  opcli wp attachment upload 42 --file build.log --description "CI output"`,
	Params: []Param{
		pID("Work package"),
		{Name: "file", Kind: String, Required: true, Desc: "Path of the file to upload"},
		{Name: "description", Kind: String, Desc: "Optional description"},
	},
	Safety: Write,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id, err := WorkPackageID(a.String("id"))
		if err != nil {
			return nil, err
		}
		res, err := e.C.Upload(ctx, fmt.Sprintf("/work_packages/%d/attachments", id), a.String("file"), a.String("description"))
		if err != nil {
			return nil, err
		}
		return compactAttachments([]map[string]any{hal.Flatten(res)})[0], nil
	},
}

var attDownload = &Op{
	Tool:  "op_download_attachment",
	Cmd:   []string{"wp", "attachment", "download"},
	Short: "Download an attachment by its id",
	Long: `Download an attachment (ids come from 'wp attachment list' or 'wp get').
The file is written to --dest (default: its original name in the current
directory). With --stdout text content is returned instead of saved, which
is how an AI agent can read a text attachment.`,
	Example: `  opcli wp attachment download 15
  opcli wp attachment download 15 --dest /tmp/log.txt
  opcli wp attachment download 15 --stdout | less`,
	Params: []Param{
		pID("Attachment"),
		{Name: "dest", Kind: String, Desc: "Destination path (default: original file name)"},
		{Name: "stdout", Kind: Bool, Desc: "Return the content instead of writing a file (text files only)"},
	},
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id := a.String("id")
		meta, err := e.C.Get(ctx, "/attachments/"+url.PathEscape(id), nil)
		if err != nil {
			return nil, err
		}
		loc := hal.Link(meta, "downloadLocation")
		if loc == "" {
			return nil, fmt.Errorf("attachment %s has no download location", id)
		}
		data, err := e.C.Download(ctx, loc)
		if err != nil {
			return nil, err
		}
		if a.Bool("stdout") {
			ct, _ := meta["contentType"].(string)
			if !isTextual(ct, data) {
				return nil, fmt.Errorf("attachment %s is binary (%s); download it to a file instead", id, ct)
			}
			return string(data), nil
		}
		out := a.String("dest")
		if out == "" {
			out = filepath.Base(fmt.Sprint(meta["fileName"]))
		}
		if err := os.WriteFile(out, data, 0o644); err != nil {
			return nil, err
		}
		return &Message{OK: true, Message: fmt.Sprintf("Saved %s (%d bytes)", out, len(data))}, nil
	},
}

func isTextual(contentType string, data []byte) bool {
	if strings.HasPrefix(contentType, "text/") || strings.Contains(contentType, "json") || strings.Contains(contentType, "xml") || strings.Contains(contentType, "yaml") {
		return true
	}
	for _, b := range data[:min(len(data), 4096)] {
		if b == 0 {
			return false
		}
	}
	return contentType == "" || contentType == "application/octet-stream"
}

var attDelete = &Op{
	Tool:    "op_delete_attachment",
	Cmd:     []string{"wp", "attachment", "delete"},
	Aliases: []string{"rm"},
	Short:   "Delete an attachment by its id",
	Example: `  opcli wp attachment delete 15`,
	Params:  []Param{pID("Attachment")},
	Safety:  Destructive,
	Run: func(ctx context.Context, e *Env, a Args) (any, error) {
		id := a.String("id")
		if err := e.C.Delete(ctx, "/attachments/"+url.PathEscape(id)); err != nil {
			return nil, err
		}
		return &Message{OK: true, Message: "Deleted attachment " + id}, nil
	},
}
