# Network shares & cloud folders — workaround

**Status:** configuring SMB/NFS/cloud sources inside the app is a non-goal for v1
(`GAP-001` in `docs/GAPS.md`). MyVideoStation indexes **local filesystem paths**;
a network share or cloud drive is made local by mounting it on the host, then
adding the mount point as a library folder.

This is the same model the Docker deployment already uses: the media root is a
bind mount passed to the container, and the app only ever sees normal files.

## Why it is not in-app

An in-app share browser needs a storage abstraction (SMB/NFS/S3/WebDAV/SFTP
clients, credential storage, reconnect/backoff, per-backend capability quirks,
partial reads, timeouts) plus a credential/secret UX and a second permission
model. It is a separate project from media management, and every supported
platform can already mount a share in a way that is more reliable and easier to
debug (`/etc/fstab`, `mount`, `docker compose`, TrueNAS/Synology UI).

## Recipes

### Linux host (NFS or CIFS)

```bash
# NFS
sudo apt install nfs-common
sudo mount -t nfs 192.168.1.10:/volume1/video /mnt/media

# SMB / CIFS
sudo apt install cifs-utils
sudo mount -t cifs //192.168.1.10/video /mnt/media \
  -o username=media,password=secret,uid=$(id -u),gid=$(id -g),iocharset=utf8,vers=3.0
```

Make it persistent in `/etc/fstab` (NFS):
`192.168.1.10:/volume1/video /mnt/media nfs defaults,_netdev 0 0`
CIFS: use a root-owned credentials file (`credentials=/etc/cifs-media.cred,uid=1000,gid=1000`)
instead of putting the password on the command line.

Then add `/mnt/media` (or a subfolder) as a library path in the UI.

### Docker / compose

Mount the share on the **host** first, then bind-mount it into the container
read-only:

```yaml
services:
  mvs:
    volumes:
      - /mnt/media:/media:ro
      - /volume1/docker/mvs:/data
```

Do not add a SMB/NFS volume driver to compose and expect the watcher to work:
inotify does not traverse those mounts reliably. The host mount is the supported
path.

### Cloud folders (rclone mount)

```bash
rclone mount gdrive:video /mnt/cloud --vfs-cache-mode writes --allow-other
```

Use `--allow-other` (plus `user_allow_other` in `/etc/fuse.conf`) when the
container runs as a non-root UID. Reading is fine; the app never writes to media
folders, so a read-mostly mount is enough. Latency and rate limits are the
provider's, and library scans will be slower than on local storage.

## What you give up (and what still works)

| Area | On a host-mounted share |
|---|---|
| Indexing, metadata, browsing, playback | Works like local files. |
| Live watcher (GAP-002) | Network filesystems (CIFS/SMB/NFS) and Docker Desktop shares are detected and skipped; use a scheduled scan interval (e.g. every hour) instead. |
| Transcoding | Works; HLS sessions write to `/data/cache`, never to the share. |
| Permissions | The container user must be able to read the mount; match UID/GID as described in the Synology section of the README. |
| Offline share | Scanner reports walk errors and marks files missing rather than failing the library. |

## What would change our mind

A real storage-backend abstraction with first-class SMB/NFS support (credentials,
reconnect, capability detection) and a matching watcher strategy. Until then,
host mounts are the supported workflow.
