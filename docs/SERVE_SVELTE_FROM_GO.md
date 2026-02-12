# Serving a Svelte (Vite / SvelteKit) UI from Go — Best Practices

This document summarises recommended approaches for building, packaging and serving a Svelte (Vite or SvelteKit) front-end from a Go server.

## Recommendation

- Preferred: build a production static SPA (SvelteKit `adapter-static` with `fallback: index.html`) and serve the `build/` (or `dist/`) output from the Go app as static files. Use long cache TTLs for hashed assets and short/no-cache for `index.html`.
- Use SSR (adapter-node) only if you need server-side rendering for SEO or first-paint performance; otherwise prefer static or prerendered output.

## Build & Packaging (CI / Docker)

- Multi-stage Docker build pattern:
  1. Web build stage: `npm ci && npm run build` → produces `build/` (SvelteKit) or `dist/` (Vite).
  2. Copy `build/` into the Go image (e.g. `/internal/webui/dist`) before building or into the final image for runtime serving.
- CI: run the web build step first and persist artifacts, then build the Go binary that will serve them.

Example Dockerfile snippet (multi-stage):

```dockerfile
# web build
FROM node:20 AS web-build
WORKDIR /src/web
COPY web/ .
RUN npm ci && npm run build

# go build
FROM golang:1.21-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# copy web build into source tree so server can serve it
COPY --from=web-build /src/web/build /src/internal/webui/dist
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/nudged ./cmd/nudged

# final image
FROM alpine:3.18
COPY --from=build /out/nudged /usr/local/bin/nudged
COPY --from=web-build /src/web/build /internal/webui/dist
ENTRYPOINT ["/usr/local/bin/nudged","--serve"]
```

## Serving options & tradeoffs

- Static SPA (recommended): simple, low ops, can be embedded into the binary (`go:embed`) or served from disk (`http.FileServer`). Ensure SPA fallback returns `index.html` for unknown routes.
- Prerender (SvelteKit): prerender pages at build time when the route set is finite.
- SSR (adapter-node): run a Node process (or adapter/platform) for dynamic server-side rendering; reverse-proxy from Go if you want a single public ingress.

## Runtime best practices

- SPA fallback: unknown paths -> serve `index.html` to support client-side routing.
- Caching: set `Cache-Control: public, max-age=31536000, immutable` for fingerprinted assets; short/no-cache for `index.html` (e.g. `Cache-Control: no-cache`).
- Precompressed assets: produce `.br`/`.gz` during the web build (vite plugins) and serve precompressed files when `Accept-Encoding` allows it to save CPU.
- Compression: if not serving precompressed, enable gzip/Brotli at the HTTP layer.
- Security headers: add CSP, `X-Frame-Options`, `X-Content-Type-Options`, `Referrer-Policy`.
- Content-Type detection: prefer `http.FileServer` which uses mime types; set explicit headers if necessary.
- Base path / subpath: if serving under a path like `/ui/`, ensure the app's base is set or inject a `<base href="/ui/">` into `index.html`.
- ETag / If-Modified-Since: use for efficient client caching.

## Dev workflow

- Develop UI with `npm run dev` (Vite/SvelteKit). Use the dev server proxy to forward API requests to the Go backend so HMR works.
- In Docker dev, run web and Go containers side-by-side and configure hostnames or use `nip.io` for testing host-based routing.

## Minimal Go pattern (static SPA + SPA fallback)

1. Prefer serving from disk in production (copy built files into the image at `/internal/webui/dist`).
2. Optionally embed assets with `go:embed` if you want a single binary and deterministic build-time embedding.

Example: static handler with fallback and base-path support (pseudo):

```go
// serve static files and fallback to index.html for SPA routes
func MakeUIHandler(dist fs.FS, prefix string) http.Handler {
  fsys := http.FS(dist)
  fileServer := http.FileServer(fsys)

  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    // strip prefix and check if file exists
    p := strings.TrimPrefix(r.URL.Path, prefix)
    p = path.Clean(p)
    if p == "" || p == "/" {
      // serve index
      http.ServeFile(w, r, "/internal/webui/dist/index.html")
      return
    }
    // try open requested file
    if f, err := dist.Open(strings.TrimPrefix(p, "/")); err == nil {
      _ = f.Close()
      http.StripPrefix(prefix, fileServer).ServeHTTP(w, r)
      return
    }
    // fallback
    http.ServeFile(w, r, "/internal/webui/dist/index.html")
  })
}
```

Notes:
- If you use `go:embed`, embed the `build` output at compile time: `//go:embed web/build/*` and use `fs.Sub` to mount it.
- Building with `go:embed` requires the built files to exist in the source tree at compile time (CI or docker builder should copy them there before `go build`).

## Precompression serving (recommended)

- Use `vite` plugins such as `vite-plugin-compression` to write `.br`/`.gz` beside assets.
- At runtime, inspect `Accept-Encoding` and serve `.br` (Brotli) first, then `.gz`, adding `Content-Encoding` header and correct `Content-Type`.

## When to run SSR instead

- Use SSR (SvelteKit adapter-node) when you must render dynamic content server-side for SEO or optimized first-paint. Options:
  - Run the Node server and reverse-proxy from Go.
  - Use prerender for pages that can be static and only SSR the rest.

## Commands

Build web (local):
```bash
cd web
npm ci
npm run build
```

Build hub image (local):
```bash
docker compose build hub
docker compose up -d hub
```

## References

- SvelteKit docs: building, adapters, and `$app/paths`
- Vite docs: production build & asset hashing
- HTTP caching & compression best practices

---
File: `docs/SERVE_SVELTE_FROM_GO.md` — add to repo and CI as desired.
