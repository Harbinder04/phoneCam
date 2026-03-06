# phonecam (CLI-first DroidCam-like prototype)

A minimal Go-based tool to use your phone camera as an MJPEG webcam source for your laptop.

## What it does

- Runs a local server on your laptop.
- Serves a mobile web page (`/`) that accesses your phone camera.
- Pushes JPEG frames from phone → laptop over HTTP.
- Exposes an MJPEG stream on `/mjpeg` for OBS/VLC/ffmpeg/OpenCV.

## Quick start

```bash
go run ./cmd/phonecam --token supersecret --port 8090
```

Then:

1. Connect phone and laptop to the same Wi‑Fi.
2. Open the URL shown in logs on your phone:
   - `http://<laptop-ip>:8090/?token=supersecret`
3. Press **Start** in mobile page.
   - Sender enforces **minimum 30 FPS** (configurable up to 60 FPS).
4. Use MJPEG URL on laptop apps:
   - `http://127.0.0.1:8090/mjpeg`

## CLI flags

- `--bind` (default `0.0.0.0`): host/interface to bind.
- `--port` (default `8090`): listen port.
- `--token` (**required**): shared upload token.

## Example integrations

### VLC
Open Network Stream → `http://127.0.0.1:8090/mjpeg`

### ffplay
```bash
ffplay -fflags nobuffer http://127.0.0.1:8090/mjpeg
```

### OBS
Use **Media Source** or plugin that supports MJPEG URLs.

## Notes / next steps

- This is intentionally simple for a CLI-first foundation.
- Add TLS + pairing QR code for better security UX.
- For virtual camera support, bridge MJPEG to `v4l2loopback` on Linux.
- A future React Native sender app can replace the web sender page while reusing `/api/frame`.
