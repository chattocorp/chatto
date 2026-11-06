//! A small asynchronous client for Chatto's public Connect JSON API.
//!
//! Calls require a Tokio runtime. Hosts own UI and credential persistence.
//! Optional realtime projections own cancellable network tasks. Drop an HTTP
//! request future to cancel it. Response models expose a subset of the API. Raw timeline
//! events remain available for consumers that need other protocol data.

use std::{
    collections::BTreeMap,
    fmt,
    time::{Duration, SystemTime},
};

use reqwest::{header, redirect::Policy};
use serde::{Deserialize, Serialize, de::DeserializeOwned};

pub mod oauth;
pub mod realtime;
mod wire;
use serde_json::{Value, json};
use url::{Host, Url};

const MAX_RESPONSE_BYTES: usize = 4 * 1024 * 1024;

/// Errors intentionally omit URLs, credentials, response bodies, and user data.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Error {
    /// The origin, credential, or pagination arguments are invalid.
    InvalidInput(&'static str),
    /// The HTTP request failed or exceeded its timeout.
    Transport,
    /// The host could not safely read or persist credentials or settings.
    Persistence,
    /// The subscription violated the supported realtime contract.
    Realtime,
    /// The server returned a non-success HTTP status, including a redirect.
    Http(u16),
    /// The response exceeded the client's four MiB limit.
    ResponseTooLarge,
    /// The response did not have the required JSON structure.
    InvalidResponse,
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::InvalidInput(message) => f.write_str(message),
            Self::Persistence => f.write_str("Could not read or save the connection securely"),
            Self::Realtime => f.write_str("Realtime synchronization failed"),
            Self::Transport => f.write_str("Request failed or timed out"),
            Self::Http(code) => write!(f, "Server returned HTTP {code}"),
            Self::ResponseTooLarge => f.write_str("Server response exceeds four MiB"),
            Self::InvalidResponse => f.write_str("Invalid server response"),
        }
    }
}

impl std::error::Error for Error {}

/// Immutable, cheaply cloned request client for one origin and bearer credential.
///
/// Requests have a 30-second timeout, never follow redirects, and never retry.
/// Create a new client when the credential changes. Debug output omits both
/// origin and credential. The client does not log request or response data.
#[derive(Clone)]
pub struct Client {
    http: reqwest::Client,
    origin: Url,
    authorization: header::HeaderValue,
    minimum_cursor: Option<header::HeaderValue>,
}

impl fmt::Debug for Client {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("Client").finish_non_exhaustive()
    }
}

impl Client {
    /// Accept HTTPS origins, or HTTP origins on localhost and loopback IPs.
    /// Paths, user information, queries, and fragments are not accepted.
    pub fn new(origin: &str, token: &str) -> Result<Self, Error> {
        let origin =
            Url::parse(origin).map_err(|_| Error::InvalidInput("Enter a valid server origin"))?;
        let loopback = match origin.host() {
            Some(Host::Domain(name)) => name == "localhost",
            Some(Host::Ipv4(ip)) => ip.is_loopback(),
            Some(Host::Ipv6(ip)) => ip.is_loopback(),
            None => false,
        };
        if origin.host().is_none()
            || !origin.username().is_empty()
            || origin.password().is_some()
            || origin.path() != "/"
            || origin.query().is_some()
            || origin.fragment().is_some()
            || !(origin.scheme() == "https" || origin.scheme() == "http" && loopback)
        {
            return Err(Error::InvalidInput(
                "Use an HTTPS origin, or HTTP on loopback",
            ));
        }
        if token.trim().is_empty() {
            return Err(Error::InvalidInput("A bearer token is required"));
        }
        let mut authorization = header::HeaderValue::from_str(&format!("Bearer {token}"))
            .map_err(|_| Error::InvalidInput("Invalid bearer token"))?;
        authorization.set_sensitive(true);
        let http = reqwest::Client::builder()
            .redirect(Policy::none())
            .timeout(Duration::from_secs(30))
            .build()
            .map_err(|_| Error::Transport)?;
        Ok(Self {
            http,
            origin,
            authorization,
            minimum_cursor: None,
        })
    }

    async fn call<T: DeserializeOwned>(&self, method: &str, input: Value) -> Result<T, Error> {
        self.post(&format!("/api/connect/chatto.api.v1.{method}"), input, true)
            .await
    }

    async fn post<T: DeserializeOwned>(
        &self,
        path: &str,
        input: Value,
        authenticated: bool,
    ) -> Result<T, Error> {
        let endpoint = self
            .origin
            .join(path)
            .map_err(|_| Error::InvalidInput("Invalid API endpoint"))?;
        let mut request = self
            .http
            .post(endpoint)
            .header("Connect-Protocol-Version", "1")
            .json(&input);
        if let Some(cursor) = &self.minimum_cursor {
            request = request.header("Chatto-Realtime-Minimum-Cursor", cursor.clone());
        }
        if authenticated {
            request = request.header(header::AUTHORIZATION, self.authorization.clone());
        }
        let mut response = request.send().await.map_err(|_| Error::Transport)?;
        if !response.status().is_success() {
            return Err(Error::Http(response.status().as_u16()));
        }
        if response
            .content_length()
            .is_some_and(|len| len > MAX_RESPONSE_BYTES as u64)
        {
            return Err(Error::ResponseTooLarge);
        }
        let mut bytes = Vec::new();
        while let Some(chunk) = response.chunk().await.map_err(|_| Error::Transport)? {
            if chunk.len() > MAX_RESPONSE_BYTES - bytes.len() {
                return Err(Error::ResponseTooLarge);
            }
            bytes.extend_from_slice(&chunk);
        }
        serde_json::from_slice(&bytes).map_err(|_| Error::InvalidResponse)
    }

    /// Renew signed attachment URLs through the authorized room contract.
    /// The host must not reuse returned URLs across accounts or rooms.
    pub async fn attachment_urls(
        &self,
        room_id: &str,
        asset_ids: &[String],
    ) -> Result<Value, Error> {
        if room_id.is_empty()
            || asset_ids.is_empty()
            || asset_ids.len() > 32
            || asset_ids.iter().any(String::is_empty)
        {
            return Err(Error::InvalidInput(
                "A room and one to 32 asset IDs are required",
            ));
        }
        self.call(
            "AssetService/BatchGetAssets",
            json!({"roomId":room_id,"assetIds":asset_ids,"thumbnail":{"width":800,"height":600}}),
        )
        .await
    }

    /// Sign in through the server's direct-login endpoint. The server must allow
    /// password login. Passwords are neither retained nor logged. The request is
    /// anonymous, bounded, and never follows redirects or retries.
    pub async fn login_password(
        origin: &str,
        username: &str,
        password: &str,
    ) -> Result<HumanSession, Error> {
        let anonymous = Self::new(origin, "origin-validation")?;
        if username.trim().is_empty() || password.is_empty() {
            return Err(Error::InvalidInput("Enter a username and password"));
        }
        #[derive(Deserialize)]
        #[serde(rename_all = "camelCase")]
        struct Response {
            token: String,
            refresh_token: String,
            expires_in: i64,
            refresh_token_expires_in: i64,
        }
        let response: Response = anonymous
            .post(
                "/auth/login",
                json!({"login": username, "password": password}),
                false,
            )
            .await?;
        const MAX_LIFETIME: i64 = 315_360_000;
        if response.token.is_empty()
            || response.refresh_token.is_empty()
            || !(1..=MAX_LIFETIME).contains(&response.expires_in)
            || !(1..=MAX_LIFETIME).contains(&response.refresh_token_expires_in)
        {
            return Err(Error::InvalidResponse);
        }
        let now = SystemTime::now();
        let client = Self::new(origin, &response.token).map_err(|_| Error::InvalidResponse)?;
        Ok(HumanSession {
            client,
            access_token: response.token,
            refresh_request_id: String::new(),
            oauth_client_id: String::new(),
            refresh_token: response.refresh_token,
            access_expires_at: now
                .checked_add(Duration::from_secs(response.expires_in as u64))
                .ok_or(Error::InvalidResponse)?,
            session_expires_at: now
                .checked_add(Duration::from_secs(
                    response.refresh_token_expires_in as u64,
                ))
                .ok_or(Error::InvalidResponse)?,
        })
    }

    /// Bind resource reads to an opaque realtime boundary. Never parse this cursor.
    pub fn at_cursor(&self, cursor: &str) -> Result<Self, Error> {
        let mut client = self.clone();
        client.minimum_cursor = None;
        if !cursor.is_empty() {
            let mut value =
                header::HeaderValue::from_str(cursor).map_err(|_| Error::InvalidResponse)?;
            value.set_sensitive(true);
            client.minimum_cursor = Some(value);
        }
        Ok(client)
    }

    /// Read the server's public display identity.
    pub async fn server_profile(&self) -> Result<ServerProfile, Error> {
        #[derive(Deserialize)]
        struct Response {
            profile: ServerProfile,
        }
        let response: Response = self
            .call("ServerService/GetServerProfile", json!({}))
            .await?;
        Ok(response.profile)
    }

    /// Read all ordered room groups, including sidebar links.
    pub async fn room_groups(&self) -> Result<Vec<RoomGroup>, Error> {
        #[derive(Deserialize)]
        struct Response {
            #[serde(default)]
            groups: Vec<RoomGroup>,
        }
        let response: Response = self
            .call("RoomDirectoryService/ListRoomGroups", json!({}))
            .await?;
        Ok(response.groups)
    }

    /// Resolve up to 100 public user profiles. Callers choose labels and avatars.
    pub async fn profiles(&self, ids: &[String]) -> Result<BTreeMap<String, Profile>, Error> {
        if ids.len() > 100 {
            return Err(Error::InvalidInput("At most 100 users per batch"));
        }
        if ids.is_empty() {
            return Ok(BTreeMap::new());
        }
        #[derive(Deserialize)]
        struct User {
            user: Profile,
        }
        #[derive(Deserialize)]
        struct Response {
            #[serde(default)]
            users: Vec<User>,
        }
        let response: Response = self
            .call("UserService/BatchGetUsers", json!({"userIds":ids}))
            .await?;
        Ok(response
            .users
            .into_iter()
            .map(|entry| (entry.user.id.clone(), entry.user))
            .collect())
    }

    /// Read authoritative unread notification totals. Important is a subset of unread.
    pub async fn notification_counts(&self) -> Result<BTreeMap<String, NotificationCount>, Error> {
        #[derive(Deserialize)]
        #[serde(rename_all = "camelCase")]
        struct Response {
            #[serde(default)]
            room_unread_counts: Vec<NotificationCount>,
        }
        let response: Response = self
            .call(
                "NotificationService/ListNotificationOccurrences",
                json!({"page":{"limit":1}}),
            )
            .await?;
        Ok(response
            .room_unread_counts
            .into_iter()
            .map(|count| (count.room_id.clone(), count))
            .collect())
    }

    /// Mark the room through its current latest root event as read. This is an
    /// explicit command; the host decides when the user has read the room.
    pub async fn mark_room_read(&self, room: &str) -> Result<(), Error> {
        if room.is_empty() {
            return Err(Error::InvalidInput("Choose a room"));
        }
        let _: Value = self
            .call("RoomService/MarkRoomAsRead", json!({"roomId":room}))
            .await?;
        Ok(())
    }

    /// Read the authenticated account's public profile.
    pub async fn viewer(&self) -> Result<Profile, Error> {
        #[derive(Deserialize)]
        struct User {
            profile: Profile,
        }
        #[derive(Deserialize)]
        struct Response {
            user: User,
        }
        let response: Response = self.call("ViewerService/GetViewer", json!({})).await?;
        if response.user.profile.id.is_empty() {
            return Err(Error::InvalidResponse);
        }
        Ok(response.user.profile)
    }

    /// Read a live directory page. Offsets can shift as room visibility changes;
    /// deduplicate by ID when merging pages. A zero limit uses the server default.
    pub async fn rooms(&self, offset: u32, limit: u32) -> Result<RoomPage, Error> {
        if offset > i32::MAX as u32 || limit > 100 {
            return Err(Error::InvalidInput("Room limit must be at most 100"));
        }
        self.call(
            "RoomDirectoryService/ListRooms",
            json!({"page": {"offset": offset, "limit": limit}}),
        )
        .await
    }

    /// Read unfiltered room timeline events and their included resources.
    /// Unknown event kinds remain available in `events` as JSON.
    pub async fn room_events(
        &self,
        room_id: &str,
        options: TimelineOptions,
    ) -> Result<EventPage, Error> {
        if room_id.is_empty()
            || options.limit > 500
            || options.before.is_some() && options.after.is_some()
        {
            return Err(Error::InvalidInput(
                "Choose a room, one cursor direction, and a limit up to 500",
            ));
        }
        let mut input = json!({"roomId": room_id, "limit": options.limit});
        if let Some(before) = options.before {
            input["before"] = before.into();
        }
        if let Some(after) = options.after {
            input["after"] = after.into();
        }
        #[derive(Deserialize)]
        struct Response {
            page: EventPage,
        }
        let response: Response = self.call("RoomService/GetRoomEvents", input).await?;
        Ok(response.page)
    }

    /// Project non-deleted message posts and author profiles from a room page.
    /// Lifecycle events are omitted. Self-posts and thread echoes are retained.
    pub async fn timeline(
        &self,
        room_id: &str,
        options: TimelineOptions,
    ) -> Result<TimelinePage, Error> {
        self.room_events(room_id, options).await?.messages()
    }
}

/// Human credentials held only in memory. This type deliberately has no Debug
/// implementation. It does not retain the password. The host controls renewal
/// and persistence through explicit methods.
#[derive(Clone)]
pub struct HumanSession {
    client: Client,
    access_token: String,
    refresh_request_id: String,
    oauth_client_id: String,
    refresh_token: String,
    /// The access token's local expiry estimate from the server's lifetime.
    pub access_expires_at: SystemTime,
    /// The server session's local expiry estimate.
    pub session_expires_at: SystemTime,
}

impl HumanSession {
    /// Get an immutable client using this session's current access token.
    pub fn client(&self) -> Client {
        self.client.clone()
    }

    /// Export sensitive session credentials for the host's secure store. Never log them.
    pub fn credentials(&self) -> SessionCredentials {
        SessionCredentials {
            origin: self.client.origin.to_string(),
            access_token: self.access_token.clone(),
            refresh_token: self.refresh_token.clone(),
            refresh_request_id: self.refresh_request_id.clone(),
            oauth_client_id: self.oauth_client_id.clone(),
            access_expires_at: self.access_expires_at,
            session_expires_at: self.session_expires_at,
        }
    }

    /// Restore an immutable session. Expired access tokens can still be renewed
    /// while the refresh session is valid. No request or persistence is automatic.
    pub fn restore(saved: SessionCredentials) -> Result<Self, Error> {
        if saved.refresh_token.is_empty() {
            return Err(Error::InvalidInput("Missing refresh credential"));
        }
        let client = Client::new(&saved.origin, &saved.access_token)?;
        Ok(Self {
            client,
            access_token: saved.access_token,
            refresh_token: saved.refresh_token,
            refresh_request_id: saved.refresh_request_id,
            oauth_client_id: saved.oauth_client_id,
            access_expires_at: saved.access_expires_at,
            session_expires_at: saved.session_expires_at,
        })
    }

    /// Establish the identity of the next refresh. Persist credentials AFTER this
    /// call and BEFORE refresh(). Keep that prepared session on uncertain failure.
    pub fn prepare_refresh(&mut self) {
        if self.refresh_request_id.is_empty() {
            self.refresh_request_id = uuid::Uuid::new_v4().to_string();
        }
    }

    /// Rotate credentials once. Exact retries use the same prepared session.
    /// The host must persist the replacement BEFORE using its client.
    pub async fn refresh(&self) -> Result<Self, Error> {
        if self.refresh_request_id.is_empty() {
            return Err(Error::InvalidInput("Prepare and save the refresh first"));
        }
        #[derive(Deserialize)]
        struct Response {
            access_token: String,
            refresh_token: String,
            expires_in: i64,
            refresh_token_expires_in: i64,
        }
        let response: Response = self
            .client
            .post(
                "/oauth/token",
                json!({"grant_type":"refresh_token", "refresh_token":self.refresh_token,
            "refresh_request_id":self.refresh_request_id,"client_id":self.oauth_client_id}),
                false,
            )
            .await?;
        if response.access_token.is_empty()
            || response.refresh_token.is_empty()
            || !(1..=315_360_000).contains(&response.expires_in)
            || !(1..=315_360_000).contains(&response.refresh_token_expires_in)
        {
            return Err(Error::InvalidResponse);
        }
        let now = SystemTime::now();
        Self::restore(SessionCredentials {
            origin: self.client.origin.to_string(),
            oauth_client_id: self.oauth_client_id.clone(),
            access_token: response.access_token,
            refresh_token: response.refresh_token,
            refresh_request_id: String::new(),
            access_expires_at: now
                .checked_add(Duration::from_secs(response.expires_in as u64))
                .ok_or(Error::InvalidResponse)?,
            session_expires_at: now
                .checked_add(Duration::from_secs(
                    response.refresh_token_expires_in as u64,
                ))
                .ok_or(Error::InvalidResponse)?,
        })
    }

    /// Revoke this human session on its origin. On failure, the caller must
    /// decide whether to retry or discard local credentials. No retry is automatic.
    pub async fn logout(&self) -> Result<(), Error> {
        let _: Value = self
            .client
            .post(
                "/auth/logout",
                json!({"refreshToken": self.refresh_token}),
                true,
            )
            .await?;
        Ok(())
    }
}

/// Public profile fields used to identify an author. Do not log profile data.
#[derive(Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct Profile {
    pub id: String,
    pub login: String,
    pub display_name: String,
    /// Optional server-provided image address; hosts choose when and how to load it.
    pub avatar_url: Option<String>,
}

/// Directory metadata; it does not grant access to the room's history.
#[derive(Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct Room {
    pub id: String,
    pub name: String,
    /// Protocol enum name, including values unknown to this crate.
    pub kind: String,
    pub description: String,
    pub archived: bool,
}

/// Viewer-specific directory state. Hosts choose its presentation.
#[derive(Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct RoomViewerState {
    pub is_member: bool,
    pub has_unread: bool,
}

/// A room together with membership and direct-message participants.
#[derive(Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RoomEntry {
    pub room: Room,
    #[serde(default)]
    pub viewer_state: RoomViewerState,
    #[serde(default)]
    pub member_user_ids: Vec<String>,
}

/// Offset pagination metadata from the server.
#[derive(Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct PageInfo {
    pub has_more: bool,
}

/// One live directory page, in server order.
#[derive(Clone, Deserialize)]
pub struct RoomPage {
    #[serde(default)]
    pub rooms: Vec<RoomEntry>,
    #[serde(default)]
    pub page: PageInfo,
}

/// Timeline page selection. Cursors are opaque and bound to the viewer and room.
#[derive(Default)]
pub struct TimelineOptions {
    /// Maximum events, up to 500. Zero uses the server default.
    pub limit: u32,
    pub before: Option<String>,
    pub after: Option<String>,
}

/// Raw timeline page. Event and includes JSON preserves unmodeled protocol data.
#[derive(Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct EventPage {
    #[serde(default)]
    pub events: Vec<Value>,
    #[serde(default)]
    pub includes: Value,
    #[serde(default)]
    pub start_cursor: String,
    #[serde(default)]
    pub end_cursor: String,
    #[serde(default)]
    pub has_older: bool,
    #[serde(default)]
    pub has_newer: bool,
}

impl EventPage {
    fn messages(self) -> Result<TimelinePage, Error> {
        let users = match self.includes.get("users") {
            Some(users) => {
                serde_json::from_value(users.clone()).map_err(|_| Error::InvalidResponse)?
            }
            None => BTreeMap::new(),
        };
        let mut messages = Vec::new();
        for event in self.events {
            if let Some(post) = event.get("messagePosted") {
                let mut message: Message = serde_json::from_value(
                    post.get("message").cloned().ok_or(Error::InvalidResponse)?,
                )
                .map_err(|_| Error::InvalidResponse)?;
                if message.id.is_empty() {
                    return Err(Error::InvalidResponse);
                }
                if message.deleted_at.is_some() {
                    continue;
                }
                if message.created_at.is_empty() {
                    message.created_at = event
                        .get("createdAt")
                        .and_then(Value::as_str)
                        .unwrap_or_default()
                        .to_owned();
                }
                messages.push(message);
            }
        }
        Ok(TimelinePage {
            messages,
            users,
            start_cursor: self.start_cursor,
            end_cursor: self.end_cursor,
            has_older: self.has_older,
            has_newer: self.has_newer,
        })
    }
}

/// A message's basic display fields. Other fields retain their protocol JSON.
#[derive(Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Message {
    pub id: String,
    #[serde(default)]
    pub actor_id: String,
    /// None means unavailable; an empty string is a present, empty body.
    pub body: Option<String>,
    #[serde(default)]
    pub created_at: String,
    pub deleted_at: Option<String>,
    /// Attachments, reactions, thread metadata, and other unmodeled fields.
    #[serde(flatten)]
    pub metadata: BTreeMap<String, Value>,
}

/// Chronological messages and included author profiles from one timeline page.
#[derive(Clone, Default)]
pub struct TimelinePage {
    pub messages: Vec<Message>,
    pub users: BTreeMap<String, Profile>,
    pub start_cursor: String,
    pub end_cursor: String,
    pub has_older: bool,
    pub has_newer: bool,
}

/// Sensitive serializable credentials. Hosts must store these privately and
/// atomically; this type deliberately has no Debug implementation.
#[derive(Clone, Serialize, Deserialize)]
pub struct SessionCredentials {
    pub origin: String,
    pub access_token: String,
    pub refresh_token: String,
    /// Retain this exact UUID across uncertain refresh attempts.
    pub refresh_request_id: String,
    /// OAuth identity required for refresh; empty for legacy password sessions.
    #[serde(default)]
    pub oauth_client_id: String,
    pub access_expires_at: SystemTime,
    pub session_expires_at: SystemTime,
}

/// Public server identity, without host presentation policy.
#[derive(Clone, Default, Deserialize)]
#[serde(default)]
pub struct ServerProfile {
    pub name: String,
    #[serde(default, rename = "logoUrl")]
    pub logo_url: Option<String>,
    /// Optional sidebar artwork supplied by the server. Hosts control image reads.
    #[serde(default, rename = "bannerUrl")]
    pub banner_url: Option<String>,
    #[serde(default)]
    pub description: Option<String>,
}

/// Ordered channel navigation supplied by the server.
#[derive(Clone, Default, Deserialize)]
#[serde(default)]
pub struct RoomGroup {
    pub id: String,
    pub name: String,
    pub items: Vec<RoomGroupItem>,
}

/// A group item can reference a visible room or an optional navigation link.
#[derive(Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct RoomGroupItem {
    pub room: Option<RoomEntry>,
    pub sidebar_link: Option<SidebarLink>,
}

/// Untrusted navigation metadata. Hosts decide whether and how to open it.
#[derive(Clone, Default, Deserialize)]
#[serde(default)]
pub struct SidebarLink {
    pub id: String,
    pub label: String,
    pub url: String,
}

/// Authoritative unread occurrence counts, rather than a count of new messages.
#[derive(Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct NotificationCount {
    pub room_id: String,
    pub unread_count: u32,
    pub important_unread_count: u32,
}
