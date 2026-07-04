# Git Calendar Core
[![Go version](https://img.shields.io/github/go-mod/go-version/git-calendar/core)](./go.mod)
[![Go](https://github.com/git-calendar/core/actions/workflows/go.yaml/badge.svg)](https://github.com/git-calendar/core/actions/workflows/go.yaml)
[![License](https://img.shields.io/github/license/git-calendar/core)](./LICENSE.txt)

A shared core for Git-backed calendar.

Related projects:
- [CORS proxy](./cmd/cors-proxy)
- Web client: [git-calendar/web-client](https://github.com/git-calendar/web-client)

## Build
Install [gomobile](https://pkg.go.dev/golang.org/x/mobile/cmd/gomobile) for Android and iOS builds:
```sh
go install golang.org/x/mobile/cmd/gomobile@latest
```

Build a target:
```sh
make build_android
make build_ios
make build_web
```
