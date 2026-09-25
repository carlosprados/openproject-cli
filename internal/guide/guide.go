// Package guide holds the usage guide shared by `opcli guide`, the MCP
// server instructions and the openproject://guide resource.
package guide

import _ "embed"

//go:embed guide.md
var Text string

// Instructions is the short version sent to MCP clients on initialize.
const Instructions = `OpenProject tools (opcli). Conventions:
- References are resolved by name: projects by id/identifier/name, users by me/login/email/name/id ("none" unassigns), statuses/types/priorities by name.
- Dates accept YYYY-MM-DD, today, yesterday, week, month, +Nd/-Nd; durations accept 1.5, 90m, 1h30m, PT1H30M.
- Before changing a status call op_list_work_package_allowed_values (field "status"): workflows restrict transitions.
- Updates only touch the parameters you pass. Descriptions/comments are Markdown.
- List results are {total,count,items}; total > count means more pages (raise max or use page).
- op_delete_* tools are irreversible: confirm with the user first.
- For endpoints without a dedicated tool: op_api_list_endpoints → op_api_describe_endpoint → op_api_request.
- Full guide: resource openproject://guide.`
