# Frameserve

A “digital photo frame” slideshow served over the web.

- **No gallery**
- **No uploads**
- Photos come from a bind-mounted directory (`./photos` -> `/photos`)
- Locked down: only serves `/`, `/static/*`, `/api/photos`, `/photos/<filename>`, `/info`

## Run

Create a folder called `photos` next to `docker-compose.yaml` and drop images in it.

```bash
docker compose up -d --build
````

Open:

* [http://localhost:8080/](http://localhost:8080/)

## Supported image types

* jpg/jpeg
* png
* webp
* gif

## Simple authentication (optional, long-lived)

Frameserve supports a “set it and forget it” shared token.

1. Set `AUTH_TOKEN` (in your shell, `.env`, or compose environment).
2. On each device, open the slideshow once with the token in the URL:

   * `http://localhost:8080/?token=YOURTOKEN`

Frameserve will set a long-lived cookie (1 year) and redirect you to the same URL without the token. After that, the device stays logged in until cookies are cleared.

Also supported:

* `Authorization: Bearer YOURTOKEN` (useful for reverse proxies / curl)

`/healthz` is intentionally left unauthenticated.

## Optional UI query params

* `seconds=10` (advance interval)
* `shuffle=1` (random order)
* `fit=contain|cover`
* `hud=1` (show on-screen status)
* `order=mtime_desc|mtime_asc|name_asc|name_desc` (API ordering)
* `refresh=60` (how often to refresh the directory listing)
* `awake=1` (best-effort screen wake lock)

Example:

[http://localhost:8080/?seconds=15&shuffle=1&fit=cover&hud=1](http://localhost:8080/?seconds=15&shuffle=1&fit=cover&hud=1)

See also:

* [http://localhost:8080/info](http://localhost:8080/info)

---
