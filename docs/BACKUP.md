# Backup & restore

Everything MyVideoStation owns lives in **one data directory** (`--data`, or
`MVS_DATA_DIR`; `/data` in the Docker image):

| path | contents |
|---|---|
| `mvs.db` (+ `mvs.db-wal` / `mvs.db-shm` while running) | libraries, items, files, metadata, people, genres, locks, collections, watch state, settings (including the TMDb API key) |
| `cache/images/` | cached posters, backdrops and episode stills (content-addressed; safe to rebuild, but large) |

**Media files are not in the data directory.** They stay in your read-only mounts
(`/media/...`, NAS paths) and must be backed up separately.

## Same path (supported, simplest)

Stop the server first, then copy the **whole** data directory:

```bash
# bare binary / compose bind mount
systemctl stop mvs            # or: docker compose -f deployments/docker-compose.yml down
cp -a /path/to/data "/backup/mvs-data-$(date +%F)"
```

Restore to the **same absolute path**, preserving ownership:

```bash
systemctl stop mvs
rm -rf /path/to/data
cp -a /backup/mvs-data-2026-11-01 /path/to/data
systemctl start mvs
```

If you copy `mvs.db` on its own while the server is running you can miss recent
writes (SQLite runs in WAL mode). Either stop the server (the copy then includes
`mvs.db-wal`/`mvs.db-shm`) or take an online snapshot:

```bash
mkdir -p /backup/mvs-data-2026-11-01
sqlite3 data/mvs.db ".backup '/backup/mvs-data-2026-11-01/mvs.db'"
cp -a data/cache /backup/mvs-data-2026-11-01/
```

`sqlite3 ... ".backup ..."` (or `VACUUM INTO`) is consistent while the server runs;
the copied DB is self-contained (no WAL needed).

## Docker (compose volume)

The compose file uses the `mvs-data` volume mounted at `/data`. Use `docker compose run`
so the project's actual volume name is used (the host volume is usually
`<compose-project>_mvs-data`; `docker volume ls` shows it):

```bash
# stop the app, then archive the volume (root so the bind-mounted backup is writable)
docker compose -f deployments/docker-compose.yml down
mkdir -p backup
docker compose -f deployments/docker-compose.yml run --rm --no-deps --user root \
  -v "$PWD/backup:/backup" --entrypoint sh myvideostation \
  -c 'tar czf /backup/mvs-data.tgz -C /data .'

# restore (stop first), then fix ownership for the non-root mvs user
docker compose -f deployments/docker-compose.yml run --rm --no-deps --user root \
  -v "$PWD/backup:/backup" --entrypoint sh myvideostation \
  -c 'rm -rf /data/* && tar xzf /backup/mvs-data.tgz -C /data && chown -R mvs:mvs /data'
docker compose -f deployments/docker-compose.yml up -d
```

`docker cp` works too if the container is stopped:
`docker cp myvideostation:/data ./backup`.

## Moving to a different path or host

- Copy the **whole** data directory, including `cache/`.
- Cached artwork paths in the DB are absolute. At startup the app re-points cached
  images whose basename exists under the new `<data>/cache/images/`, so a data-dir
  move keeps posters/backdrops working.
- **Media paths are absolute too** (`media_files.path`, library paths). Keep the
  same mounts (e.g. `/media/movies`, `/mnt/nas/video/movie`) or update the library
  paths afterwards; otherwise files show as missing until the next scan.
- If you restore only the DB and drop `cache/`, metadata is intact but artwork
  returns 404 until you refresh/enrich or rescan (sidecar/vsmeta art is
  re-imported, TMDb art is re-downloaded).

## Verify a restore

```bash
sqlite3 data/mvs.db "PRAGMA integrity_check;"   # expect: ok
./bin/mvs --listen 127.0.0.1:6500 --data ./data
curl -fsS http://127.0.0.1:6500/api/v1/health
```

Open the UI and check a few posters/backdrops. If the media moved, update the
library paths and rescan.

## Notes

- The DB contains the TMDb API key; treat backups as secrets.
- The cache can be large (tens of GB with many libraries). It is safe to exclude
  it from a quick backup and let the app rebuild it, but posters will 404 until
  re-enriched.
- The release binary and Docker image are stateless apart from this directory.
