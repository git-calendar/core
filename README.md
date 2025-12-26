### Notes
- when importing to some frontend js framework, the `vite.config.ts` will probably need:
  ```js
  export default defineConfig({
    server: {
      headers: {
        "Cross-Origin-Opener-Policy": "same-origin",
        "Cross-Origin-Embedder-Policy": "require-corp",
      },
    },
  });
  ```

### Testing Wasm parts:
```sh
# 1. install lib
go install github.com/agnivade/wasmbrowsertest@latest

# 2.A. rename so that go test finds it automatically
mv "$(go env GOPATH)/bin/wasmbrowsertest" "$(go env GOPATH)/bin/go_js_wasm_exec"
# then run like so:
GOOS=js GOARCH=wasm go test ./filesystem

# 2.B. or run the tests with a flag
GOOS=js GOARCH=wasm go test -exec /Users/firu/go/bin/wasmbrowsertest ./filesystem
```
More info here: https://github.com/agnivade/wasmbrowsertest
