# Git Calendar Core
[![Go Report Card](https://goreportcard.com/badge/github.com/firu11/git-calendar-core)](https://goreportcard.com/report/github.com/firu11/git-calendar-core)


Related projects:
- [CORS proxy](./cmd/cors-proxy)
- Web client: [git-calendar-web](https://github.com/firu11/git-calendar-web)

### Building
For Android and IOS bindings, make sure to install [gomobile](https://pkg.go.dev/golang.org/x/mobile/cmd/gomobile):
```sh
go install golang.org/x/mobile/cmd/gomobile@latest
```
You can build for all platforms using:
```sh
make
```
or individually:
```sh
make [build_android|build_ios|build_web]
```

### Testing Web/Wasm parts:
Install [wasmbrowsertest](https://github.com/agnivade/wasmbrowsertest):
```sh
go install github.com/agnivade/wasmbrowsertest@latest
```
Run tests like so:
```sh
GOOS=js GOARCH=wasm go test -exec $(go env GOPATH)/bin/wasmbrowsertest ./...
```
Or, if you don't wanna specify the `-exec`:
1. Rename the exacutable so that `go test` finds it automatically:
```sh
mv "$(go env GOPATH)/bin/wasmbrowsertest" "$(go env GOPATH)/bin/go_js_wasm_exec"
```
2. And then run tests like you normally would:
```sh
GOOS=js GOARCH=wasm go test ./...
```
