### TODO

- [ ] core functionality
  - [x] storage format
  - [ ] indexing
    - [x] interval-btree?
    - [ ] smart indexing (index file)
  - [ ] repeating events
    - [ ] infinite repeat (ghost events)
    - [x] basic repetition
    - [x] update repeating
    - [x] remove repeating
      - pkg/core/core_events.go:139-143
      - pkg/core/core_events.go:359
    - [x] tests
  - [x] connect repeating event exceptions
    - exception needs to have uuid
    - time encoded inside uuidv8
  - [ ] better tests
  - [x] load repositories
- [ ] Undo function (git reset HEAD~1)
- [ ] iCalendar compatibility
  - [x] import
    - [x] one-time import into a Git-backed calendar
    - [x] URL calendar fetched on every load
  - [ ] export
    - to a file
    - idk about url
      - github-pages?
      - custom http file server?
- [x] encryption
  - [x] storing a key
  - values-only
  - deterministic? (same input <=> same output)
    - +good git diffs
    - -patterns across files can be found
- [ ] local notifications (managed by client)
  - core has some method like "fetch" for polling (15/30 min interval)
  - -push notifications (almost instant) need a backend

---

### Data Folder Structure

```
.git-calendar-data/
├── default/
│   ├── .git/
│   ├── events/
│   │   └── <UUID>.json
│   ├── tags/
│   │   └── <UUID>.json
│   ├── index.jsonl
│   └── index-rich.jsonl
├── shared/
│   ├── .git/
│   ├── events/
│   │   └── <UUID>.json
│   ├── tags/
│   │   └── <UUID>.json
│   ├── index.jsonl
│   └── index-rich.jsonl
├── imported
├── default.key
├── shared.readonly
└── shared.key
```


### Testing Web/Wasm parts:
(there are no wasm-only tests at this time, but could be useful)

Install [wasmbrowsertest](https://github.com/agnivade/wasmbrowsertest):
```sh
go install github.com/agnivade/wasmbrowsertest@latest
```
Run tests like so:
```sh
GOOS=js GOARCH=wasm go test -exec $(go env GOPATH)/bin/wasmbrowsertest ./pkg/... ./cmd/wasm
```
Or, if you don't wanna specify the `-exec`:
1. Rename the exacutable so that `go test` finds it automatically:
```sh
mv "$(go env GOPATH)/bin/wasmbrowsertest" "$(go env GOPATH)/bin/go_js_wasm_exec"
```
2. And then run tests like you normally would:
```sh
GOOS=js GOARCH=wasm go test ./pkg/... ./cmd/wasm
```
