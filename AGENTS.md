# Dragon QR Quest Agent Notes

This repo is a small Go web app for a kid-friendly QR-code fantasy quest. Keep changes simple and event-operator friendly.

## Project Shape

- Entry point: `cmd/dragonqr/main.go`
- Game rules and persistence: `internal/game`
- HTTP handlers and QR generation: `internal/server`
- HTML templates: `templates`
- CSS: `static/style.css`
- Editable quest content: `quest.yaml`
- Kubernetes manifests: `k8s`

The app renders server-side HTML with Go `html/template`. There is no frontend build pipeline.

## Runtime Behavior

- Players start at `/q/start`, create an adventurer, then scan `/q/{codeID}` URLs.
- Player identity is a browser cookie named `dragonqr_player`.
- Player progress is stored in `data/players.json`.
- Organizer pages:
  - `/organizer`
  - `/organizer/print`
- Combat route:
  - `POST /combat/roll`
- Organizer image generation routes:
  - `POST /organizer/images/generate`
  - `POST /organizer/images/generate/{id}`
- Organizer auth uses HTTP Basic auth when `ORGANIZER_PASSWORD` is set. Username is `organizer`.
- QR images on the print page are generated as PNG data URLs. Keep them typed as `template.URL`; plain strings are sanitized to `#ZgotmplZ`.
- Printable cards are intentionally reusable: they should show stable IDs, QR codes, and optional station art, but not story titles, placement notes, or visible raw URLs.
- QR payloads still point at `/q/{codeID}`; changing a code `id` or `DRAGONQR_BASE_URL` still requires reprinting.
- Player progress should be durable across title/description changes. Store and compare stable code IDs where possible; render current display names from `quest.yaml`.
- Existing player JSON may contain older title-backed `items` or `companions`. Keep compatibility when changing player display logic.
- Enemy and dragon scans start or resume a tap-to-roll combat flow. Do not resolve combat immediately inside the scan handler.
- If player health reaches `0`, normal scans are blocked until the player scans a `healing` code.

## Station Images

- Optional per-code `image_prompt` can be set in `quest.yaml`; otherwise prompts are built from the code title, type, description, and quest title.
- Organizer-triggered image generation uses `OPENAI_API_KEY` and OpenAI `gpt-image-2` through `github.com/openai/openai-go/v3`.
- Generated station images are saved as WebP files under `static/generated/stations/{codeID}.webp`.
- Existing generated images should not be overwritten by "generate missing" actions.
- The Dockerfile copies `static/`, so generate station images before `docker build` if they should be baked into an image.
- Keep generated image assets local and optional. Normal gameplay, QR printing, and tests should work without `OPENAI_API_KEY`.

## Deployment Assumptions

- Live domain is `https://dragon.northbriton.net`.
- Kubernetes namespace is `dragon`.
- Public origin is configured with `DRAGONQR_BASE_URL`.
- The ingress host is `dragon.northbriton.net`.
- Use `kubectl apply -f k8s/`; there is intentionally no `kustomization.yaml`.
- The deployment must stay at `replicas: 1` while storage is a single JSON file on a PVC.

Do not scale to multiple replicas without replacing JSON-file persistence or adding safe cross-process storage. A second pod can corrupt or overwrite progress.

## Local Commands

Run tests:

```sh
go test ./...
```

Run locally:

```sh
go run ./cmd/dragonqr -addr 127.0.0.1:8097 -base-url http://127.0.0.1:8097
```

Build Docker image:

```sh
docker build -t dragonqr:dev .
```

Validate Kubernetes manifests without touching a cluster:

```sh
kubectl create --dry-run=client --validate=false -f k8s/
```

## Editing Guidance

- Prefer changing `quest.yaml` for story, clues, code IDs, rewards, and stats.
- If any QR `id` changes, organizers must reprint QR codes. Story-only changes should not require reprinting.
- Keep the app usable on phones.
- Keep YAML fields backward-compatible unless tests and README are updated.
- Do not commit or delete live runtime state in `data/players.json` unless explicitly asked. `data/.gitkeep` is the tracked placeholder.
- Avoid adding a frontend build system unless there is a clear need.
- Keep combat state in `Player.Combat` small and JSON-serializable; progress is persisted after each roll.
- For combat tests, use deterministic roll helpers instead of relying on random rolls.

## Test Expectations

Before handing off code changes, run:

```sh
go test ./...
```

For deployment changes, also run:

```sh
kubectl create --dry-run=client --validate=false -f k8s/
```

For QR print changes, verify `/organizer/print` does not contain `#ZgotmplZ` and contains `src="data:image/png;base64,` for each quest code.

Also verify reusable print-card behavior: `/organizer/print` should not expose code titles or raw `/q/{id}` URLs as visible text.
