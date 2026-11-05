#!/usr/bin/env bash
# Moonfin contract validator.
#
# Moonfin (https://github.com/Moonfin-Client/Moonfin-Core) is a Flutter client
# with its own Dart Jellyfin API layer, so it is more permissive than the Kotlin
# SDK clients but calls a slightly different path set (notably
# /Users/{userId}/Items/{itemId} for detail and Quick Connect during login).
#
# This script walks Moonfin's real request sequence with curl + jq and checks
# status codes and the response shapes its parsers read. No Dart/Flutter needed.
set -euo pipefail

BASE="${1:-${MVS_URL:-}}"
USER_NAME="${2:-${MVS_USER:-}}"
PASS="${3:-${MVS_PASS:-}}"
if [ -z "$BASE" ] || [ -z "$USER_NAME" ] || [ -z "$PASS" ]; then
	echo "usage: $0 <server-url> <user> <password>   (or MVS_URL/MVS_USER/MVS_PASS)" >&2
	exit 2
fi
BASE="${BASE%/}"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

PASSED=0
FAILED=0
TOKEN=""
SERVER_ID=""
CLIENT_ID="moonfin-validator-$$"
AUTH=()

pass() { PASSED=$((PASSED + 1)); printf '  \033[32mPASS\033[0m %s\n' "$1"; }
fail() { FAILED=$((FAILED + 1)); printf '  \033[31mFAIL\033[0m %s: %s\n' "$1" "$2"; }

# req NAME METHOD PATH [BODY] [EXTRA-CURL...]
# Writes the body to $TMP/<slug>.json and sets $CODE.
req() {
	local name="$1" method="$2" path="$3" body="${4:-}"
	local out="$TMP/$(printf '%s' "$name" | tr -c 'A-Za-z0-9' '_').out"
	local args=(-sS -o "$out" -w '%{http_code}' -X "$method" -H "Authorization: ${AUTH_HDR:-}")
	if [ -n "${TOKEN:-}" ]; then
		args+=(-H "X-Emby-Token: $TOKEN")
	fi
	if [ -n "$body" ]; then
		args+=(-H 'Content-Type: application/json' --data-binary "$body")
	fi
	CODE="$(curl "${args[@]}" "$BASE$path")"
	BODY_FILE="$out"
}

expect() { # NAME EXPECTED_CODE
	if [ "$CODE" = "$2" ]; then pass "$1 (HTTP $CODE)"; else fail "$1" "expected HTTP $2, got $CODE: $(head -c 300 "$BODY_FILE" 2>/dev/null)"; fi
}

expect_jq() { # NAME FILTER
	if jq -e "$2" "$BODY_FILE" >/dev/null 2>&1; then
		pass "$1"
	else
		fail "$1" "jq '$2' failed: $(head -c 300 "$BODY_FILE" 2>/dev/null)"
	fi
}

AUTH_HDR='MediaBrowser Client="Moonfin", Device="Validator", DeviceId="'"$CLIENT_ID"'", Version="3.0.0"'

echo "== Moonfin contract: $BASE =="

# ---- discovery -------------------------------------------------------------
req public GET '/System/Info/Public'
expect 'GET /System/Info/Public' 200
expect_jq 'public info: Version + ProductName + Id' \
	'.Version and .ProductName == "Jellyfin Server" and (.Id | length > 0) and .ServerName'

req ping GET '/System/Ping'
expect 'GET /System/Ping' 200

# Moonfin assumes Quick Connect exists for every Jellyfin server. A 401 is what
# a server with the feature disabled returns, and makes Moonfin switch to the
# password form instead of retrying the code flow.
req qc-init POST '/QuickConnect/Initiate'
expect 'POST /QuickConnect/Initiate (disabled -> 401)' 401

req branding GET '/Branding/Configuration'
expect 'GET /Branding/Configuration' 200

req users-public GET '/Users/Public'
expect 'GET /Users/Public' 200
expect_jq 'public users is an array' 'type == "array"'

# ---- login -----------------------------------------------------------------
req login POST '/Users/AuthenticateByName' \
	"$(jq -nc --arg u "$USER_NAME" --arg p "$PASS" '{Username:$u,Pw:$p}')"
expect 'POST /Users/AuthenticateByName' 200
expect_jq 'login: AccessToken + User.Id + ServerId' \
	'.AccessToken and (.User.Id | length > 0) and .ServerId'
TOKEN="$(jq -r '.AccessToken // empty' "$BODY_FILE")"
USER_ID="$(jq -r '.User.Id // empty' "$BODY_FILE")"
SERVER_ID="$(jq -r '.ServerId // empty' "$BODY_FILE")"
if [ -z "$TOKEN" ] || [ -z "$USER_ID" ]; then
	echo "login did not return a token/user id; cannot continue" >&2
	exit 1
fi
AUTH_HDR='MediaBrowser Client="Moonfin", Device="Validator", DeviceId="'"$CLIENT_ID"'", Version="3.0.0", Token="'"$TOKEN"'"'

req me GET '/Users/Me'
expect 'GET /Users/Me' 200
expect_jq 'user: Id/Name/Policy/Configuration' \
	'.Id and .Name and .Policy and .Configuration'

# ---- views and home rows ---------------------------------------------------
req views GET '/UserViews?includeHidden=false'
expect 'GET /UserViews' 200
expect_jq 'views: Items array' '.Items | type == "array"'
LIB="$(jq -r '.Items[0].Id // empty' "$BODY_FILE")"

req latest GET "/Users/$USER_ID/Items/Latest?Limit=12"
expect 'GET /Users/{id}/Items/Latest' 200
expect_jq 'latest: array (Moonfin wraps it)' 'type == "array"'

req resume GET '/UserItems/Resume?Limit=12&IncludeItemTypes=Movie,Episode&MediaTypes=Video'
expect 'GET /UserItems/Resume' 200
expect_jq 'resume: Items array' '.Items | type == "array"'

req nextup GET "/Shows/NextUp?UserId=$USER_ID&Limit=12"
expect 'GET /Shows/NextUp' 200
expect_jq 'next up: Items array' '.Items | type == "array"'

# ---- library browse --------------------------------------------------------
ITEM=""
if [ -n "$LIB" ]; then
	req browse GET "/Users/$USER_ID/Items?ParentId=$LIB&Recursive=true&IncludeItemTypes=Movie,Episode&Limit=24&Fields=ItemCounts,PrimaryImageTag,ImageTags,BackdropImageTags&SortBy=SortName&SortOrder=Ascending"
	expect 'GET /Users/{id}/Items (library)' 200
	expect_jq 'browse: Items array' '.Items | type == "array"'
	ITEM="$(jq -r '.Items[0].Id // empty' "$BODY_FILE")"
else
	fail 'library browse' 'no user views returned; seed a library first'
fi

# ---- detail through the user-scoped path (Moonfin uses this one) -----------
if [ -n "$ITEM" ]; then
	req detail GET "/Users/$USER_ID/Items/$ITEM?Fields=Chapters,MediaSources,MediaStreams,People,Overview,Genres,UserData,RecursiveItemCount,ChildCount,DateCreated"
	expect 'GET /Users/{id}/Items/{itemId}' 200
	expect_jq 'detail: Id + Type + MediaSources + video codec' \
		'.Id and .Type and (.MediaSources | type == "array") and
		 (.MediaSources[0].MediaStreams | map(select(.Type == "Video")) | length > 0)'

	req ancestors GET "/Items/$ITEM/Ancestors"
	expect 'GET /Items/{id}/Ancestors' 200
	expect_jq 'ancestors: array' 'type == "array"'

	req trailers GET "/Users/$USER_ID/Items/$ITEM/LocalTrailers"
	expect 'GET /Users/{id}/Items/{itemId}/LocalTrailers' 200
	expect_jq 'trailers: array' 'type == "array"'

	req intros GET "/Users/$USER_ID/Items/$ITEM/Intros"
	expect 'GET /Users/{id}/Items/{itemId}/Intros' 200
	expect_jq 'intros: Items array' 'if type == "array" then true else (.Items | type == "array") end'

	req similar GET "/Items/$ITEM/Similar?Limit=6"
	expect 'GET /Items/{id}/Similar' 200
	expect_jq 'similar: Items array' '.Items | type == "array"'

	req theme GET "/Items/$ITEM/ThemeMedia?InheritFromParent=true"
	expect 'GET /Items/{id}/ThemeMedia' 200
	expect_jq 'theme media: ThemeSongsResult.Items' '.ThemeSongsResult.Items | type == "array"'

	req features GET "/Items/$ITEM/SpecialFeatures"
	expect 'GET /Items/{id}/SpecialFeatures' 200
	expect_jq 'special features: array (or Items array)' 'if type == "array" then true else (.Items | type == "array") end'

	req segments GET "/MediaSegments/$ITEM"
	expect 'GET /MediaSegments/{id}' 200
	expect_jq 'segments: Items array' '.Items | type == "array"'

	# Moonfin builds image URLs without auth headers.
	img_code="$(curl -sS -o "$TMP/image.bin" -w '%{http_code}' "$BASE/Items/$ITEM/Images/Primary?maxWidth=300&quality=90")"
	CODE="$img_code"; BODY_FILE="$TMP/image.bin"
	expect 'GET /Items/{id}/Images/Primary (anonymous)' 200
	if [ -s "$TMP/image.bin" ]; then pass 'image body is not empty'; else fail 'image body' 'empty'; fi

	req user-image GET "/Users/$USER_ID/Images/Primary"
	expect 'GET /Users/{id}/Images/Primary (no avatars -> 404)' 404

	# ---- playback ----------------------------------------------------------
	req playback POST "/Items/$ITEM/PlaybackInfo?UserId=$USER_ID" \
		'{"EnableDirectPlay":true,"EnableDirectStream":true,"EnableTranscoding":true,"AllowVideoStreamCopy":true,"AllowAudioStreamCopy":true,"AutoOpenLiveStream":true,"DeviceProfile":{"MaxStreamingBitrate":120000000,"DirectPlayProfiles":[{"Container":"mkv,mp4","Type":"Video"}]}}'
	expect 'POST /Items/{id}/PlaybackInfo' 200
	expect_jq 'playback: MediaSources + Id + streams + PlaySessionId' \
		'.PlaySessionId and (.MediaSources | length > 0) and (.MediaSources[0].Id | length > 0) and
		 (.MediaSources[0].MediaStreams | type == "array")'
	SRC="$(jq -r '.MediaSources[0].Id // empty' "$BODY_FILE")"
	PLAY_SESSION="$(jq -r '.PlaySessionId // empty' "$BODY_FILE")"

	stream_code="$(curl -sS -o /dev/null -w '%{http_code}' -r 0-1023 \
		"$BASE/Videos/$ITEM/stream?Static=true&MediaSourceId=$SRC")"
	if [ "$stream_code" = 200 ] || [ "$stream_code" = 206 ]; then
		pass "GET /Videos/{id}/stream (anonymous range, HTTP $stream_code)"
	else
		fail 'GET /Videos/{id}/stream' "HTTP $stream_code"
	fi

	# ---- user state --------------------------------------------------------
	req userdata POST "/UserItems/$ITEM/UserData" \
		'{"PlaybackPositionTicks":123450000,"LastPlayedDate":"2026-01-01T00:00:00.000Z"}'
	expect 'POST /UserItems/{id}/UserData' 200
	expect_jq 'user data: position persisted' '.PlaybackPositionTicks == 123450000'

	req userdata-get GET "/UserItems/$ITEM/UserData"
	expect 'GET /UserItems/{id}/UserData' 200

	req rating POST "/UserItems/$ITEM/Rating?Likes=true"
	expect 'POST /UserItems/{id}/Rating' 204
	req rating-del DELETE "/UserItems/$ITEM/Rating"
	expect 'DELETE /UserItems/{id}/Rating' 204

	req played POST "/UserPlayedItems/$ITEM"
	expect 'POST /UserPlayedItems/{id}' 204
	req played-del DELETE "/UserPlayedItems/$ITEM"
	expect 'DELETE /UserPlayedItems/{id}' 204

	req fav POST "/UserFavoriteItems/$ITEM"
	expect 'POST /UserFavoriteItems/{id}' 204
	req fav-del DELETE "/UserFavoriteItems/$ITEM"
	expect 'DELETE /UserFavoriteItems/{id}' 204

	# ---- session / progress ------------------------------------------------
	req capabilities POST '/Sessions/Capabilities/Full' \
		'{"PlayableMediaTypes":["Video"],"SupportedCommands":[],"SupportsMediaControl":true}'
	expect 'POST /Sessions/Capabilities/Full' 204

	req start POST '/Sessions/Playing' \
		"$(jq -nc --arg id "$ITEM" --arg ps "$PLAY_SESSION" --arg src "$SRC" \
			'{ItemId:$id,PlaySessionId:$ps,MediaSourceId:$src,PositionTicks:0,PlayMethod:"DirectPlay",CanSeek:true}')"
	expect 'POST /Sessions/Playing' 204

	req progress POST '/Sessions/Playing/Progress' \
		"$(jq -nc --arg id "$ITEM" --arg ps "$PLAY_SESSION" \
			'{ItemId:$id,PlaySessionId:$ps,PositionTicks:10000000,IsPaused:false}')"
	expect 'POST /Sessions/Playing/Progress' 204

	req stopped POST '/Sessions/Playing/Stopped' \
		"$(jq -nc --arg id "$ITEM" --arg ps "$PLAY_SESSION" \
			'{ItemId:$id,PlaySessionId:$ps,PositionTicks:20000000}')"
	expect 'POST /Sessions/Playing/Stopped' 204
else
	fail 'item endpoints' 'no item found in the library; seed media first'
fi

# ---- preferences / localization / misc -------------------------------------
req display GET "/DisplayPreferences/usersettings?userId=$USER_ID&client=Moonfin"
expect 'GET /DisplayPreferences/{name}' 200

req device-options POST '/Devices/Options' '{"CustomPrefs":{}}'
expect 'POST /Devices/Options' 204

req user-config POST '/Users/Configuration' '{"GroupedFolders":[],"SubtitleMode":"Default"}'
expect 'POST /Users/Configuration' 204

for path in /Localization/Cultures /Localization/Countries /Localization/Options /Localization/ParentalRatings; do
	req "loc-${path##*/}" GET "$path"
	expect "GET $path" 200
	expect_jq "$path is an array" 'type == "array"'
done

req artists GET "/Artists?UserId=$USER_ID&Limit=5"
expect 'GET /Artists' 200
expect_jq 'artists: Items array' '.Items | type == "array"'

# ---- logout ----------------------------------------------------------------
req logout POST '/Sessions/Logout'
expect 'POST /Sessions/Logout' 204

req after-logout GET '/Users/Me'
expect 'revoked token rejected after logout' 401

echo
if [ "$FAILED" -eq 0 ]; then
	printf '\033[32m== Moonfin contract: %d passed, 0 failed ==\033[0m\n' "$PASSED"
	exit 0
fi
printf '\033[31m== Moonfin contract: %d passed, %d failed ==\033[0m\n' "$PASSED" "$FAILED"
exit 1
