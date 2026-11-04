@file:DependsOn("org.jellyfin.sdk:jellyfin-core-jvm:1.9.4")
@file:DependsOn("org.jetbrains.kotlinx:kotlinx-coroutines-core-jvm:1.11.0")
@file:Repository("https://repo1.maven.org/maven2/")

// Jellyfin/Findroid compatibility validator.
//
// Runs the same SDK version Findroid 1.2.0 uses (jellyfin-sdk-kotlin 1.9.4)
// against a server and walks Findroid's real call sequence. This catches
// deserialization/contract failures before installing the Android app.
//
// Usage: ./run.sh [url] [username] [password]
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
import org.jellyfin.sdk.model.api.AuthenticateUserByName
import org.jellyfin.sdk.model.api.BaseItemKind
import org.jellyfin.sdk.model.api.ClientCapabilitiesDto
import org.jellyfin.sdk.model.api.DeviceOptionsDto
import org.jellyfin.sdk.model.api.DeviceProfile
import org.jellyfin.sdk.model.api.GeneralCommandType
import org.jellyfin.sdk.model.api.MediaType
import org.jellyfin.sdk.model.api.PlaybackInfoDto
import org.jellyfin.sdk.model.api.SubtitleDeliveryMethod
import org.jellyfin.sdk.model.api.SubtitleProfile
import org.jellyfin.sdk.api.client.extensions.authenticationApi
import org.jellyfin.sdk.api.client.extensions.brandingApi
import org.jellyfin.sdk.api.client.extensions.deviceApi
import org.jellyfin.sdk.api.client.extensions.displayPreferenceApi
import org.jellyfin.sdk.api.client.extensions.libraryApi
import org.jellyfin.sdk.api.client.extensions.mediaInfoApi
import org.jellyfin.sdk.api.client.extensions.mediaSegmentApi
import org.jellyfin.sdk.api.client.extensions.sessionApi
import org.jellyfin.sdk.api.client.extensions.showApi
import org.jellyfin.sdk.api.client.extensions.suggestionApi
import org.jellyfin.sdk.api.client.extensions.systemApi
import org.jellyfin.sdk.api.client.extensions.userApi
import org.jellyfin.sdk.api.client.extensions.userDataApi
import org.jellyfin.sdk.api.client.extensions.userViewApi
import org.jellyfin.sdk.api.client.extensions.videoApi
import org.jellyfin.sdk.model.UUID

val baseUrl = (args.getOrNull(0) ?: System.getenv("MVS_URL") ?: "http://127.0.0.1:6500").trimEnd('/')
val username = args.getOrNull(1) ?: System.getenv("MVS_USER") ?: "admin"
val password = args.getOrNull(2) ?: System.getenv("MVS_PASS") ?: "test-password-123"
val deviceId = "mvs-validator"
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

fun httpPost(path: String, json: String): Pair<Int, Int> {
    val token = accessToken ?: error("no token")
    val req = HttpRequest.newBuilder()
        .uri(URI("$baseUrl$path"))
        .timeout(Duration.ofSeconds(15))
        .header("Content-Type", "application/json")
        .header("Authorization", "MediaBrowser Client=\"Findroid\", Device=\"Validator\", DeviceId=\"$deviceId\", Version=\"1.2.0\", Token=\"$token\"")
        .POST(HttpRequest.BodyPublishers.ofString(json))
        .build()
    val res = http.send(req, HttpResponse.BodyHandlers.ofByteArray())
    return res.statusCode() to res.body().size
}

fun httpGet(url: String, range: String? = null): Pair<Int, Int> {
    val builder = HttpRequest.newBuilder().uri(URI(url)).timeout(Duration.ofSeconds(15)).GET()
    if (range != null) builder.header("Range", range)
    val res = http.send(builder.build(), HttpResponse.BodyHandlers.ofByteArray())
    return res.statusCode() to res.body().size
}

println("== MyVideoStation Jellyfin/Findroid validator ==")
println("server:   $baseUrl")
println("user:     $username")
println("sdk:      jellyfin-sdk-kotlin 1.9.4 (Findroid 1.2.0)")
println()

val jellyfin = createJellyfin {
    clientInfo = ClientInfo(name = "Findroid", version = "1.2.0")
    deviceInfo = DeviceInfo(id = deviceId, name = "MVS Validator")
}
val api = jellyfin.createApi(baseUrl = baseUrl)

var publicInfo: org.jellyfin.sdk.model.api.PublicSystemInfo? = null
step("GET /System/Info/Public") {
    val info = api.systemApi.getPublicSystemInfo().content
    publicInfo = info
    println("      server=${info.serverName} version=${info.version} product=${info.productName}")
    check(info.productName == "Jellyfin Server") { "unexpected productName ${info.productName}" }
}

step("GET /Branding/Configuration") {
    val branding = api.brandingApi.getBrandingOptions().content
    println("      disclaimer=${branding.loginDisclaimer?.take(40)} splash=${branding.splashscreenEnabled}")
}

step("GET /QuickConnect/Enabled") {
    val enabled = api.authenticationApi.getQuickConnectEnabled().content
    println("      enabled=$enabled")
}

step("GET /Users/Public") {
    val users = api.userApi.getPublicUsers().content
    println("      users=${users.map { it.name }}")
}

var userId: UUID? = null
var accessToken: String? = null
step("POST /Users/AuthenticateByName (+ Findroid saveAuthenticationResult)") {
    val result = api.authenticationApi.authenticateUserByName(
        data = AuthenticateUserByName(username = username, pw = password),
    ).content
    // Exactly what Findroid's SetupRepositoryImpl.saveAuthenticationResult does.
    val id = result.user!!.id
    val name = result.user!!.name!!
    val serverId = result.serverId!!
    val token = result.accessToken!!
    println("      user=$name id=$id server=$serverId token=${token.take(6)}…")
    println("      policy.admin=${result.user!!.policy?.isAdministrator} session.active=${result.sessionInfo?.isActive}")
    userId = id
    accessToken = token
    api.update(accessToken = token)
}

fun requireLogin() {
    check(accessToken != null) { "login did not complete" }
}

step("GET /Users/Me") {
    requireLogin()
    val me = api.userApi.getCurrentUser().content
    println("      me=${me.name} id=${me.id}")
    check(me.id == userId) { "current user mismatch" }
}

step("GET /Items (bare: must be libraries, Findroid getLibraries)") {
    requireLogin()
    val libraries = api.libraryApi.getItems(userId).content.items
    libraries.forEach { println("      library=${it.name} type=${it.type} collectionType=${it.collectionType}") }
    check(libraries.isNotEmpty()) { "no libraries returned" }
    libraries.forEach {
        check(it.type == org.jellyfin.sdk.model.api.BaseItemKind.COLLECTION_FOLDER) {
            "bare /Items returned ${it.type} (${it.name}); clients treat it as a library"
        }
        check(it.collectionType != null) { "library ${it.name} has no collectionType" }
    }
}

step("GET /Items/Suggestions") {
    requireLogin()
    val suggestions = api.suggestionApi.getSuggestions(
        userId = userId,
        limit = 6,
        type = listOf(BaseItemKind.MOVIE, BaseItemKind.SERIES),
    ).content
    println("      suggestions=${suggestions.items.map { it.name }}")
    check(suggestions.items.isNotEmpty()) { "suggestions row is empty" }
}

var firstViewId: UUID? = null
step("GET /UserViews") {
    requireLogin()
    val views = api.userViewApi.getUserViews().content
    views.items.forEach { println("      view=${it.name} type=${it.collectionType} id=${it.id}") }
    firstViewId = views.items.firstOrNull()?.id
}

var firstItemId: UUID? = null
step("GET /Items/Latest (library row)") {
    requireLogin()
    val latest = api.libraryApi.getLatestMedia(
        parentId = firstViewId,
        limit = 20,
    ).content
    latest.forEach { println("      latest=${it.name} type=${it.type} id=${it.id}") }
    firstItemId = firstItemId ?: latest.firstOrNull()?.id
}

step("GET /Items/Resume") {
    requireLogin()
    val resume = api.libraryApi.getResumeItems(limit = 50, mediaTypes = listOf(MediaType.VIDEO)).content
    println("      resume=${resume.items.map { it.name }}")
    firstItemId = firstItemId ?: resume.items.firstOrNull()?.id
}

step("GET /Shows/NextUp") {
    requireLogin()
    val next = api.showApi.getNextUp(limit = 10).content
    println("      nextUp=${next.items.map { it.name }}")
}

step("GET /Items (movie library)") {
    requireLogin()
    val items = api.libraryApi.getItems(
        parentId = firstViewId,
        recursive = true,
        includeItemTypes = listOf(BaseItemKind.MOVIE),
        limit = 20,
    ).content
    println("      items=${items.items.map { it.name }}")
    firstItemId = firstItemId ?: items.items.firstOrNull()?.id
}

var detail: org.jellyfin.sdk.model.api.BaseItemDto? = null
step("GET /Items/{id}") {
    requireLogin()
    val id = firstItemId ?: error("no item available (library empty?)")
    val item = api.libraryApi.getItem(itemId = id, userId = userId).content
    detail = item
    println("      item=${item.name} type=${item.type} mediaSources=${item.mediaSources?.size} people=${item.people?.size}")
    val source = item.mediaSources?.firstOrNull() ?: error("detail has no mediaSources (play button would be hidden)")
    check(!source.mediaStreams.isNullOrEmpty()) { "source has no mediaStreams" }
    source.mediaStreams!!.forEach { stream ->
        check(!stream.codec.isNullOrBlank()) { "stream ${stream.type}/${stream.index} has no codec" }
    }
    println("      streams=${source.mediaStreams!!.map { "${it.type}:${it.codec}" }}")
    if (!item.people.isNullOrEmpty()) {
        item.people!!.forEach { person ->
            check(person.id.toString().isNotBlank()) { "person ${person.name} has no id" }
        }
        println("      people=${item.people!!.map { it.name }}")
    }
}

// Findroid builds image URLs itself, with a lowercase "items" prefix and a tag
// query parameter (see FindroidImages.kt). This is exactly what Coil fetches.
step("Findroid artwork URLs (lowercase items/Images + tag)") {
    val item = detail ?: error("no item detail")
    val primaryTag = item.imageTags?.get(org.jellyfin.sdk.model.api.ImageType.PRIMARY)
    if (primaryTag == null) {
        println("      no Primary image tag on the item")
    } else {
        val url = "$baseUrl/items/${item.id}/Images/Primary?tag=$primaryTag"
        val (status, bytes) = httpGet(url)
        println("      primary=$url status=$status bytes=$bytes")
        check(status == 200) { "primary image status $status" }
    }
    val backdropTag = item.backdropImageTags?.firstOrNull()
    if (backdropTag != null) {
        val url = "$baseUrl/items/${item.id}/Images/Backdrop/0?tag=$backdropTag"
        val (status, bytes) = httpGet(url)
        println("      backdrop=$url status=$status bytes=$bytes")
        check(status == 200) { "backdrop image status $status" }
    } else {
        println("      no Backdrop image tag on the item")
    }
}

step("GET /UserItems/{id}/UserData") {
    requireLogin()
    val id = firstItemId ?: error("no item available")
    val data = api.userDataApi.getItemUserData(itemId = id).content
    println("      played=${data.played} ticks=${data.playbackPositionTicks} favorite=${data.isFavorite}")
}

step("GET /MediaSegments/{id}") {
    requireLogin()
    val id = firstItemId ?: error("no item available")
    val segments = api.mediaSegmentApi.getItemSegments(id).content
    println("      segments=${segments.items.size}")
}

var streamUrl: String? = null
step("POST /Items/{id}/PlaybackInfo + stream URL") {
    requireLogin()
    val id = firstItemId ?: error("no item available")
    val response = api.mediaInfoApi.getPostedPlaybackInfo(
        itemId = id,
        data = PlaybackInfoDto(
            userId = userId,
            deviceProfile = DeviceProfile(
                name = "Direct play all",
                maxStaticBitrate = 1_000_000_000,
                maxStreamingBitrate = 1_000_000_000,
                codecProfiles = emptyList(),
                containerProfiles = emptyList(),
                directPlayProfiles = emptyList(),
                transcodingProfiles = emptyList(),
                subtitleProfiles = listOf(
                    SubtitleProfile("srt", SubtitleDeliveryMethod.EXTERNAL),
                    SubtitleProfile("ass", SubtitleDeliveryMethod.EXTERNAL),
                ),
            ),
            maxStreamingBitrate = 1_000_000_000,
        ),
    ).content
    response.mediaSources?.forEach {
        println("      source=${it.name} protocol=${it.protocol} container=${it.container} direct=${it.supportsDirectPlay}")
    }
    val source = response.mediaSources?.firstOrNull() ?: error("no media sources")
    val url = api.videoApi.getVideoStreamUrl(id, static = true, mediaSourceId = source.id)
    streamUrl = url
    println("      stream=$url")
}

step("GET /Videos/{id}/stream (anonymous, range)") {
    val url = streamUrl ?: error("no stream URL")
    val (status, bytes) = httpGet(url, range = "bytes=0-1023")
    println("      status=$status bytes=$bytes")
    check(status == 200 || status == 206) { "unexpected stream status $status" }
}

step("GET /Items/{id}/Images/Primary (anonymous)") {
    val id = firstItemId ?: error("no item available")
    val (status, bytes) = httpGet("$baseUrl/Items/$id/Images/Primary")
    println("      status=$status bytes=$bytes")
    check(status == 200) { "unexpected image status $status" }
}

step("POST /Sessions/Capabilities/Full") {
    requireLogin()
    api.sessionApi.postCapabilities(
        playableMediaTypes = listOf(MediaType.VIDEO),
        supportedCommands = listOf(GeneralCommandType.DISPLAY_MESSAGE),
        supportsMediaControl = true,
        supportsPersistentIdentifier = true,
    )
    println("      accepted")
}

step("GET /DisplayPreferences/usersettings") {
    requireLogin()
    val prefs = api.displayPreferenceApi.getDisplayPreferences(
        displayPreferencesId = "usersettings",
        userId = userId,
        client = "Findroid",
    ).content
    println("      sortBy=${prefs.sortBy} sortOrder=${prefs.sortOrder}")
}

step("POST /Devices/Options") {
    requireLogin()
    val (status, _) = httpPost("/Devices/Options", """{"CustomName":"MVS Validator"}""")
    println("      status=$status")
    check(status == 204) { "unexpected status $status" }
}

step("POST /Sessions/Playing + /Stopped") {
    requireLogin()
    val id = firstItemId ?: error("no item available")
    val body = """{"ItemId":"$id","PositionTicks":0,"PlayMethod":"DirectPlay"}"""
    val (startStatus, _) = httpPost("/Sessions/Playing", body)
    val (progressStatus, _) = httpPost("/Sessions/Playing/Progress", body)
    val (stopStatus, _) = httpPost("/Sessions/Playing/Stopped", body)
    println("      start=$startStatus progress=$progressStatus stop=$stopStatus")
    check(startStatus == 204 && progressStatus == 204 && stopStatus == 204) { "unexpected statuses" }
}

println()
println("== Summary: $passes passed, $failures failed ==")
if (failures > 0) {
    println("Server contract check FAILED — fix the reported steps before testing the Android app.")
    exitProcess(1)
}
println("Server contract check PASSED. Install/launch Findroid against $baseUrl.")
