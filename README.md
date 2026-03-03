# vnc-recorder

VNC recorder written in Go with:
- environment variable and CLI flag configuration,
- frame capture through a Go VNC client (not `ffmpeg -f vnc`),
- local output as a single file or segmented files,
- optional S3 upload,
- container image builds with `ko`.

## Prerequisites

- Go 1.24+
- `ffmpeg` available in `PATH` (or via `FFMPEG_PATH`)
- `ko` for OCI image builds
- `task` (https://taskfile.dev)

## Configuration

Each option can be configured with an environment variable and overridden by a CLI flag.

| Env var | Flag | Default |
|---|---|---|
| `VNC_HOST` | `--vnc-host` | `localhost` |
| `VNC_PORT` | `--vnc-port` | `5900` |
| `VNC_PASSWORD` | `--vnc-password` | `` |
| `VNC_CONNECT_MAX_RETRIES` | `--vnc-connect-max-retries` | `30` |
| `VNC_CONNECT_RETRY_DELAY` | `--vnc-connect-retry-delay` | `2` |
| `VIDEO_FPS` | `--video-fps` | `25` |
| `VIDEO_QUALITY` | `--video-quality` | `23` |
| `SEGMENT_DURATION` | `--segment-duration` | `300` |
| `OUTPUT_DIR` | `--output-dir` | `/tmp/vnc-recordings` |
| `FILE_BASENAME` | `--file-basename` | `recording` |
| `FFMPEG_PATH` | `--ffmpeg-path` | `ffmpeg` |
| `UPLOAD_S3` | `--upload-s3` | `true` if `S3_BUCKET` is set |
| `S3_BUCKET` | `--s3-bucket` | `` |
| `S3_PREFIX` | `--s3-prefix` | `` |

### Local recording modes

- Single file: `SEGMENT_DURATION=0`
- Segments: `SEGMENT_DURATION>0` (files named `segment_<session>_%03d.mp4`)

## Development (TDD)

```bash
task test
```

## Build binaries

```bash
task build
```

## Build image with ko

```bash
task image
```

`ko` uses `.ko.yaml` with `docker.io/jrottenberg/ffmpeg:6.1-alpine` as the base image, so `ffmpeg` is included in the final image.

## Release

A GitHub Actions workflow automatically publishes:
- binaries (`linux`, `macOS`, `windows`) to a GitHub Release,
- an OCI image to GHCR via `ko` (with `ffmpeg` included),
- release notes changelog generated from commits between the previous and current tag.

Trigger: push a `v*` tag (for example `v1.0.0`).

On pull requests, the `changelog-linter` workflow validates changelog updates (`CHANGELOG.md`, `## tip`) unless only docs/CI/meta files changed.

## Example run

```bash
task run -- \
  --vnc-host 127.0.0.1 \
  --vnc-port 5900 \
  --vnc-password '<password>' \
  --segment-duration 60 \
  --file-basename video \
  --output-dir /tmp/vnc-recordings \
  --upload-s3=false
```
