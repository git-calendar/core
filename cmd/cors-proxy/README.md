# Git Calendar Cors Proxy
A simple proxy that adds the [CORS](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CORS) headers to every request.\
It’s used as a workaround for browser security restrictions when accessing third-party services like GitHub, GitLab, Codeberg, etc., which we don’t control.

When using a [bare Git repository](https://git-scm.com/book/en/v2/Git-on-the-Server-Getting-Git-on-a-Server) on a [VPS](https://en.wikipedia.org/wiki/Virtual_private_server) this proxy is not necessary. 
You can use any [reverse-proxy](https://en.wikipedia.org/wiki/Reverse_proxy) of your choice (e.g. [Caddy](https://caddyserver.com/) or [Nginx](https://nginx.org/en/)), and just add the CORS headers directly.\
Example configuration for Caddy:\
(based on [this](https://www.jamesatkins.com/posts/git-over-http-with-caddy/) very cool article)
```caddyfile
your-repo-domain.com {
    # CORS setup (wildcards for origin, headers etc. often fail with credentials)
    header {
        Access-Control-Allow-Origin  "https://calendar-web-domain.com" # TODO
        Access-Control-Allow-Methods "GET, POST, OPTIONS" # git uses these HTTP methods
        Access-Control-Allow-Headers "Authorization, Content-Type, Git-Protocol" # git uses these headers
        Access-Control-Expose-Headers "Content-Length, Content-Range, Git-Protocol" # let client Wasm see those headers
        Access-Control-Allow-Credentials "true" # required for Basic Auth to work
    }

    # Handle preflight (OPTIONS) requests
    @options {
        method OPTIONS
    }
    respond @options 204 # No Content

    # Authentication
    basic_auth / {
        # generate password hash with 'caddy hash-password'
        your_vps_user_name $2a$14$Zkx19XLiW6VYouLHR5NmfOFU0z2GTNmpkT/5qqR7hx4IjWJPDhjvG
    }

    # Route to git-http-backend
    @git_cgi path_regexp "^.*/(HEAD|info/refs|objects/info/[^/]+|git-upload-pack|git-receive-pack)$"
    @git_static path_regexp "^.*/objects/([0-9a-f]{2}/[0-9a-f]{38}|pack/pack-[0-9a-f]{40}\.(pack|idx))$"
    vars git_dir /srv/git # or /home/git which contains repos like my_project.git/
    # make sure caddy and fcgiwrap have the right permissions to this directory
    # make sure selinux doesnt restrict the access
    
    handle @git_cgi {
        reverse_proxy unix//run/fcgiwrap.socket { # You will need fcgiwrap installed on your VPS
            transport fastcgi {
                env SCRIPT_FILENAME /usr/libexec/git-core/git-http-backend # depends on distro; find the executable by `find /usr -name "git-http-backend"`
                env GIT_HTTP_EXPORT_ALL 1
                env GIT_PROJECT_ROOT {vars.git_dir}
            }
        }
    }
    
    handle @git_static {
        file_server {
            root {vars.git_dir}
        }
    }
}
```

## Build and run
```sh
go run ./cmd/cors-proxy
```
```sh
go build -o ./build/cors-proxy ./cmd/cors-proxy
```

### Enviroment variables (optional; those values are default)
```sh
CORS_PROXY_HOST=0.0.0.0
CORS_PROXY_PORT=8000
CORS_PROXY_PRODUCTION=false
CORS_PROXY_UPSTREAM_TIMEOUT=15s
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

### TODO 
- rate-limiter (redis? for multi instance deployment)
