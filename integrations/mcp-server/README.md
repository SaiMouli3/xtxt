# xtxt-mcp

An MCP server that hands agents the structure inside
[XTXT](https://github.com/SaiMouli3/xtxt) documents instead of making them
infer it from prose.

```sh
npx xtxt-mcp /path/to/notes
```

## Claude Code / Claude Desktop

```json
{
  "mcpServers": {
    "xtxt": {
      "command": "npx",
      "args": ["-y", "xtxt-mcp", "/absolute/path/to/your/notes"]
    }
  }
}
```

The single argument is a **document root**. Every path the tools accept is
resolved inside it and anything that escapes is refused — exposing a document
reader to an agent must not become a way to read the whole filesystem.

## Tools

| Tool | What it returns |
|---|---|
| `xtxt_list` | Every document, with title, heading count, task count, record count |
| `xtxt_extract` | One document's outline, tasks, records, links, media, code and text |
| `xtxt_tasks` | Every task across every document, filterable by `open_only` and `owner` |
| `xtxt_records` | Every record block of a given type — `decision`, `knowledge`, or one nobody has invented yet |
| `xtxt_search` | Case-insensitive search across prose **and record field values** |
| `xtxt_validate` | Syntax errors and semantic warnings for one document |
| `xtxt_render` | A document as HTML or plain text |

## Why this is different from reading the files

An agent given a folder of Markdown has to infer that "Ship the parser — in
progress, Subbu, due Aug 15" is a task with an owner and a due date. That
inference is a model call with a failure rate, and it silently gets worse as
the document grows.

Here it is a parse:

```json
{ "title": "Ship the reference parser", "status": "In Progress",
  "owner": "Subbu", "due": "2026-08-15", "done": false, "line": 20 }
```

`xtxt_tasks` across a whole vault is one call, and nothing is missed or guessed.

Field names carry no meaning at the format level — `@experiment` with a
`Confidence` field works without anyone updating this server, which is the same
guarantee the parser makes.

## Tests

```sh
npm test
```

Drives the server over a real stdio transport, the way a client would.
