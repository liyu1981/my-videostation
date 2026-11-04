@file:DependsOn("org.jellyfin.sdk:jellyfin-core-jvm:1.7.1")
@file:DependsOn("org.jetbrains.kotlinx:kotlinx-coroutines-core-jvm:1.11.0")
@file:Repository("https://repo1.maven.org/maven2/")

// Wholphin (Android TV) compatibility validator.
//
// Wholphin uses jellyfin-sdk-kotlin 1.7.1 (minimum server 10.10). This script
// walks the Jellyfin calls Wholphin makes (library list, home rows, detail,
// theme songs, persons, genres, cultures, playback, progress) so contract
// problems are visible before installing the TV app.
//
// Usage: ./run-wholphin.sh [url] [username] [password]
//   or set MVS_URL, MVS_USER, MVS_PASS.

import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.time.Duration
import kotlin.system.exitProcess
import kotlinx.coroutines.runBlocking
import org.jellyfin.sdk.createJellyfin
import org.jellyfin.sdk.model.ClientInfo
import org.jellyfin.sdk.model.DeviceInfo
import org.jellyfin.sdk.model.UUID
import org.jellyfin.sdk.model.api.AuthenticateUserByName
import org.jellyfin.sdk.model.api.BaseItemKind
import org.jellyfin.sdk.model.api.DeviceProfile
import org.jellyfin.sdk.model.api.ImageType
import org.jellyfin.sdk.model.api.MediaType
import org.jellyfin.sdk.model.api.PlaybackInfoDto
import org.jellyfin.sdk.api.client.extensions.genresApi
import org.jellyfin.sdk.api.client.extensions.imageApi
import org.jellyfin.sdk.api.client.extensions.itemsApi
import org.jellyfin.sdk.api.client.extensions.libraryApi
import org.jellyfin.sdk.api.client.extensions.localizationApi
import org.jellyfin.sdk.api.client.extensions.mediaInfoApi
import org.jellyfin.sdk.api.client.extensions.personsApi
import org.jellyfin.sdk.api.client.extensions.sessionApi
import org.jellyfin.sdk.api.client.extensions.systemApi
import org.jellyfin.sdk.api.client.extensions.tvShowsApi
import org.jellyfin.sdk.api.client.extensions.userApi
import org.jellyfin.sdk.api.client.extensions.userLibraryApi
import org.jellyfin.sdk.api.client.extensions.userViewsApi
import org.jellyfin.sdk.api.client.extensions.videosApi

val baseUrl = (args.getOrNull(0) ?: System.getenv("MVS_URL") ?: "http://127.0.0.1:6500").trimEnd('/')
val username = args.getOrNull(1) ?: System.getenv("MVS_USER") ?: "admin"
val password = args.getOrNull(2) ?: System.getenv("MVS_PASS") ?: "test-password-123"
val deviceId = "wholphin-validator"
val http = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(10)).build()

var failures = 0
var passes = 0

fun step(name: String, block: suspend () -> Unit) {
    val started = System.currentTimeMillis()
    try {
        runBlocking { block() }
        passes++
        println("PASS  $name (${System.currentTimeMillis() - started}ms)")
    } catch (t: Throwable) {
        failures++
        println()
        println("FAIL  $name")
        println("      ${t::class.qualifiedName}: ${t.message}")
        var cause = t.cause
        while (cause != null) {
            println("      cause: ${cause::class.qualifiedName}: ${cause.message}")
            cause = cause.cause
        }
        t.printStackTrace(System.out)
        println()
    }
}

fun httpGet(url: String, range: String? = null, token: String? = null): Triple<Int, Int, String> {
    val builder = HttpRequest.newBuilder().uri(URI(url)).timeout(Duration.ofSeconds(15)).GET()
    if (range != null) builder.header("Range", range)
    if (token != null) builder.header("X-Emby-Token", token)
    val res = http.send(builder.build(), HttpResponse.BodyHandlers.ofByteArray())
    return Triple(res.statusCode(), res.body().size, res.headers().firstValue("Content-Type").orElse(""))
}

fun httpPost(path: String, json: String, token: String): Int {
    val req = HttpRequest.newBuilder()
        .uri(URI("$baseUrl$path"))
        .timeout(Duration.ofSeconds(15))
        .header("Content-Type", "application/json")
        .header("X-Emby-Token", token)
        .POST(HttpRequest.BodyPublishers.ofString(json))
        .build()
    return http.send(req, HttpResponse.BodyHandlers.ofByteArray()).statusCode()
}

fun httpDelete(path: String, token: String): Int {
    val req = HttpRequest.newBuilder()
        .uri(URI("$baseUrl$path"))
        .timeout(Duration.ofSeconds(15))
        .header("X-Emby-Token", token)
        .DELETE()
        .build()
    return http.send(req, HttpResponse.BodyHandlers.ofByteArray()).statusCode()
}

println("== MyVideoStation Jellyfin/Wholphin validator ==")
println("server:   $baseUrl")
println("user:     $username")
println("sdk:      jellyfin-sdk-kotlin 1.7.1 (Wholphin)")
println()

val jellyfin = createJellyfin {
    clientInfo = ClientInfo(name = "Wholphin", version = "validator")
    deviceInfo = DeviceInfo(id = deviceId, name = "MVS Validator")
}
val api = jellyfin.createApi(baseUrl = baseUrl)

step("GET /System/Info/Public") {
    val info = api.systemApi.getPublicSystemInfo().content
    println("      server=${info.serverName} version=${info.version} product=${info.productName}")
    check(info.productName == "Jellyfin Server") { "unexpected productName" }
}

step("GET /Users/Public") {
    val users = api.userApi.getPublicUsers().content
    println("      users=${users.map { it.name }}")
}

var userId: UUID? = null
var accessToken: String? = null
step("POST /Users/AuthenticateByName") {
    val result = api.userApi.authenticateUserByName(
        data = AuthenticateUserByName(username = username, pw = password),
    ).content
    userId = result.user!!.id
    accessToken = result.accessToken!!
    println("      user=${result.user!!.name} id=${result.user!!.id} server=${result.serverId}")
    api.update(accessToken = accessToken)
}

fun requireLogin() = check(accessToken != null) { "login did not complete" }

step("GET /Users/Me + /Users/{id}") {
    requireLogin()
    val me = api.userApi.getCurrentUser().content
    val byId = api.userApi.getUserById(userId!!).content
    println("      me=${me.name} byId=${byId.name}")
}

var firstViewId: UUID? = null
step("GET /UserViews") {
    requireLogin()
    val views = api.userViewsApi.getUserViews().content
    views.items.forEach { println("      view=${it.name} type=${it.type} collectionType=${it.collectionType}") }
    check(views.items.isNotEmpty()) { "no views" }
    firstViewId = views.items.firstOrNull()?.id
}

var movieId: UUID? = null
step("GET /Items (movies in first view)") {
    requireLogin()
    val items = api.itemsApi.getItems(
        parentId = firstViewId,
        recursive = true,
        includeItemTypes = listOf(BaseItemKind.MOVIE),
        limit = 20,
    ).content
    println("      movies=${items.items.map { it.name }}")
    movieId = items.items.firstOrNull()?.id
}

var seriesId: UUID? = null
step("GET /Items (series, recursive)") {
    requireLogin()
    val items = api.itemsApi.getItems(
        includeItemTypes = listOf(BaseItemKind.SERIES),
        recursive = true,
        limit = 20,
    ).content
    println("      series=${items.items.map { it.name }}")
    seriesId = items.items.firstOrNull()?.id
}

step("GET /UserItems/Resume (exclude episodes)") {
    requireLogin()
    val resume = api.itemsApi.getResumeItems(
        limit = 50,
        mediaTypes = listOf(MediaType.VIDEO),
        excludeItemTypes = listOf(BaseItemKind.EPISODE),
    ).content
    println("      resume=${resume.items.map { it.name }}")
}

step("GET /Shows/NextUp") {
    requireLogin()
    val next = api.tvShowsApi.getNextUp(limit = 10).content
    println("      nextUp=${next.items.map { it.name }}")
}

step("GET /Shows/{id}/Episodes") {
    requireLogin()
    val id = seriesId ?: error("no series in library (add one to the test library)")
    val episodes = api.tvShowsApi.getEpisodes(seriesId = id, userId = userId, limit = 50).content
    println("      episodes=${episodes.items.map { it.name }}")
    check(episodes.items.isNotEmpty()) { "no episodes" }
}

var detail: org.jellyfin.sdk.model.api.BaseItemDto? = null
step("GET /Items/{id} (movie detail)") {
    requireLogin()
    val id = movieId ?: error("no movie in library")
    val item = api.userLibraryApi.getItem(itemId = id, userId = userId).content
    detail = item
    val source = item.mediaSources?.firstOrNull() ?: error("no mediaSources")
    check(!source.mediaStreams.isNullOrEmpty()) { "no mediaStreams" }
    source.mediaStreams!!.forEach { check(!it.codec.isNullOrBlank()) { "stream ${it.type}/${it.index} has no codec" } }
    if (!item.people.isNullOrEmpty()) {
        item.people!!.forEach { check(it.id.toString().isNotBlank()) { "person ${it.name} has no id" } }
    }
    println("      item=${item.name} sources=${item.mediaSources?.size} people=${item.people?.size} streams=${source.mediaStreams!!.map { "${it.type}:${it.codec}" }}")
}

step("GET /Items/{id}/ThemeSongs (ThemeMediaResult)") {
    requireLogin()
    val id = movieId ?: error("no movie")
    val songs = api.libraryApi.getThemeSongs(itemId = id, userId = userId).content
    println("      themeSongs=${songs.items.size} owner=${songs.ownerId}")
}

step("GET /Items/{id}/LocalTrailers + SpecialFeatures + Intros") {
    requireLogin()
    val id = movieId ?: error("no movie")
    val trailers = api.userLibraryApi.getLocalTrailers(id).content
    val features = api.userLibraryApi.getSpecialFeatures(id).content
    val intros = api.userLibraryApi.getIntros(id).content
    println("      trailers=${trailers.size} features=${features.size} intros=${intros.items.size}")
}

step("GET /Persons") {
    requireLogin()
    val persons = api.personsApi.getPersons(limit = 20).content
    println("      persons=${persons.items.map { it.name }}")
}

step("GET /Genres") {
    requireLogin()
    val genres = api.genresApi.getGenres(userId = userId).content
    println("      genres=${genres.items.map { it.name }}")
}

step("GET /Localization/Cultures") {
    requireLogin()
    val cultures = api.localizationApi.getCultures().content
    println("      cultures=${cultures.take(5).map { it.name }} (${cultures.size})")
    check(cultures.isNotEmpty()) { "no cultures" }
}

step("Findroid/Wholphin artwork URL") {
    val item = detail ?: error("no detail")
    val tag = item.imageTags?.get(ImageType.PRIMARY) ?: error("no primary image tag")
    val url = api.imageApi.getItemImageUrl(item.id, ImageType.PRIMARY, tag = tag, maxWidth = 400)
    val (status, bytes, ct) = httpGet(url)
    println("      url=$url status=$status bytes=$bytes type=$ct")
    check(status == 200) { "image status $status" }
}

step("GET /UserImage (404 expected: no avatars)") {
    requireLogin()
    val url = api.imageApi.getUserImageUrl(userId)
    val (status, _, _) = httpGet(url)
    println("      url=$url status=$status")
    check(status == 200 || status == 404) { "unexpected user image status $status" }
}

var streamUrl: String? = null
step("POST /Items/{id}/PlaybackInfo + stream URL") {
    requireLogin()
    val id = movieId ?: error("no movie")
    val response = api.mediaInfoApi.getPostedPlaybackInfo(
        itemId = id,
        data = PlaybackInfoDto(
            userId = userId,
            deviceProfile = DeviceProfile(
                name = "Wholphin validator",
                maxStaticBitrate = 1_000_000_000,
                maxStreamingBitrate = 1_000_000_000,
                directPlayProfiles = emptyList(),
                transcodingProfiles = emptyList(),
                containerProfiles = emptyList(),
                codecProfiles = emptyList(),
                subtitleProfiles = emptyList(),
            ),
            maxStreamingBitrate = 1_000_000_000,
            enableDirectPlay = true,
            enableDirectStream = true,
            enableTranscoding = true,
            allowVideoStreamCopy = true,
            allowAudioStreamCopy = true,
        ),
    ).content
    val source = response.mediaSources.firstOrNull() ?: error("no media sources")
    println("      source=${source.name} directPlay=${source.supportsDirectPlay} directStream=${source.supportsDirectStream} transcode=${source.supportsTranscoding}")
    streamUrl = api.videosApi.getVideoStreamUrl(id, static = true, mediaSourceId = source.id, tag = source.eTag)
    println("      stream=$streamUrl")
}

step("GET /Videos/{id}/stream (anonymous, range)") {
    val url = streamUrl ?: error("no stream URL")
    val (status, bytes, _) = httpGet(url, range = "bytes=0-1023")
    println("      status=$status bytes=$bytes")
    check(status == 200 || status == 206) { "stream status $status" }
}

step("POST /Sessions/Capabilities") {
    requireLogin()
    api.sessionApi.postCapabilities(
        playableMediaTypes = listOf(MediaType.VIDEO),
        supportsMediaControl = true,
        supportsPersistentIdentifier = true,
    )
    println("      accepted")
}

step("Playback reports + played/favorite (raw HTTP)") {
    requireLogin()
    val id = movieId ?: error("no movie")
    val token = accessToken!!
    val itemEnc = id.toString()
    val body = """{"ItemId":"$itemEnc","PositionTicks":0,"PlayMethod":"DirectPlay"}"""
    val start = httpPost("/Sessions/Playing", body, token)
    val progress = httpPost("/Sessions/Playing/Progress", body, token)
    val stop = httpPost("/Sessions/Playing/Stopped", body, token)
    val played = httpPost("/UserPlayedItems/$itemEnc", "{}", token)
    val unplayed = httpDelete("/UserPlayedItems/$itemEnc", token)
    val favorite = httpPost("/UserFavoriteItems/$itemEnc", "{}", token)
    val unfavorite = httpDelete("/UserFavoriteItems/$itemEnc", token)
    println("      start=$start progress=$progress stop=$stop played=$played/$unplayed favorite=$favorite/$unfavorite")
    check(start == 204 && progress == 204 && stop == 204) { "playback report statuses" }
    check(played == 204 && unplayed == 204 && favorite == 204 && unfavorite == 204) { "user-data statuses" }
}

step("POST /DisplayPreferences/usersettings") {
    requireLogin()
    val status = httpPost("/DisplayPreferences/usersettings?userId=$userId&client=Wholphin", """{"SortBy":"SortName"}""", accessToken!!)
    println("      status=$status")
    check(status == 204) { "display preferences status $status" }
}

step("GET /Items/{id}/Download (attachment)") {
    requireLogin()
    val id = movieId ?: error("no movie")
    val url = api.libraryApi.getDownloadUrl(id)
    val (status, bytes, ct) = httpGet(url, token = accessToken)
    println("      url=$url status=$status bytes=$bytes type=$ct")
    check(status == 200) { "download status $status" }
}

println()
println("== Summary: $passes passed, $failures failed ==")
if (failures > 0) {
    println("Wholphin contract check FAILED.")
    exitProcess(1)
}
println("Wholphin contract check PASSED.")
