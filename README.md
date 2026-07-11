# Docker Volume Sync

A single container service to synchronize multiple Docker volumes with any remote storage provider supported by `rclone` (AWS S3, Google Drive, Backblaze B2, Azure, etc.). It discovers volumes to back up via Docker labels, restores them on startup, and schedules periodic backups.

## Features

- **Single Container**: One `volumesync` container can manage multiple volumes across different services.
- **Label-based Discovery**: Enable backup and configure schedules via Docker labels on your application containers.
- **Universal Support**: Uses `rclone` under the hood to support 50+ backend storages.
- **Safe Backups**: Optionally stops containers attached to the volume during backup to ensure data integrity.
- **Robust Healthcheck**: The service can be configured to only mark itself healthy once a specific number of volumes have been discovered and restored. This avoids race conditions in `docker-compose`.
- **Dynamic Discovery**: Automatically discovers and schedules backups for new containers added after `volumesync` has started.

## Configuration

### Global Environment Variables (on `volumesync` container)

| Variable | Description | Default | Required |
| :--- | :--- | :--- | :--- |
| `DESTINATION_PATH` | The destination URI according to rclone syntax (e.g., `s3:my-bucket/backups`). | - | **Yes** |
| `COMPRESSION` | Set to `true` to compress files at the destination (gzip). Acts as the default for all volumes; override per volume with the `volumesync.compression` label. | `false` | No |

*Note: You must also provide rclone credentials for your `DESTINATION_PATH` via standard rclone environment variables (e.g., `RCLONE_CONFIG_S3_TYPE=s3`).*

### Docker Labels (on application containers)

| Label | Description | Required | Default |
|:---|:---|:---|:---|
| `volumesync.enabled` | Set to `true` to enable backup for this container's volume. | **Yes** | - |
| `volumesync.volume` | The Docker volume name to back up. | **Yes** | - |
| `volumesync.schedule` | Cron expression for the backup schedule (e.g., `0 3 * * *`). | **Yes** | - |
| `volumesync.delete` | If `true`, delete files in destination not present in source. | No | `false` |
| `volumesync.concurrency` | Number of concurrent file transfers. | No | `16` |
| `volumesync.stop` | Whether to stop this container during backup. | No | `true` |
| `volumesync.stop_grace_period` | Grace period when stopping (e.g., `30s`, `1m`). | No | `30s` |
| `volumesync.subpath` | Subdirectory under `DESTINATION_PATH` for this volume. | No | `volumesync.volume` |
| `volumesync.uid` | User ID to apply to folders during initial sync (restore). | No | - |
| `volumesync.gid` | Group ID to apply to folders during initial sync (restore). | No | - |
| `volumesync.compression` | Compress this volume's files at the destination. Overrides `COMPRESSION` in both directions, so a volume can opt out of a globally-enabled default. | No | `COMPRESSION` |
| `volumesync.exclude` | `;`-separated glob patterns to skip (e.g. `*.log;cache/**`). See [Filtering](#filtering). | No | - |
| `volumesync.include` | `;`-separated glob patterns to sync *exclusively* (e.g. `data/**`). Anything not matching is skipped. See [Filtering](#filtering). | No | - |

## Filtering

Use `volumesync.exclude` to keep junk out of a backup, and `volumesync.include` to back up only part
of a volume. Both take standard rclone glob patterns. Match a folder and everything under it with
`**`:

```yaml
labels:
  - volumesync.exclude=*.log;cache/**;tmp/**
  - volumesync.include=data/**
```

**Patterns are separated by `;`, not `,`** — rclone globs use commas for brace alternation, so a
pattern like `*.{jpg,png}` stays in one piece.

**Excludes take precedence over includes.** If you set both, the volume is narrowed to the included
paths and then the excluded ones are carved back out of it:

| | `exclude=*.log`, `include=data/**` |
|:---|:---|
| `data/app.db` | backed up |
| `data/app.log` | skipped — the exclude beats the include |
| `other/notes.txt` | skipped — not included |

Two things worth knowing:

- **Filters apply to restores too, not just backups.** The same filters are used in both directions,
  so an excluded path is neither backed up nor restored.
- **Adding an exclude does not clean up the destination.** Files already backed up under a pattern you
  later exclude become invisible to the sync: they are neither restored nor deleted, even with
  `volumesync.delete=true`, and will keep occupying storage until you remove them yourself.

An invalid pattern is not fatal to the service, but that volume is skipped (and logged) rather than
being backed up with the wrong rules — so its healthcheck will never report ready.

## Compression

Setting `COMPRESSION=true` (or `volumesync.compression=true` on a single volume) compresses files
with gzip on the way to the destination and transparently decompresses them on restore. It is off by
default.

> [!WARNING]
> **Only enable compression against a fresh `DESTINATION_PATH`/`subpath`. Never switch it on (or
> off) over a destination that already holds backups.**
>
> Compression changes the on-remote layout: files are stored as `name.<size>.gz` plus a `name.json`
> sidecar, so the destination is no longer a plain browsable mirror. rclone's compress backend
> **cannot see** files that were written uncompressed — they simply do not appear when it lists the
> remote.
>
> So if you enable compression over an existing uncompressed backup and a volume is later recreated,
> the restore will see an empty remote and bring the volume back **empty** — and the next backup with
> `volumesync.delete=true` will then delete the existing backup from the destination. There is no
> code guard against this; point compression at a fresh path.

Note that files are only stored gzipped when that actually makes them smaller; incompressible or very
small files are stored as-is (with a `.bin` extension). rclone marks its compress backend as
experimental.

## Usage

### Docker Compose Example

Mount every volume you want to back up under `/volumes/<name>` in the `volumesync` container. Applications that depend on the restored data should use `depends_on: volumesync: condition: service_healthy`.

```yaml
services:
  # The single sync service
  volumesync:
    image: ghcr.io/dedalusj/docker-volume-sync:latest
    environment:
      - DESTINATION_PATH=s3:my-backup-bucket
      # Provide rclone backend configs
      - RCLONE_CONFIG_S3_TYPE=s3
      - RCLONE_CONFIG_S3_PROVIDER=AWS
      - RCLONE_CONFIG_S3_ENV_AUTH=true
      - RCLONE_CONFIG_S3_REGION=ap-southeast-4
      - AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID}
      - AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY}
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - db_data:/volumes/db_data
      - app_data:/volumes/app_data
    healthcheck:
      # Expect at least 2 volumes to be restored (db_data and app_data)
      test: ["CMD", "/app/volumesync", "health", "2"]
      interval: 10s
      retries: 30

  # An application with backup enabled
  db:
    image: postgres:13
    volumes:
      - db_data:/var/lib/postgresql/data
    labels:
      - volumesync.enabled=true
      - volumesync.volume=db_data
      - volumesync.schedule=0 3 * * *
      - volumesync.delete=true
    depends_on:
      volumesync:
        condition: service_healthy

  # Another application
  app:
    image: nginx:alpine
    volumes:
      - app_data:/usr/share/nginx/html
    labels:
      - volumesync.enabled=true
      - volumesync.volume=app_data
      - volumesync.schedule=@hourly
      - volumesync.stop=false # Don't stop nginx during backup
    depends_on:
      volumesync:
        condition: service_healthy

volumes:
  db_data:
    name: db_data
  app_data:
    name: app_data
```
