//! One optional, in-memory server projection backed by protocol-v4 WebSockets.
//! Resources come from cursor-fenced Connect reads, never event text previews.
//! The host supplies credentials and owns their secure persistence.

use crate::{
    Client, Error, NotificationCount, Profile, RoomEntry, RoomGroup, ServerProfile,
    TimelineOptions, TimelinePage, wire,
};
use futures_util::{SinkExt, StreamExt};
use prost::Message;
use std::{
    collections::{BTreeMap, BTreeSet},
    future::Future,
    pin::Pin,
    sync::Arc,
    time::Duration,
};
use tokio::{
    sync::{mpsc, watch},
    task::JoinHandle,
    time::{Instant, sleep, timeout},
};
use tokio_tungstenite::{
    connect_async_with_config,
    tungstenite::{self, protocol::WebSocketConfig},
};

/// Credential lookup may renew a session. A true argument requires renewal.
/// Hosts must serialize refreshes, persist prepared refresh identity before the
/// request, and persist replacement credentials before returning the client.
/// Every returned client must use the same origin and account for this server.
pub type ClientProvider =
    Arc<dyn Fn(bool) -> Pin<Box<dyn Future<Output = Result<Client, Error>> + Send>> + Send + Sync>;

/// Complete directory state from authorized resource reads.
#[derive(Clone, Default)]
pub struct Directory {
    pub server: ServerProfile,
    pub viewer: Profile,
    pub rooms: Vec<RoomEntry>,
    pub groups: Vec<RoomGroup>,
    pub users: BTreeMap<String, Profile>,
    pub notifications: BTreeMap<String, NotificationCount>,
}

/// Operational state, with no server-provided strings or private error bodies.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Status {
    #[default]
    Connecting,
    Synchronizing,
    Connected,
    Reconnecting,
    Failed,
}

/// Immutable connection snapshot. Reset generations invalidate all copied data.
/// Only one active room is retained. Reconnects replace the full directory and
/// active history; no cursor or content is persisted by this module.
#[derive(Clone, Default)]
pub struct ServerState {
    pub status: Status,
    pub generation: u64,
    pub directory: Directory,
    pub selected: Option<String>,
    pub timeline: TimelinePage,
    pub loading: bool,
    pub error: Option<Error>,
}

#[derive(Clone)]
enum Command {
    Select(String),
    Older,
    Refresh,
}

/// Owns the websocket, reader, resource requests, and update stream. Drop cancels
/// all tasks. Each updates() receiver observes the latest state independently.
pub struct LiveServer {
    commands: mpsc::Sender<Command>,
    updates: watch::Receiver<Arc<ServerState>>,
    task: JoinHandle<()>,
}
impl Drop for LiveServer {
    fn drop(&mut self) {
        self.task.abort();
    }
}
impl LiveServer {
    /// Start on a Tokio runtime. The caller selects a history retention cap,
    /// between 50 and 500 messages. Saved selection is checked by the history API before any history becomes visible.
    pub fn start(provider: ClientProvider, selected: Option<String>, history_limit: usize) -> Self {
        let (commands, receiver) = mpsc::channel(32);
        let (sender, updates) = watch::channel(Arc::new(ServerState::default()));
        let task = tokio::spawn(run(
            provider,
            selected,
            history_limit.clamp(50, 500),
            receiver,
            sender,
        ));
        Self {
            commands,
            updates,
            task,
        }
    }
    /// Subscribe to coalesced state changes, including connection resets.
    pub fn updates(&self) -> watch::Receiver<Arc<ServerState>> {
        self.updates.clone()
    }
    /// Select one room. A replaced selection cancels an in-flight history read.
    pub fn select(&self, room: String) -> Result<(), Error> {
        self.send(Command::Select(room))
    }
    /// Extend the active timeline, up to the host-selected retention cap.
    pub fn older(&self) -> Result<(), Error> {
        self.send(Command::Older)
    }
    /// Refresh the directory and active timeline without replacing credentials.
    pub fn refresh(&self) -> Result<(), Error> {
        self.send(Command::Refresh)
    }
    fn send(&self, command: Command) -> Result<(), Error> {
        self.commands.try_send(command).map_err(|_| Error::Realtime)
    }
}

/// Cancels child tasks even when the owning worker is aborted during an await.
struct ScopedTask<T>(JoinHandle<T>);
impl<T> Drop for ScopedTask<T> {
    fn drop(&mut self) {
        self.0.abort();
    }
}

#[derive(Default)]
struct Impact {
    directory: bool,
    room: Option<String>,
    all_timelines: bool,
    privacy: bool,
}
fn impact(event: &wire::Event) -> Option<Impact> {
    use wire::event::Kind::*;
    let mut change = Impact::default();
    match &event.kind {
        Some(
            Posted(room)
            | Edited(room)
            | Retracted(room)
            | Pinned(room)
            | Unpinned(room)
            | Thread(room)
            | ReactionAdded(room)
            | ReactionRemoved(room)
            | ThreadViewer(room),
        ) => {
            change.room = Some(room.room_id.clone());
            // Counts and unread flags can change with message activity too.
            change.directory = true;
            change.privacy = matches!(&event.kind, Some(Retracted(_)));
        }
        Some(
            RoomDeleted(_)
            | Left(_)
            | RoomUniversalChanged(_)
            | DeletedAccount(_)
            | RoleDeleted(_)
            | RoleAssigned(_)
            | RoleRevoked(_)
            | RolePermissions(_)
            | Permissions(_)
            | Deactivation(_),
        ) => {
            change.directory = true;
            change.all_timelines = true;
            change.privacy = true;
        }
        Some(Profile(_)) => {
            change.directory = true;
            change.all_timelines = true;
        }
        Some(_) => {
            change.directory = true;
        }
        None if event.cursor.is_some() => {
            change.directory = true;
            change.all_timelines = true;
            change.privacy = true;
        }
        None => return None, // Unmodeled cursorless typing/presence is transient.
    }
    Some(change)
}

enum Signal {
    Connecting,
    Ready(String, String),
    Changed(Impact, Option<String>),
    Heartbeat(Option<String>),
    Disconnected,
    Failed(Error),
}

async fn reader(
    provider: ClientProvider,
    ack: watch::Sender<Option<String>>,
    tx: mpsc::Sender<Signal>,
) {
    let mut delay = 1u64;
    let mut renew = false;
    loop {
        if tx.send(Signal::Connecting).await.is_err() {
            return;
        }
        let client = match provider(renew).await {
            Ok(client) => client,
            Err(Error::Transport | Error::Http(429 | 500..=599)) => {
                if tx.send(Signal::Disconnected).await.is_err() {
                    return;
                }
                sleep(Duration::from_secs(delay)).await;
                delay = (delay * 2).min(30);
                continue;
            }
            Err(error) => {
                let _ = tx.send(Signal::Failed(error)).await;
                return;
            }
        };
        renew = false;
        let resume = ack.borrow().clone();
        let mut synchronized = false;
        let outcome = read_socket(&client, resume.as_deref(), &tx, &mut synchronized).await;
        if synchronized {
            delay = 1;
        }
        let mut hint = 0;
        match outcome {
            Ok(close) => {
                hint = close
                    .retry_after
                    .map(|value| value.seconds.max(0) as u64)
                    .unwrap_or(0);
                match close.code {
                    1 | 2 => {
                        let _ = tx.send(Signal::Failed(Error::Realtime)).await;
                        return;
                    }
                    4 | 7 => {
                        let _ = tx.send(Signal::Failed(Error::Http(401))).await;
                        return;
                    }
                    5 => renew = true,
                    6 | 8 => {
                        ack.send_replace(None);
                    }
                    _ if !close.reconnect => {
                        let _ = tx.send(Signal::Failed(Error::Realtime)).await;
                        return;
                    }
                    _ => {}
                }
                delay = 1;
            }
            Err(Error::Http(code)) if (300..500).contains(&code) && code != 429 => {
                let _ = tx.send(Signal::Failed(Error::Http(code))).await;
                return;
            }
            Err(Error::Realtime | Error::InvalidResponse) => {
                let _ = tx.send(Signal::Failed(Error::Realtime)).await;
                return;
            }
            Err(_) => {}
        }
        if tx.send(Signal::Disconnected).await.is_err() {
            return;
        }
        let jitter = u64::from(uuid::Uuid::new_v4().as_bytes()[0]) * delay * 500 / 255;
        let wait = Duration::from_millis((delay * 1000 + jitter).max(hint.saturating_mul(1000)));
        if Instant::now().checked_add(wait).is_none() {
            let _ = tx.send(Signal::Failed(Error::Realtime)).await;
            return;
        }
        sleep(wait).await;
        delay = (delay * 2).min(30);
    }
}

async fn read_socket(
    client: &Client,
    resume: Option<&str>,
    tx: &mpsc::Sender<Signal>,
    synchronized: &mut bool,
) -> Result<wire::Close, Error> {
    let mut endpoint = client
        .origin
        .join("/api/realtime")
        .map_err(|_| Error::Realtime)?;
    endpoint
        .set_scheme(if client.origin.scheme() == "https" {
            "wss"
        } else {
            "ws"
        })
        .map_err(|_| Error::Realtime)?;
    let config = WebSocketConfig::default()
        .max_message_size(Some(4 << 20))
        .max_frame_size(Some(4 << 20));
    let (mut socket, _) = timeout(
        Duration::from_secs(10),
        connect_async_with_config(endpoint.as_str(), Some(config), false),
    )
    .await
    .map_err(|_| Error::Transport)?
    .map_err(|error| match error {
        tungstenite::Error::Http(response) => Error::Http(response.status().as_u16()),
        _ => Error::Transport,
    })?;
    let token = client
        .authorization
        .to_str()
        .map_err(|_| Error::InvalidResponse)?
        .strip_prefix("Bearer ")
        .ok_or(Error::InvalidResponse)?;
    let subscribe = wire::Subscribe {
        protocol_version: 4,
        bearer_token: Some(token.into()),
        resume_cursor: resume.map(str::to_owned),
        initial_state: 1,
    };
    timeout(
        Duration::from_secs(10),
        socket.send(tungstenite::Message::Binary(
            subscribe.encode_to_vec().into(),
        )),
    )
    .await
    .map_err(|_| Error::Transport)?
    .map_err(|_| Error::Transport)?;
    let mut caught = false;
    let mut queued = 0;
    loop {
        let message = timeout(Duration::from_secs(90), socket.next())
            .await
            .map_err(|_| Error::Transport)?
            .ok_or(Error::Transport)?
            .map_err(|_| Error::Transport)?;
        match message {
            tungstenite::Message::Binary(bytes) => {
                let frame = wire::Frame::decode(bytes).map_err(|_| Error::Realtime)?;
                match frame.kind.ok_or(Error::Realtime)? {
                    wire::frame::Kind::Close(close) => return Ok(close),
                    wire::frame::Kind::CaughtUp(boundary) => {
                        if caught
                            || boundary.cursor.is_empty()
                            || !matches!(boundary.recovery, 1 | 3)
                            || boundary.recovery == 1 && resume.is_none()
                        {
                            return Err(Error::Realtime);
                        }
                        caught = true;
                        *synchronized = true;
                        tx.send(Signal::Ready(boundary.cursor, client.origin.to_string()))
                            .await
                            .map_err(|_| Error::Realtime)?;
                    }
                    wire::frame::Kind::Heartbeat(heartbeat) => {
                        if caught {
                            tx.send(Signal::Heartbeat(heartbeat.cursor))
                                .await
                                .map_err(|_| Error::Realtime)?;
                        }
                    }
                    wire::frame::Kind::Event(event) => {
                        if !caught {
                            queued += 1;
                            if queued > 1000 {
                                return Ok(wire::Close {
                                    code: 6,
                                    reconnect: true,
                                    retry_after: None,
                                });
                            }
                        }
                        if let Some(change) = impact(&event) {
                            tx.send(Signal::Changed(change, event.cursor))
                                .await
                                .map_err(|_| Error::Realtime)?;
                        } else if caught && event.cursor.is_some() {
                            tx.send(Signal::Heartbeat(event.cursor))
                                .await
                                .map_err(|_| Error::Realtime)?;
                        }
                    }
                }
            }
            tungstenite::Message::Ping(data) => {
                socket
                    .send(tungstenite::Message::Pong(data))
                    .await
                    .map_err(|_| Error::Transport)?;
            }
            tungstenite::Message::Pong(_) => {}
            tungstenite::Message::Close(_) => return Err(Error::Transport),
            _ => return Err(Error::Realtime),
        }
    }
}

struct Loaded {
    directory: Option<Directory>,
    selected: Option<String>,
    timeline: Option<TimelinePage>,
}
async fn load(
    provider: ClientProvider,
    cursor: String,
    directory: Option<Directory>,
    selected: Option<String>,
    before: Option<String>,
    limit: usize,
    timeline_dirty: bool,
) -> Result<Loaded, Error> {
    let client = provider(false).await?.at_cursor(&cursor)?;
    let fetched = if directory.is_none() {
        Some(load_directory(&client).await?)
    } else {
        None
    };
    // Directory membership is navigation metadata, not authorization. The
    // history endpoint decides whether a saved or directly selected room is readable.
    let timeline = if timeline_dirty {
        match &selected {
            Some(room) => Some(
                client
                    .timeline(
                        room,
                        TimelineOptions {
                            before,
                            limit: limit as u32,
                            ..Default::default()
                        },
                    )
                    .await?,
            ),
            None => Some(TimelinePage::default()),
        }
    } else {
        None
    };
    Ok(Loaded {
        directory: fetched,
        selected,
        timeline,
    })
}

async fn load_directory(client: &Client) -> Result<Directory, Error> {
    let viewer = client.viewer().await?;
    let server = client.server_profile().await?;
    let groups = client.room_groups().await?;
    let mut rooms = BTreeMap::new();
    let mut offset = 0;
    loop {
        let page = client.rooms(offset, 100).await?;
        for entry in page.rooms {
            rooms.insert(entry.room.id.clone(), entry);
        }
        if !page.page.has_more {
            break;
        }
        offset = offset.checked_add(100).ok_or(Error::InvalidResponse)?;
        if offset > 100_000 {
            return Err(Error::ResponseTooLarge);
        }
    }
    let mut ids = BTreeSet::new();
    for entry in rooms.values() {
        ids.extend(entry.member_user_ids.iter().cloned());
    }
    ids.remove(&viewer.id);
    let ids: Vec<_> = ids.into_iter().collect();
    let mut users = BTreeMap::from([(viewer.id.clone(), viewer.clone())]);
    for chunk in ids.chunks(100) {
        users.extend(client.profiles(chunk).await?);
    }
    let notifications = match client.notification_counts().await {
        Ok(counts) => counts,
        Err(Error::Http(404 | 501)) => BTreeMap::new(),
        Err(error) => return Err(error),
    };
    Ok(Directory {
        viewer,
        server,
        rooms: rooms.into_values().collect(),
        groups,
        users,
        notifications,
    })
}

async fn run(
    provider: ClientProvider,
    mut desired: Option<String>,
    history_limit: usize,
    mut commands: mpsc::Receiver<Command>,
    updates: watch::Sender<Arc<ServerState>>,
) {
    let (tx, mut signals) = mpsc::channel(8);
    let (ack, _) = watch::channel(None);
    let _reader = ScopedTask(tokio::spawn(reader(provider.clone(), ack.clone(), tx)));
    let mut state = ServerState::default();
    let mut origin: Option<String> = None;
    let mut identity: Option<String> = None;
    let mut resource_provider = provider.clone();
    let mut read_cursor = String::new();
    let mut ready = false;
    let mut cursor = String::new();
    let mut dirty_directory = true;
    let mut dirty_timeline = true;
    let mut before = None;
    let mut pending: Option<ScopedTask<Result<Loaded, Error>>> = None;
    let mut deadline: Option<Instant> = None;
    loop {
        tokio::select! {
            signal = signals.recv() => {
                let Some(signal) = signal else {
                    state=ServerState { generation:state.generation+1,status:Status::Failed,error:Some(Error::Realtime),..Default::default() };
                    updates.send_replace(Arc::new(state)); return;
                };
                match signal {
                    Signal::Connecting => { state.status = if state.generation==0 { Status::Connecting } else { Status::Reconnecting }; }
                    Signal::Disconnected => {
                        ready=false; pending=None; deadline=None;
                        state=ServerState { generation:state.generation+1,status:Status::Reconnecting,..Default::default() };
                        dirty_directory=true; dirty_timeline=true; before=None;
                    }
                    Signal::Failed(error) => {
                        state=ServerState { generation:state.generation+1,status:Status::Failed,error:Some(error),..Default::default() };
                        updates.send_replace(Arc::new(state)); return;
                    }
                    Signal::Ready(next, endpoint) => {
                        if origin.as_ref().is_some_and(|old|*old!=endpoint) {
                            state=ServerState {generation:state.generation+1,status:Status::Failed,error:Some(Error::Realtime),..Default::default()};
                            updates.send_replace(Arc::new(state)); return;
                        }
                        origin=Some(endpoint.clone());
                        let inner=provider.clone();
                        resource_provider=Arc::new(move|renew| {let inner=inner.clone();let endpoint=endpoint.clone();Box::pin(async move {
                            let client=inner(renew).await?;
                            if client.origin.as_str()!=endpoint {return Err(Error::Realtime);}
                            Ok(client)
                        })});
                        cursor=next; ready=true; dirty_directory=true; dirty_timeline=true;
                        state.status=Status::Synchronizing; deadline=Some(Instant::now());
                    }
                    Signal::Changed(change,next) => {
                        if let Some(next)=next { cursor=next; }
                        let relevant = change.all_timelines || change.room.as_ref().is_some_and(|room| desired.as_ref()==Some(room));
                        dirty_directory |= change.directory;
                        dirty_timeline |= relevant;
                        if change.privacy {
                            state=ServerState { generation:state.generation+1,status:Status::Synchronizing,..Default::default() };
                            dirty_directory=true; dirty_timeline=true; before=None;
                        }
                        if change.privacy {pending=None;}
                        if (dirty_directory || dirty_timeline) && pending.is_none() && ready {
                            deadline.get_or_insert(Instant::now()+Duration::from_millis(60));
                        }
                    }
                    Signal::Heartbeat(next) => {
                        if let Some(next)=next { cursor=next; if pending.is_none() && !dirty_directory && !dirty_timeline { ack.send_replace(Some(cursor.clone())); } }
                        // Time-based notification expiry and session rotation do
                        // not always have an event. Re-read the directory on heartbeats.
                        dirty_directory=true;
                        if ready && pending.is_none() { deadline.get_or_insert(Instant::now()); }
                    }
                }
            }
            command = commands.recv() => {
                let Some(command)=command else { return; };
                match command {
                    Command::Select(room) => {
                        desired=Some(room.clone()); state.selected=Some(room); state.timeline=TimelinePage::default();
                        dirty_timeline=true; before=None;
                    }
                    Command::Older if !state.timeline.start_cursor.is_empty() && state.timeline.has_older && state.timeline.messages.len()<history_limit => {
                        before=Some(state.timeline.start_cursor.clone()); dirty_timeline=true;
                    }
                    Command::Older => continue,
                    Command::Refresh => { dirty_directory=true; dirty_timeline=true; before=None; }
                }
                if pending.is_some(){dirty_directory=true;dirty_timeline=true;}
                pending=None;
                if ready { deadline=Some(Instant::now()); }
            }
            _ = async { if let Some(at)=deadline { sleep(at.saturating_duration_since(Instant::now())).await; } else { std::future::pending::<()>().await; } } => {
                deadline=None;
                let directory=(!dirty_directory).then(||state.directory.clone());
                let limit = if before.is_some() { 50.min(history_limit-state.timeline.messages.len()) } else { state.timeline.messages.len().max(50).min(history_limit) };
                read_cursor=cursor.clone();
                pending=Some(ScopedTask(tokio::spawn(load(resource_provider.clone(),cursor.clone(),directory,desired.clone(),before.clone(),limit,dirty_timeline))));
                dirty_directory=false;dirty_timeline=false;
                state.loading=true;
            }
            result = async { match pending.as_mut() { Some(task)=>(&mut task.0).await, None=>std::future::pending().await } } => {
                pending=None;
                match result {
                    Ok(Ok(loaded)) => {
                        if let Some(directory)=loaded.directory {
                            if identity.as_ref().is_some_and(|old|*old!=directory.viewer.id) {
                                state=ServerState{generation:state.generation+1,status:Status::Failed,error:Some(Error::Realtime),..Default::default()};
                                updates.send_replace(Arc::new(state));return;
                            }
                            identity=Some(directory.viewer.id.clone());
                            state.directory=directory;
                        }
                        state.selected=loaded.selected;
                        if let Some(mut page)=loaded.timeline {
                            if before.is_some() {
                                page.messages.retain(|message| !state.timeline.messages.iter().any(|old| old.id==message.id));
                                page.messages.append(&mut state.timeline.messages);
                                page.users.append(&mut state.timeline.users);
                                page.end_cursor=state.timeline.end_cursor.clone();
                                page.has_newer=state.timeline.has_newer;
                            }
                            state.timeline=page;
                            if state.timeline.messages.len()>=history_limit { state.timeline.has_older=false; }
                        }
                        state.status=Status::Connected; state.loading=false; state.error=None;
                        before=None;
                        ack.send_replace(Some(read_cursor.clone()));
                        if dirty_directory||dirty_timeline {deadline=Some(Instant::now()+Duration::from_millis(60));}
                    }
                    Ok(Err(Error::Http(404|403))) if desired.is_some() => {
                        desired=None;
                        state=ServerState{generation:state.generation+1,status:Status::Synchronizing,..Default::default()};
                        state.error=Some(Error::Http(403)); state.loading=false;
                        dirty_directory=true; dirty_timeline=true; before=None;
                        deadline=Some(Instant::now()+Duration::from_secs(1));
                    }
                    Ok(Err(error @ (Error::Http(400|401|403)|Error::Persistence|Error::Realtime))) => {
                        state=ServerState { generation:state.generation+1,status:Status::Failed,error:Some(error),..Default::default() };
                        updates.send_replace(Arc::new(state)); return;
                    }
                    _ => {
                        state.timeline=TimelinePage::default(); state.loading=false; state.error=Some(Error::Transport);
                        dirty_directory=true; dirty_timeline=true; before=None;
                        deadline=Some(Instant::now()+Duration::from_secs(2));
                    }
                }
            }
        }
        updates.send_replace(Arc::new(state.clone()));
    }
}

#[cfg(test)]
mod tests;
