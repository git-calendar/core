### TODO
- [ ] core funtionality
  - [ ] storage format
  - [ ] indexing
    - interval-btree?
- [ ] iCalendar compatibility
  - [ ] import (periodical & one-time)
  - [ ] export
    - to a file
    - idk about url
      - github-pages? (cant be encrypted ig)
      - custom http file server?
- [ ] encryption
  - [ ] storing a key (in opfs?)
  - values-only
  - deterministic? (same input <=> same output)
    - +good git diffs
    - -patterns across files can be found
- [ ] local notifications (managed by client)
  - core has some method like "fetch" for polling (15/30 min interval)
  - -push notifications (almost instant) need a backend

---

### Repo Structure (prototype)
```
.git-calendar-data
├── events
│   └── <UUID>.json
├── index.jsonl
└── index-rich.jsonl
```

### Encryption
- Why?
> to hide data when using a public GitHub/GitLab/Gitea/Codeberg instance as a git remote

- values-only encryption might be best for this
  - json structure stays
    - git diffs work well (unlike whole file encryption)
  - something like this:
  ```json
  {
    "title": "Meeting",
    "location": "https://zoom.us/my/abcd",
    "from": "2011-10-05T14:48:00.000Z",
    ...
  }
            |         - something like AES-SIV or XChaCha20 encryption
            v         - base64 representation
  {
    "title": "/sNrzDJP/K1mmAI6LkBOk3Rv4+JeQQ==",
    "location": "29J6yCgbtHpeoPCr6pRB9Z8yrmNswW4n5xOFRK1IvGwduFtkljE=",
    "from": "gZY/iXYQq3gU+sv38NsG4sh7sSw+kjqMttCEhnT8HQ9orN/XIGsg",
    ...
  }
  ```

Workflow:
1. pull changes to disk (FileSystem) (should work all right with this approach)
2. decrypt data (using a key stored somewhere) to a separate folder on disk
3. make changes
4. encrypt repository
5. push to remote
