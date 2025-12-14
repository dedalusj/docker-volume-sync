# Docker Volume S3 Sync

A Go-based sidecar container to synchronize a local Docker volume with an AWS S3 bucket. It supports initial restoration from S3 on startup and scheduled backups to S3, with optional container stopping to ensure data consistency.

## Features

- **Bi-directional Sync**:
  - **Restore**: On startup, if a sentinel file is missing, it downloads the S3 content to the volume.
  - **Backup**: Periodically syncs the volume content to S3.
- **Safe Backups**: Can temporarily stop containers attached to the volume during backup to ensure data integrity (using the Docker API).
- **Concurrent Transfers**: Uses multiple concurrent workers for faster uploads and downloads.
- **Pruning**: Option to delete files in the destination that are no longer present in the source.
- **Cron Scheduling**: Flexible backup scheduling using cron expressions.

## detailed Configuration

The application is configured using environment variables.

| Variable | Description | Default | Required |
| :--- | :--- | :--- | :--- |
| `S3_PATH` | The destination S3 URI (e.g., `s3://my-bucket/backups/data`). | - | **Yes** |
| `SYNC_SCHEDULE` | Cron expression for the backup schedule (e.g., `@daily`, `0 3 * * *`). | - | **Yes** |
| `VOLUME_PATH` | The path inside the container where the volume is mounted. | `/data` | No |
| `VOLUME_NAME` | The name of the Docker volume to manage. If set, containers attached to this volume will be stopped during backup. | - | No |
| `DOCKER_STOP_GRACE_PERIOD` | Human-readable duration to wait when stopping containers (e.g., `30s`, `1m`). | `2m` | No |
| `SYNC_DELETE` | If set to `true`, files deleted in the volume will be deleted from S3 during backup (and vice-versa during restore). | `false` | No |
| `SYNC_CONCURRENCY` | Number of concurrent file transfers. | `16` | No |
| `AWS_REGION` | AWS Region (standard AWS SDK). | - | Yes |
| `AWS_ACCESS_KEY_ID` | AWS Access Key ID (standard AWS SDK). | - | Yes |
| `AWS_SECRET_ACCESS_KEY` | AWS Secret Access Key (standard AWS SDK). | - | Yes |

## Usage

### Docker Compose Example

Add `s3sync` as a service in your `docker-compose.yml`. Ensure it mounts the same volume as your application and has access to the Docker socket if you want it to stop/start containers.

```yaml
version: '3.8'

services:
  # Your main application
  db:
    image: postgres:13
    volumes:
      - db_data:/var/lib/postgresql/data
    restart: always

  # The sidecar sync service
  backup:
    build: .
    environment:
      - S3_PATH=s3://my-company-backups/postgres
      - SYNC_SCHEDULE=0 3 * * *  # Run at 3 AM daily
      - VOLUME_NAME=my_project_db_data
      - VOLUME_PATH=/data
      - DOCKER_STOP_GRACE_PERIOD=30s
      - SYNC_DELETE=true
      - AWS_REGION=us-west-2
      - AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID}
      - AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY}
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock # Required to stop/start containers
      - db_data:/data # Mount the shared volume to /data (default VOLUME_PATH)
    depends_on:
      - db

volumes:
  db_data:
    name: my_project_db_data
```

### How it Works

1.  **Startup**:
    *   The container starts and checks for a sentinel file (`.s3sync_done`) in `VOLUME_PATH`.
    *   **If missing**: It assumes this is a fresh install. It performs a **Restore** (S3 -> Volume). After success, it creates the sentinel file.
    *   **If present**: It skips the restore step.

2.  **Scheduled Loop**:
    *   The internal cron scheduler waits for the next configured time.
    *   **Stop Containers**: If `VOLUME_NAME` is set, it queries the Docker API for all running containers using that volume and stops them.
    *   **Backup**: It syncs `VOLUME_PATH` -> `S3_PATH`.
    *   **Start Containers**: It restarts the containers that were stopped.

## Building

```bash
docker build -t s3sync .
```
