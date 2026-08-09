# CLAUDE.md

> # 🟩 LIGNE HYDRACKER-ONLY — versions 1.x
>
> | | |
> |---|---|
> | Dossier | `/Users/gandalf/go-post-tools-hydracker` |
> | Repo | `Gandalfleblanc/Go-Post-Tools-Hydracker` |
> | Tags | `vX.Y.Z` (1.x) |
> | Public | utilisateurs qui n'ont **que** Hydracker |
> | Cross-post Elysium | **désactivé** (`elysiumCrossPostEnabled = false`) |
>
> L'autre ligne vit dans `/Users/gandalf/go-post-tools` (versions **8.x**, avec
> Elysium). Les deux dossiers sont indépendants : ne jamais ajouter ici de remote
> vers l'autre repo, ni de branche de l'autre ligne.
>
> **Divergences à conserver lors d'un report de correctif depuis la ligne 8.x :**
> `elysiumCrossPostEnabled = false` · URLs updater et `teamListURL` pointées sur
> `Go-Post-Tools-Hydracker` · aucune UI Elysium dans `App.svelte` /
> `HydrackerTab.svelte`.

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Wails v2 desktop app (Go backend + Svelte frontend) for uploading content to Hydracker via three workflows : DDL (1Fichier + Send.now), NZB (ParPar → Nyuu → Hydracker), Torrent (FTP → create → Hydracker → ruTorrent seedbox).

## Common commands

```bash
# Dev mode (hot reload frontend)
~/go/bin/wails dev

# Production build (current platform)
~/go/bin/wails build -platform darwin/arm64

# Production build with DevTools (right-click → Inspect)
~/go/bin/wails build -platform darwin/arm64 -devtools

# Run the built app from terminal (to see Go stdout/stderr logs)
./build/bin/Go-Post-Tools.app/Contents/MacOS/Go-Post-Tools

# Linux build requires the webkit2_41 tag (Ubuntu 24.04 ships webkit2gtk 4.1)
wails build -platform linux/amd64 -tags webkit2_41
```

CI is `.github/workflows/build.yml` — triggered by `git push origin vX.Y.Z`. Builds 4 platforms (macOS arm64/x64, Linux amd64, Windows amd64) and publishes a GitHub Release.

## Architecture

### Wails bridge
Methods on `*App` in `app.go` are exposed to the frontend via auto-generated bindings in `frontend/wailsjs/go/main/`. Frontend calls them with `import { FnName } from '../wailsjs/go/main/App.js'`. The bindings are regenerated on every `wails build` — **don't edit them manually**.

### Real-time progress
Long-running workflows stream progress to Svelte via `wailsruntime.EventsEmit(ctx, "name", data)` on the Go side, and `EventsOn("name", cb)` on the JS side. Naming: `nzb:status`, `nzb:parpar`, `nzb:nyuu`, `ddl:log`, `ddl:progress`, `ddl:done`, `ddl:posting`, `torrent:status`, `torrent:ftp`, `torrent:create`, `torrent:seedbox`. All events are also routed to the **Journal** tab via the shared store `frontend/src/logs.js`.

### Workflows
Each workflow is a single method on `*App` that orchestrates multiple modules in `internal/` and emits events for each step:
- `PostDDLWorkflow` : parallel upload (goroutines + WaitGroup) to 1Fichier + Send.now, then sequential `UploadLien` to Hydracker
- `PostNzbWorkflow` : ParPar → collect par2 files → Nyuu → cleanup par2 → `UploadNzb`
- `PostTorrentWorkflow` : ftpup → torrent.Create (sets `private=1`, info_hash recomputed by server) → `UploadTorrent` → download Hydracker-modified .torrent via `/api/v1/torrents/{id}/download` (Bearer required) → seedbox.Upload to ruTorrent

### Hydracker API quirks
- `POST /api/v1/torrents`, `/nzb`, `/liens` accept **language/sub names** (not IDs) — e.g. `langues[]=TrueFrench`, `subs[]=French`
- Language list comes from `/meta/langs`, but subs are validated against a **separate hardcoded list** — the two are stored in `frontend/src/hydrackerData.js` (extracted from `window.bootstrapData` on the site, since there's no `/meta/subs` endpoint)
- Torrent `download_url` in the POST response is a **site URL** that returns HTML to unauthenticated clients — use `GET /api/v1/torrents/{id}/download` with Bearer instead
- POST `/liens` response returns `{"liens": [{...}], "status": "success"}` (array, plural) — not a single `lien` object

### Upload progress (DDL)
`internal/uploader/progress.go` wraps the **read side** of an `io.Pipe` (what the HTTP client actually reads). Measuring on the write side gives disk throughput, not network speed. Throttled to emit at most every 250 ms; capped at 99 % until the HTTP response arrives (then 100 %). For 1Fichier, `Content-Length` is pre-computed by running the `multipart.Writer` on an empty buffer with a fixed boundary, then `contentLength = overhead + fileSize`.

### MediaInfo in WebView
`frontend/src/HydrackerTab.svelte` loads `mediainfo.js` dynamically and reads file chunks via Go (`GetFileSize`, `ReadFileChunk`). **Wails serializes `[]byte` as a base64 string**, not a number array — `toU8()` in the component decodes it with `atob` before passing to the WASM module. Without this, `readChunk` returns 0 bytes and only the "General" track is parsed.

### Auto-fill logic
Audio languages come from `mediaInfo.audios[].lang` (ISO codes), mapped via `mapAudioTracks()` : multiple `fr` tracks become `[TrueFrench, French (Canada), FRENCH AD]` in order. Filename parser (`internal/parser/filename.go`) is a fallback only — its `MULTi`/`VFF` tags are ignored. Subs use the separate `HYD_SUBS` list (no `TrueFrench`/`VO` entries). Flags `langsAutoFilled` / `subsAutoFilled` prevent re-fill after user deletion.

### Config & secrets
User config lives in `~/.config/go-post-tools/config.json` (outside the repo). No token, URL, or password is ever hardcoded. Tokens are passed to the Go backend via `SaveConfig` and injected into requests at runtime.

### Seedbox (ruTorrent WebUI)
Not SFTP — HTTP POST to `{baseURL}/php/addtorrent.php` with Basic Auth. Response is HTML; success is detected by the absence of `addtorrentfailed` / `,"error"` in the body. The uploaded file must be the Hydracker-generated .torrent (announce URL includes the user's passkey), not the local one.
