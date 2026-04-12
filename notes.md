### TODO

- [ ] core functionality
  - [x] storage format
  - [ ] indexing
    - [x] interval-btree?
    - [ ] smart indexing (index file)
  - [x] repeating events
    - infinite shadow (ghost events)
  - [x] connect repeating event exceptions
    - exception needs to have uuid
    - time encoded inside uuidv8
  - [ ] config file per repo
    - tags
  - [ ] better tests
  - [x] load repositories
- [ ] iCalendar compatibility
  - [ ] import (periodical & one-time)
  - [ ] export
    - to a file
    - idk about url
      - github-pages?
      - custom http file server?
- [x] encryption
  - [x] storing a key (in opfs?)
  - values-only
  - deterministic? (same input <=> same output)
    - +good git diffs
    - -patterns across files can be found
- [ ] local notifications (managed by client)
  - core has some method like "fetch" for polling (15/30 min interval)
  - -push notifications (almost instant) need a backend

---

### Folder Structure

```
.git-calendar-data/
├── main/
│   ├── .git/
│   ├── events/
│   │   └── <UUID>.json
│   ├── index.jsonl
│   └── index-rich.jsonl
├── shared/
│   ├── .git/
│   ├── events/
│   │   └── <UUID>.json
│   ├── index.jsonl
│   └── index-rich.jsonl
├── main.key
└── shared.key
```
