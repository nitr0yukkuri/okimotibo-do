# Okimochi Board (おきもちぼ〜ど)

[日本語](README.md) | [English](README.en.md)

Okimochi Board is a presence board that reads whether someone wants to be approached while working on a PC from hand gestures and facial expressions, then shares the resulting status in real time with screens in the same room.

Recognition normally runs in the browser with MediaPipe. The PC confirms the status and sends it to a phone display through a Go WebSocket API. In the normal recognition flow, camera video is not sent to the server.

- Public app: https://okimotibo-public.vercel.app/
- Public repository: https://github.com/nitr0yukkuri/okimotibo-do

## Problem

When someone is working, others often have to guess whether it is a good time to approach. Okimochi Board focuses primarily on the person’s explicit hand gestures, with facial expressions as a secondary signal, and turns them into a status visible to others.

Possible settings include a personal desk, a lab, a school, a shared office, or remote work.

## Current user flow

1. Sign in with a Google account on a PC.
2. Show a hand gesture to the PC camera, or press a button on the PC screen.
3. The PC sends the confirmed status to the same room.
4. A phone reads the status for the same account and room and displays it for nearby people.
5. If the WebSocket disconnects, reconnects, or the screen resumes, the client retrieves the current value from the status API.

Signed-in users get their rooms from Supabase `room_members`. The phone screen is currently read-only. It cannot change the status; the primary inputs are camera recognition or the manual controls on the PC.

## Status definitions

| Input | Gesture / expression | API status | Meaning on screen |
|---|---|---|---|
| Thumb up | `thumb_up` | `available` | “I’m up for it / free!” |
| Thumb sideways | `sideways_thumb` | `neutral` | “That’s fine / I can respond” |
| Thumb and little finger | `shaka` | `neutral` | “That’s fine / I can respond” |
| Thumb down | `thumb_down` | `busy` | “Not now / working” |
| Smile (secondary) | `smile` | `available` | “I’m up for it” |
| Focused expression (secondary) | `frown` | `busy` | “Not now” |

A facial `frown` does not determine someone’s emotion. Multiple blendshapes for the eyebrows, eyes, and mouth are used as a secondary signal that the person may be concentrating. Explicit hand gestures take priority.

## Main features

### PC control screen

- Change status with hand gestures
- Change status manually with three on-screen buttons
- Preview the camera image
- Turn expression recognition on or off
- Always-on-top mini panel using Chrome’s Document Picture-in-Picture
- Google OAuth sign-in
- Sign out and reconnect

### Phone display

- Show a read-only screen on phones and narrow displays
- Display the status received from the PC with a background color, icon, and text
- Add the app to the home screen as a PWA
- Does not use the phone’s camera or images

### Synchronization

- A PC and phone share the same status when signed into the same account and room
- PC camera recognition and manual button changes are sent over WebSocket
- Manual status is held indefinitely and takes priority over face recognition; an explicit hand gesture can change it
- The PC display reflects only states saved and distributed by the server; it does not display an unpersisted recognition result first
- The phone receives `status.changed` over WebSocket, fetches the current state on connect, reconnect, and screen resume, and also rechecks the current state every 15 seconds while open to recover from missed Redis events
- Validate sequence and `capturedAt` so an older state from the same user cannot overwrite a newer one

The backend also has temporary pairing APIs using a QR code or passphrase. A pairing grant lasts 10 minutes, can be used once, and is read-only. The main path currently uses the same account on both devices.

## AI recognition

### Hand recognition

Combines MediaPipe Gesture Recognizer in the browser with custom classification based on 21 hand landmarks.

- Uses MediaPipe’s standard classification for `thumb_up` and `thumb_down`
- Detects `shaka` from whether the thumb, index, middle, ring, and little fingers are extended, using landmarks
- Detects `sideways_thumb` from the thumb’s direction using landmarks
- Excludes a closed fist from status updates
- Default inference interval: 100 ms (about 10 FPS)
- Minimum confidence for standard classification: 0.70
- Confirms a state when 4 of the latest 6 samples agree
- Holds the previous state for up to 1 second during a temporary detection loss
- Prevents face recognition from overriding a hand result for 3 seconds after a hand is recognized

The current up/down thumb check uses the Y direction in the camera image. A sideways gesture may be unstable when the wrist is rotated substantially.

### Face recognition

MediaPipe Face Landmarker blendshapes are processed in the browser.

- Calibrate the first 5 samples as the user’s baseline
- Smile threshold: confidence of at least 0.55
- Frown threshold: confidence of at least 0.35
- Confirm a candidate that persists for about 1.5 seconds
- Lock the confirmed result for 4 seconds
- Gradually update the baseline while the expression is neutral
- Use expressions as a secondary input only when no hand gesture is available

This recognition does not claim that the person truly feels a particular emotion; it is an input signal for changing the displayed status.

### Optional Py-Feat v2 API

The `emotion-api` includes an image-based expression API using Py-Feat v2. The normal app does not use this API automatically; it uses MediaPipe recognition in the browser.

When Py-Feat is explicitly enabled through library use, a JPEG resized to a maximum width of 640 px is sent to the API. The API processes it as a temporary file and deletes it after inference. The pretrained Py-Feat v2 model is intended for research and non-commercial use.

## Architecture

```text
PC browser
  ├─ Camera
  │   ├─ MediaPipe Gesture Recognizer
  │   ├─ Hand-landmark classifier
  │   └─ MediaPipe Face Landmarker
  ├─ Manual buttons
  └─ StateSocket
       │ recognition.update
       ▼
Go HTTP / WebSocket API
  ├─ Authentication and room permission checks
  ├─ Validate sequence, timestamp, and confidence
  ├─ Persist current status
  └─ Broadcast status.changed to the same room
       ├─ Phone display
       ├─ Separate PC window
       └─ M5Stick prototype client

Supabase Auth / Postgres
  ├─ rooms
  ├─ room_members
  ├─ current_statuses
  └─ pairing_grants

Optional demo path:
  Browser image → Python / Py-Feat v2 API
```

## Data and privacy

With normal MediaPipe recognition, camera video, face images, and full frames are not sent to the server. WebSocket and Supabase store or distribute metadata such as status, gesture, expression summary, confidence, and timestamps.

Supabase `current_statuses` stores the current status. Accepted, meaningful status changes are recorded in `status_events`. Heartbeat lease renewals do not change the recognized state and are not recorded as history events.

Only when Py-Feat v2 is explicitly used is a resized JPEG sent to the Python API. The image is not stored by the Go API or Supabase.

Never include the following secrets in the frontend in production:

- Supabase Secret key
- Supabase service role key
- Backend administration token

The frontend uses the public Supabase key and the user access token obtained after sign-in.

## Directories

```text
frontend/       React + TypeScript UI, recognition library, and PWA
backend/        Go HTTP / WebSocket API
emotion-api/    Optional Python + Py-Feat v2 expression API
supabase/       Postgres migrations and RLS
hardware/       M5StickC Plus2 display client prototype
render.yaml     Render Blueprint
```

## Requirements

- Node.js version capable of running the Vite, TypeScript, and Vitest versions in `frontend/package.json`
- Go 1.24+
- Python 3.11 (only if using the Py-Feat v2 API)
- A browser that supports a camera and WebSocket
- Supabase Auth configuration for Google sign-in in production

## Setup

Install dependencies from the repository root:

```text
npm ci
```

Run frontend tests and build:

```text
npm test
npm run build
```

### Start the frontend locally

```text
npm run dev
```

It normally opens at http://localhost:5173/. The Vite development proxy forwards `/api` and WebSocket traffic to `localhost:8080`.

To try local anonymous mode, set the following in `frontend/.env.local`:

```dotenv
VITE_ANONYMOUS_MODE=true
VITE_ROOM_ID=local-room
VITE_USER_ID=local-user
```

Anonymous mode is for local testing. Use Supabase Auth and room permissions in production.

### Start the Go API locally

Run this in a separate terminal:

```powershell
cd backend
$env:ALLOW_ANONYMOUS="true"
go run ./cmd/server
```

In anonymous mode, the API uses the `userId` sent by the frontend as-is for validation. To test production-like authentication, set `ALLOW_ANONYMOUS` to false and configure `SUPABASE_URL` and `SUPABASE_SECRET_KEY`.

### Verify the backend in local Kubernetes (kind)

The Kubernetes setup starts the Backend, Emotion API, and Redis Pub/Sub. The Backend defaults to one Pod. Redis only relays events between Pods; it is not the source of truth for status. Because the anonymous-mode Memory store is separate for each Pod, scale to two replicas only in a production-like environment connected to Supabase or when specifically testing inter-Pod events.

Install kind, Docker Desktop, and kubectl, then run these commands from the repository root:

```powershell
kind create cluster --name okimochi --config .\k8s\kind-config.yaml
docker build -t okimochi-backend:dev .\backend
docker build -t okimochi-emotion-api:dev .\emotion-api
kind load docker-image okimochi-backend:dev --name okimochi
kind load docker-image okimochi-emotion-api:dev --name okimochi
kubectl apply -k .\k8s
kubectl -n okimochi rollout status deployment/okimochi-backend
kubectl -n okimochi rollout status deployment/okimochi-emotion-api
kubectl -n okimochi port-forward service/okimochi-backend 8080:8080
```

In another terminal, check liveness and readiness:

```powershell
Invoke-WebRequest http://127.0.0.1:8080/healthz
Invoke-WebRequest http://127.0.0.1:8080/readyz
```

kind enables anonymous verification. For production-like authentication, set `ALLOW_ANONYMOUS` to false and create a Secret instead of writing credentials directly into a manifest:

```powershell
kubectl -n okimochi create secret generic okimochi-backend-secret `
  --from-literal=SUPABASE_URL="https://your-project.supabase.co" `
  --from-literal=SUPABASE_SECRET_KEY="your-secret-key"
```

When deleting the cluster, target only the named kind cluster:

```powershell
kind delete cluster --name okimochi
```

To test IoT long-press, Lease, and Heartbeat behavior without hardware:

```powershell
cd backend
go run ./cmd/iot-simulator --url ws://127.0.0.1:8080/api/v1/ws
```

Use `b` / `n` / `a` to send statuses. The simulator renews a 30-second Lease with a Heartbeat every 10 seconds. On `d` disconnect, the status is cleared after a 5-second reconnect grace period.

To connect an M5Stick on the LAN to kind, enable the port mapping in `k8s/kind-config.yaml` when creating the cluster, then manually apply the following after considering the implications. This exposes an anonymous Backend to the network and is for local learning only.

```powershell
kubectl apply -f .\k8s\backend-nodeport-service.yaml
```

Set the M5Stick’s `WS_HOST` to the PC’s LAN IP and `WS_PORT` to `30080`. In production, use `wss://` through an authenticated, TLS-enabled Ingress or LoadBalancer instead of NodePort.

### Production-like Kubernetes configuration

`k8s/` is for learning with kind in anonymous mode and one Pod; do not apply it directly to production. Shared manifests are in `k8s/base/`, with production-specific changes in `k8s/overlays/production/`.

The production overlay enables multiple Backend and Emotion API Pods, HPA, PDB, startup probes, NetworkPolicy, and a WebSocket Ingress. Local Redis does not persist data, so it is not included in the production overlay. Set up a shared store such as Supabase and a managed Redis instance with TLS first.

Before applying it, replace the example domain and image names with the real environment’s values, and prepare an Ingress controller and Metrics Server. Do not store Secrets in Git.

```powershell
kubectl -n okimochi create secret generic okimochi-backend-secret `
  --from-literal=SUPABASE_URL="https://your-project.supabase.co" `
  --from-literal=SUPABASE_SECRET_KEY="your-secret-key" `
  --from-literal=REDIS_URL="rediss://your-managed-redis:6380"

kubectl kustomize .\k8s\overlays\production
kubectl apply -k .\k8s\overlays\production
kubectl -n okimochi rollout status deployment/okimochi-backend
kubectl -n okimochi get pods,svc,ingress,hpa,pdb,networkpolicy
```

See [`k8s/overlays/production/README.md`](k8s/overlays/production/README.md) for detailed prerequisites.

### Optional Python expression API

Prepare a Python environment only if you use the Py-Feat v2 API:

```powershell
py -3.11 -m venv .venv
.\.venv\Scripts\Activate.ps1
pip install -c emotion-api\constraints.txt -r emotion-api\requirements.txt
$env:ALLOWED_ORIGINS="http://localhost:5173,http://127.0.0.1:5173"
python -m uvicorn app:app --app-dir emotion-api --host 127.0.0.1 --port 8000
```

Health check:

```text
GET http://127.0.0.1:8000/healthz
```

To use this API from the frontend, explicitly pass `emotion.url` to `useStateRecognition`. The normal App screen does not pass this option.

## Using the frontend library

To use the recognition and synchronization library without the UI, pass a video element:

```tsx
import { useRef } from "react";
import { useStateRecognition } from "@okimochi/recognition-client";

export function CameraRuntime() {
  const videoRef = useRef<HTMLVideoElement>(null);

  useStateRecognition(videoRef, {
    face: { enabled: true },
    socket: {
      url: "ws://localhost:8080/api/v1/ws",
      token: "SUPABASE_ACCESS_TOKEN",
      roomId: "ROOM_UUID",
      clientId: crypto.randomUUID(),
      userId: "USER_UUID"
    }
  });

  return <video ref={videoRef} hidden />;
}
```

A production setup can also serve WASM and MediaPipe models from the same origin. The current default URLs use a CDN.

## Authentication and synchronization settings

### Frontend environment variables

Set these in `frontend/.env.local` or the deployment environment.

| Variable | Purpose |
|---|---|
| `VITE_SUPABASE_URL` | Supabase Project URL |
| `VITE_SUPABASE_ANON_KEY` | Public Supabase key |
| `VITE_WS_URL` | WebSocket API URL. Optional in development; set to `wss://` in production |
| `VITE_PUBLIC_APP_URL` | Public app URL used to create QR / pairing links |
| `VITE_ROOM_ID` | For unauthenticated local testing or a fixed room; optional when signed in |
| `VITE_USER_ID` | User ID for unauthenticated local testing |
| `VITE_ANONYMOUS_MODE` | Set to `true` to enable anonymous mode for local use and demos |

If signed in and `VITE_ROOM_ID` is omitted, the app currently uses the first room returned by `room_members`. There is no UI for selecting among multiple rooms.

### Backend environment variables

| Variable | Purpose |
|---|---|
| `PORT` | HTTP port. Default: 8080 |
| `ALLOWED_ORIGINS` | Comma-separated list of permitted Origins |
| `ALLOW_ANONYMOUS` | `true` is for local testing; set to `false` in production |
| `SUPABASE_URL` | Supabase Project URL |
| `SUPABASE_SECRET_KEY` | Backend-only Secret key |
| `REDIS_URL` | Redis URL for relaying events between Pods. If unset, use the local single-Pod Hub only |
| `STATUS_TTL` | Lifetime of automatically recognized statuses: 0 means no expiry, maximum 24 hours. Manual frontend statuses remain indefinite for compatibility |

The default `STATUS_TTL` is 15 minutes. When an automatically recognized status expires, the API stops returning it and the screen returns to its initial state (“available”). Manual statuses from IoT include `leaseSeconds` and expire if Heartbeats stop. Expiry or disconnect broadcasts `status.cleared`. If the PC’s public connection disconnects, after a 5-second reconnect grace period the API conditionally deletes only the status that was current at disconnection. `render.yaml` sets the TTL to 15 minutes.

## Go API

### HTTP endpoints

```text
GET  /healthz
GET  /readyz
GET  /api/v1/rooms/{roomId}/status/{userId}
POST /api/v1/pairing
POST /api/v1/pairing/claim
GET  /api/v1/ws
```

### WebSocket flow

1. The client sends `client.hello` within 8 seconds.
2. The Backend validates the Supabase access token, room membership, or anonymous configuration.
3. After authentication, it returns `server.ready` with the connecting user’s current status (the `state` field is omitted if none exists).
4. The PC sends `recognition.update`.
5. The Backend saves the current status and broadcasts `status.changed` to the same room.
6. The phone or another client updates its display.

For IoT Lease renewal, use these messages:

```json
{"type":"recognition.update","sequence":1,"capturedAt":"2026-01-01T00:00:00Z","status":"busy","source":"manual","leaseSeconds":30}
{"type":"status.heartbeat","leaseSeconds":30}
```

`status.heartbeat` extends only a manual status owned by the same `clientId`. A Heartbeat after expiry returns `state_expired`; the IoT client sends a new status with its next action.

Example message:

```json
{
  "type": "recognition.update",
  "sequence": 1,
  "capturedAt": "2026-08-25T10:30:15.420Z",
  "status": "available",
  "source": "hand",
  "hand": {
    "gesture": "thumb_up",
    "confidence": 0.91,
    "handedness": "Right"
  },
  "face": null
}
```

`source` can be `hand`, `face`, `manual`, or `none`. The Backend also checks consistency between hand gestures, expressions, and statuses.

The server validates:

- Message size no greater than 16 KiB
- Monotonically increasing `sequence`
- `capturedAt` not too far from the current time
- Confidence between 0 and 1
- Valid combinations of `source`, `status`, `hand`, and `face`
- Supabase access token and `room_members` when Supabase is enabled
- Read-only restrictions for paired clients

WebSocket sends a ping every 25 seconds. The frontend reconnects after a disconnect using exponential backoff.

## Supabase

Apply migrations in this order:

1. `supabase/migrations/001_initial.sql`
2. `supabase/migrations/002_manual_recognition_source.sql`
3. `supabase/migrations/003_pairing_grants.sql`
4. `supabase/migrations/004_manual_priority_and_expiry.sql`

Main tables:

- `rooms`: rooms
- `room_members`: user-to-room membership
- `current_statuses`: current status per user
- `status_events`: schema for history
- `pairing_grants`: temporary credentials for QR / passphrase pairing

RLS is enabled so normal users can read rooms they belong to. The Backend uses the service role to write current statuses and operate pairing information.

To use Google sign-in, enable the Google Provider in Supabase Auth and set the public app URL as the Site URL and Redirect URL. Do not expose the Secret key in the browser.

## Deployment

### Frontend

Deploy to static hosting such as Vercel with these settings:

- Build command: `npm ci && npm run build`
- Output directory: `frontend/dist`
- `VITE_SUPABASE_URL`
- `VITE_SUPABASE_ANON_KEY`
- `VITE_WS_URL`
- `VITE_PUBLIC_APP_URL`

Use a TLS-enabled `wss://` URL for `VITE_WS_URL` in production.

### Go API and Py-Feat API

`render.yaml` defines three services:

- `okimochi-api`: Go WebSocket / HTTP API
- `okimochi-emotion-api`: optional Py-Feat v2 API
- `okimochi-frontend`: static frontend

The Go API can also be deployed to Cloud Run. Configure timeouts, Origin, Supabase Secret key, and health checks for the environment because WebSocket connections are long-lived.

In production, set `ALLOW_ANONYMOUS=false` and allow only the actual frontend Origin in `ALLOWED_ORIGINS`.

## M5StickC Plus2 prototype

`hardware/m5stick/main.ino` contains a prototype client that displays WebSocket status on an M5StickC Plus2 screen.

See [`hardware/m5stick/README.md`](hardware/m5stick/README.md) for uploading the program to the pictured device using Arduino IDE.

```text
Camera / manual control
  → Go API
  → WebSocket
  → M5StickC Plus2 display
```

The current sketch is for local testing. Wi-Fi, host, port, and token are source-code placeholders; TLS WebSocket and production authentication need to be configured separately. If `TARGET_USER_ID` is empty, it displays only the authenticated user from `server.ready`. If set, it must match the user authenticated by the WebSocket token. After sending a manual status, it sends `status.heartbeat` every 10 seconds to renew the 30-second Lease. The M5Stick is not required by the app; it is a prototype for expanding the display to a physical device.

### IoT simulator without hardware

If you do not have a device, use the development simulator included with the Backend to reproduce M5Stick communication. The simulator sends the same WebSocket messages as the M5Stick, without going through the frontend.

Start the local Backend in anonymous mode:

```powershell
cd backend
$env:ALLOW_ANONYMOUS="true"
$env:SUPABASE_URL=""
$env:SUPABASE_SECRET_KEY=""
go run ./cmd/server
```

In another terminal, start the simulator:

```powershell
cd backend
go run ./cmd/iot-simulator
```

Simulator commands:

- `b` / `busy`: concentrating
- `n` / `neutral`: okay to approach
- `a` / `available`: available
- `d` / `disconnect`: disconnect
- `r` / `reconnect`: reconnect
- `q` / `quit`: quit

If a status is sent while disconnected, only the latest one is queued and sent on reconnect. For an environment using Supabase authentication, pass an access token with `-token`. Do not embed production secrets in the simulator or M5Stick.

## Tests

### Automated tests

```text
npm test
npm run build

cd backend
go test ./...

cd ../emotion-api
python -m pytest
```

If Python dependencies for Py-Feat are not installed, install the Python dependencies before running Emotion API tests.

### k6 WebSocket load test

Without a camera, a publisher and multiple viewers can verify connection success rate, status-delivery delay, sequence ordering, and Heartbeats in the synchronization backend. See [`performance/k6/README.md`](performance/k6/README.md) for instructions and safety precautions when using staging.

### Manual checks

- Left and right hands, mirrored display
- Thumb up, sideways, and down; shaka; closed fist
- Camera orientation and wrist angle
- Face-only, hand-only, and neither detected
- Face recognition does not immediately overwrite a hand result
- PC manual control updates the phone
- Re-login with the same account and state restoration
- WebSocket disconnect, reconnect, and screen resume
- Resync to the current state within 15 seconds after a Redis event is missed while WebSocket remains connected
- No state leaks across different rooms
- Pairing code expires after 10 minutes, works once, and is read-only

## Current limitations

- Hand recognition is affected by camera orientation and severe occlusion
- Thumb up/down uses the image Y direction and may become unstable if the wrist is rotated significantly
- Face recognition is a secondary status signal and does not determine the person’s emotion
- MediaPipe models and WASM are loaded from a CDN by default
- Py-Feat v2 is an optional path and is not used by the normal App screen
- PWA and Document Picture-in-Picture support depends on the browser
- The M5Stick client is a local prototype; production TLS, authentication, and OTA updates are not implemented
