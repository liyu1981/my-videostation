#!/bin/sh
# Container entrypoint.
#
# Some container UIs (notably Synology Container Manager) rewrite or drop the
# image's CMD when a container is created or edited. Keep the entrypoint a
# single script so that an empty command still starts the server instead of
# handing tini an empty argument list (which prints its usage and exits).
set -e

if [ "$#" -eq 0 ]; then
	set -- /usr/local/bin/mvs
fi

exec /sbin/tini -- "$@"
