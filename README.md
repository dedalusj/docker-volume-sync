# Docker Volume Sync

A single container service to synchronize multiple Docker volumes with any remote storage provider supported by `rclone` (AWS S3, Google Drive, Backblaze B2, Azure, etc.). It discovers volumes to back up via Docker labels, restores them on startup, and schedules periodic backups.

## Features

- **Single Container**: One `volumesync` container can manage multiple volumes across different services.
- **Label-based Discovery**: Enable backup and configure schedules via Docker labels on your application containers.
- **Universal Support**: Uses `rclone` under the hood to support 50+ backend storages.
- **Safe Backups**: Optionally stops containers attached to the volume during backup to ensure data integrity.
- **Internal Healthcheck**: The service is only marked healthy once ALL discovered volumes have completed their initial sync.

## Configuration

### Global Environment Variables (on `volumesync` container)

| Variable | Description | Default | Required |
| :--- | :--- | :--- | :--- |
| `DESTINATION_PATH` | The destination URI according to rclone syntax (e.g., `s3://my-bucket/backups`). | - | **Yes** |

*Note: You must also provide rclone credentials for your `DESTINATION_PATH` via standard rclone environment variables (e.g., `RCLONE_CONFIG_MYREMOTE_TYPE=s3`).*

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

## Usage

### Docker Compose Example

Mount every volume you want to back up under `/volumes/<name>` in the `volumesync` container. Applications that depend on the restored data should use `depends_on: volumesync: condition: service_healthy`.

```yaml
services:
  # The single sync service
  volumesync:
    image: ghcr.io/dedalusj/docker-volume-sync:latest
    environment:
      - DESTINATION_PATH=s3://my-backup-bucket
      # Provide rclone backend configs
      - AWS_REGION=us-west-2
      - AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID}
      - AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY}
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - db_data:/volumes/db_data
      - app_data:/volumes/app_data
    healthcheck:
      test: ["CMD", "/app/volumesync", "health"]
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
