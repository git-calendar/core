# Git Calendar Cors Proxy
### for git-calendar-core to run in the browser.

## Build and run
```sh
go run ./cors-proxy
```
```sh
go build -o ./build/cors-proxy ./cors-proxy
```

## Usage
Normal HTTP request from the browser:
```js
const response = await fetch("https://example.com");
const html = await response.text();
console.log(html);
```
results in:
```
Access to fetch at 'https://example.com' from origin 'https://...' has been blocked by CORS policy: No 'Access-Control-Allow-Origin' header is present on the requested resource.
```

---

HTTP request through this proxy:

```js
const response = await fetch("http://localhost:8000/?url=https://example.com");
const html = await response.text();
console.log(html);
```
succeds!
```
<!doctype html><html lang="en"><head><tit...
```
