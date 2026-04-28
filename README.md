# Dragon QR Quest

Dragon QR Quest is a simple Go webserver for running a kid-friendly fantasy scavenger hunt. Players scan QR codes, create an adventurer, collect weapons and armor, meet companions, fight smaller enemies, follow clues, and eventually face the final dragon.

The game is designed for a small in-person event. Version 1 uses one browser/device per player and stores progress in a local JSON file. Multiplayer and cross-device accounts can come later.

## Quick Start

From this directory:

```sh
go run ./cmd/dragonqr -addr 127.0.0.1:8097 -base-url http://127.0.0.1:8097
```

Open:

- Player start: `http://127.0.0.1:8097/q/start`
- Organizer page: `http://127.0.0.1:8097/organizer`
- Printable QR codes: `http://127.0.0.1:8097/organizer/print`

Run tests with:

```sh
go test ./...
```

## Running the Game

1. Edit `quest.yaml` if you want to change the story, clues, stats, or QR code locations.
2. Start the server with a base URL that phones can reach.
3. Open `/organizer`.
4. Generate missing station images if you want art on the printable cards.
5. Open `/organizer/print`.
6. Print and cut out the reusable QR cards.
7. Place each QR card using the organizer note matched by stable ID.
8. Send players to the start QR code first.

For a real event, `-base-url` must be the URL players' phones can open. If the server runs on a laptop on your local network, use that laptop's LAN address, for example:

```sh
go run ./cmd/dragonqr -addr 0.0.0.0:8097 -base-url http://192.168.1.50:8097
```

Then print the QR page after starting the server with that same base URL.

## Organizer Password

Organizer pages are open by default for local setup. To require a password, set `ORGANIZER_PASSWORD` before starting the server:

```sh
ORGANIZER_PASSWORD=change-me go run ./cmd/dragonqr -addr 0.0.0.0:8097 -base-url http://192.168.1.50:8097
```

When prompted, use:

- Username: `organizer`
- Password: the value of `ORGANIZER_PASSWORD`

## Player Instructions

Players should scan the start QR code first. They will:

1. Enter their name.
2. Choose an adventurer name.
3. Scan QR codes around the play area.
4. Collect gear, companions, healing, and clues.
5. Fight smaller enemies.
6. Return to their status page when they want to check health, attack, armor, companions, and clues.
7. Scan the dragon QR code when they are ready.

Progress is remembered with a browser cookie. A player should keep using the same phone/browser during the event.

If a player scans a power-up twice, it only counts once. Enemies also cannot be farmed repeatedly for extra rewards.

## Editing `quest.yaml`

`quest.yaml` controls the whole quest. Future organizers should be able to change the game without touching Go code.

Top-level fields:

- `title`: Game title shown to players and organizers.
- `intro`: Opening story text.
- `start_code`: ID of the start QR code.
- `dragon_code`: ID of the final dragon QR code.
- `base_health`, `base_attack`, `base_armor`: Starting player stats.
- `dragon_requirements`: Minimum stats and enemies defeated before the dragon fight is allowed.
- `victory_text`: Text shown after the dragon is defeated.
- `codes`: The QR code list.

Each code has:

- `id`: Stable URL ID, used in `/q/{id}`.
- `type`: One of `start`, `weapon`, `armor`, `companion`, `enemy`, `healing`, `clue`, or `dragon`.
- `title`: Player-facing name.
- `label`: Short printable label.
- `description`: Text shown when scanned.
- `clue`: Hint shown after the scan.
- `organizer_note`: Placement note shown on organizer and print pages.
- `image_prompt`: Optional image-generation prompt. If omitted, the app builds one from the title, description, type, and quest title.
- `effects`: Stat changes for power-ups, healing, companions, and clues.
- `enemy`: Enemy stats for `enemy` and `dragon` codes.
- `rewards`: Stat changes earned after defeating a regular enemy.

Example power-up:

```yaml
- id: silver-sword
  type: weapon
  title: "Silverleaf Sword"
  label: "Weapon"
  description: "A bright blade hums in your hand."
  clue: "The next danger waits where shadows hang upside down."
  organizer_note: "Hide near books, shelves, or a story area."
  effects:
    attack: 3
```

Example enemy:

```yaml
- id: cave-bat
  type: enemy
  title: "Cave Bat Swarm"
  label: "Enemy"
  description: "A cloud of squeaking bats swoops from the dark."
  clue: "Look where water would cross if this were a tiny kingdom."
  organizer_note: "Place somewhere dim or under a table."
  enemy:
    health: 6
    attack: 2
    armor: 0
  rewards:
    health: 2
```

The printed cards show stable IDs instead of story titles or raw URLs, so you can change titles, descriptions, clues, organizer notes, and generated art while keeping the same printed cards. Reprint only when a code `id` or `-base-url` changes.

## Station Images

The organizer page can generate one local WebP image for each station. Set `OPENAI_API_KEY` before starting the server:

```sh
OPENAI_API_KEY=sk-... go run ./cmd/dragonqr -addr 127.0.0.1:8097 -base-url http://127.0.0.1:8097
```

Then open `/organizer` and use "Generate Missing Images" or the per-code buttons. Existing image files are not overwritten. Images are saved under:

```text
static/generated/stations/{code-id}.webp
```

The app uses OpenAI's `gpt-image-2` image model. If `image_prompt` is absent in `quest.yaml`, the prompt is built from the current code data. For Docker deploys, generate the images before `docker build` if you want them baked into the image; the Dockerfile already copies `static/`.

## Game Balance

Combat is intentionally simple and resolves one roll at a time:

- Scanning an enemy starts a battle page.
- Each tap rolls `1d6` for the player and `1d6` for the enemy.
- Player damage is `player roll + player attack - enemy armor`, minimum `1`.
- If the enemy survives, enemy damage is `enemy roll + enemy attack - player armor`, minimum `1`.
- The player wins when enemy health reaches `0`.
- If player health reaches `0`, other scans are blocked until a `healing` code is scanned.

Use smaller numbers for younger kids and shorter games. If players get stuck before the dragon, lower `dragon_requirements` or add more `effects` and `rewards`.


## Files

- `quest.yaml`: Editable quest content.
- `cmd/dragonqr/main.go`: Server entrypoint.
- `internal/game`: Quest validation, player state, and combat rules.
- `internal/server`: HTTP handlers and QR generation.
- `templates`: HTML pages.
- `static/style.css`: Styling.
- `data/players.json`: Created at runtime to store player progress.

## Resetting Progress

Stop the server and delete `data/players.json`:

```sh
rm data/players.json
```

The next run will start with no saved players.

## Deployment Notes

This app is meant to be run by a trusted organizer on a local machine or small private server. It does not yet include full production authentication, HTTPS setup, multiplayer accounts, or an admin editor.

For an event, make sure:

- Players' phones are on the same network as the server.
- The `-base-url` value matches the address phones can reach.
- The QR codes were printed after setting the final `-base-url`.
- Organizer pages are password-protected if the server is reachable by participants.

## Docker

Build the container image:

```sh
docker build -t dragonqr:latest .
```

Run it locally:

```sh
docker run --rm -p 8097:8080 \
  -v "$PWD/data:/app/data" \
  dragonqr:latest \
  -addr 0.0.0.0:8080 \
  -base-url http://127.0.0.1:8097 \
  -quest /app/quest.yaml \
  -data /app/data/players.json
```

## Kubernetes

Kubernetes manifests live in `k8s/` and target the `dragon` namespace. They include:

- `namespace.yaml`: Creates the `dragon` namespace.
- `pvc.yaml`: Stores `data/players.json` so player progress survives pod restarts.
- `deployment.yaml`: Runs one `dragonqr` pod.
- `service.yaml`: Exposes the pod inside the cluster.
- `ingress.yaml`: Routes public HTTP traffic to the service.

Before deploying, edit:

- `k8s/deployment.yaml`: Replace `ghcr.io/YOUR-ORG/dragonqr:latest` with the image you pushed.
- `k8s/deployment.yaml`: Set `DRAGONQR_BASE_URL` if the public URL is not `https://dragon.northbriton.net`.
- `k8s/ingress.yaml`: Adjust `ingressClassName` if your cluster does not use `nginx`.

Create an organizer password secret:

```sh
kubectl create namespace dragon
kubectl -n dragon create secret generic dragonqr-organizer --from-literal=password='change-me'
```

Deploy:

```sh
kubectl apply -f k8s/
```

Check rollout:

```sh
kubectl -n dragon rollout status deployment/dragonqr
kubectl -n dragon get pods,svc,ingress
```

The Kubernetes deployment uses `replicas: 1` because player state is stored in one JSON file on a single persistent volume.
