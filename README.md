# Frameserve 📸

**A digital photo frame… but served over the web.**

Frameserve turns a folder of photos into a clean, full-screen slideshow you can open in any browser — TVs, tablets, old laptops, wall displays, kiosks, you name it.

No galleries.  
No uploads.  
No clutter.

Just photos, one at a time, like a real digital photo frame.

---

## What is Frameserve?

Frameserve is a small, self-hosted web app that:

- Reads photos from a directory on your machine
- Displays them **one-by-one** in a looping slideshow
- Runs entirely inside a Docker container
- Works great on “set it and forget it” devices

Think of it as:

> *“A cloud photo frame — except you own it.”*

---

## What it deliberately does **not** do

This is a design choice, not a limitation:

- ❌ No gallery view
- ❌ No thumbnails
- ❌ No web uploads
- ❌ No file management UI
- ❌ No database

The **photos folder is the source of truth**.  
If you can add files to that folder, Frameserve will show them.

---

## Quick start (the friendly version)

### 1️⃣ Put your photos somewhere

Create a folder called `photos` and drop images into it:

```

photos/
vacation.jpg
family.png
dog.webp

````

Supported formats:
- JPG / JPEG
- PNG
- WebP
- GIF

---

### 2️⃣ Run Frameserve with Docker

Here’s the simplest `docker-compose.yaml`:

```yaml
services:
  frameserve:
    image: davidhfrankelcodes/frameserve:latest
    restart: unless-stopped
    ports:
      - "8080:80"
    volumes:
      - ./photos:/photos:ro
````

Then run:

```bash
docker compose up -d
```

---

### 3️⃣ Open it in a browser

Go to:

👉 **[http://localhost:8080/](http://localhost:8080/)**

That’s it.
Your slideshow should start immediately.

---

## Using it like a real photo frame

Frameserve is designed for devices that just sit there and show photos.

### Keyboard shortcuts (optional)

If you’re on a keyboard-enabled device:

* **Space** — pause / resume
* **← / →** — previous / next photo
* **F** — fullscreen
* **H** — toggle on-screen HUD

---

## Customizing the slideshow (no settings screen needed)

Everything is controlled via the URL.

Example:

```
http://localhost:8080/?seconds=15&shuffle=1&fit=cover
```

### Common options

| Option                      | What it does                                 |
| --------------------------- | -------------------------------------------- |
| `seconds=15`                | Time each photo stays on screen              |
| `shuffle=1`                 | Random photo order                           |
| `fit=contain` / `fit=cover` | Letterbox vs full-bleed                      |
| `hud=1`                     | Show on-screen status                        |
| `refresh=60`                | How often to re-scan the photos folder       |
| `awake=1`                   | Best-effort request to keep the screen awake |

📌 Tip: Bookmark your favorite URL once and never touch it again.

---

## Simple authentication (optional)

Frameserve supports **long-lived, low-friction access control** — ideal for TVs and wall displays.

### How it works

1. Set a shared token via environment variable:

   ```bash
   AUTH_TOKEN=some-long-random-string
   ```

2. On a new device, open once with:

   ```
   http://your-server/?token=YOURTOKEN
   ```

3. Frameserve stores a **1-year cookie** and redirects you to a clean URL.

After that, the device stays logged in until cookies are cleared.

No logins.
No sessions to babysit.
No user accounts.

---

## Why this exists (design philosophy)

Frameserve was built with a few strong opinions:

* The filesystem is already a great UI
* Digital photo frames shouldn’t need cloud accounts
* TVs and tablets deserve simple software
* Containers should be small, locked down, and boring
* The best config screen is the browser’s address bar

It’s intentionally minimal — but carefully thought through.

---

## Endpoints (for the curious)

You don’t need these, but they exist:

* `/` — slideshow
* `/info` — usage help
* `/api/photos` — JSON list of images
* `/photos/<filename>` — serves image bytes
* `/healthz` — health check (no auth)

---

## Perfect use cases

* Wall-mounted tablet
* TV browser app
* Old laptop on a shelf
* Photo display at an event
* Office lobby screen
* Self-hosted “family frame”

---

## License & status

This project is intentionally small, stable, and done-on-purpose.

If you want to extend it — great.
If you want to fork it — even better.

---

**Frameserve**
*A digital photo frame that respects your time, your files, and your attention.*
