use super::*;
use axum::{
    Json, Router,
    extract::{
        Path, State, WebSocketUpgrade,
        ws::{Message as AxumMessage, WebSocket},
    },
    http::HeaderMap,
    response::IntoResponse,
    routing::{get, post},
};
use serde_json::{Value, json};
use std::sync::{
    Mutex,
    atomic::{AtomicBool, AtomicU32, Ordering},
};
use tokio::sync::broadcast;

#[derive(Clone, Debug)]
enum Notice {
    Event(wire::Event),
    Close(u32),
    Disconnect,
}
struct PeerState {
    notices: broadcast::Sender<Notice>,
    subscriptions: Mutex<Vec<wire::Subscribe>>,
    requests: Mutex<Vec<(String, Option<String>)>>,
    version: AtomicU32,
    deny_a: AtomicBool,
    slow_a: AtomicBool,
    paged_history: AtomicBool,
}
struct Peer {
    origin: String,
    state: Arc<PeerState>,
    task: JoinHandle<()>,
}
impl Drop for Peer {
    fn drop(&mut self) {
        self.task.abort();
    }
}
impl Peer {
    async fn new() -> Self {
        let (notices, _) = broadcast::channel(32);
        let state = Arc::new(PeerState {
            notices,
            subscriptions: Mutex::new(vec![]),
            requests: Mutex::new(vec![]),
            version: AtomicU32::new(0),
            deny_a: AtomicBool::new(false),
            slow_a: AtomicBool::new(false),
            paged_history: AtomicBool::new(false),
        });
        let router = Router::new()
            .route("/api/realtime", get(websocket))
            .route("/api/connect/{*method}", post(api))
            .with_state(state.clone());
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let origin = format!("http://{}", listener.local_addr().unwrap());
        let task = tokio::spawn(async move {
            axum::serve(listener, router).await.unwrap();
        });
        Self {
            origin,
            state,
            task,
        }
    }
    fn provider(&self) -> ClientProvider {
        let origin = self.origin.clone();
        Arc::new(move |_| {
            let origin = origin.clone();
            Box::pin(async move { Client::new(&origin, "synthetic-token") })
        })
    }
    fn event(&self, cursor: &str, kind: wire::event::Kind) {
        self.state
            .notices
            .send(Notice::Event(wire::Event {
                id: cursor.into(),
                cursor: Some(cursor.into()),
                kind: Some(kind),
            }))
            .unwrap();
    }
}
async fn websocket(
    upgrade: WebSocketUpgrade,
    State(state): State<Arc<PeerState>>,
) -> impl IntoResponse {
    upgrade.on_upgrade(move |socket| handle_socket(socket, state))
}
async fn handle_socket(mut socket: WebSocket, state: Arc<PeerState>) {
    let mut notices = state.notices.subscribe();
    let Some(Ok(AxumMessage::Binary(bytes))) = socket.recv().await else {
        return;
    };
    let subscribe = wire::Subscribe::decode(bytes).unwrap();
    let recovery = if subscribe.resume_cursor.is_some() {
        1
    } else {
        3
    };
    let count = {
        let mut subscriptions = state.subscriptions.lock().unwrap();
        subscriptions.push(subscribe);
        subscriptions.len()
    };
    let frame = wire::Frame {
        kind: Some(wire::frame::Kind::CaughtUp(wire::CaughtUp {
            cursor: format!("ready-{count}"),
            recovery,
        })),
    };
    if socket
        .send(AxumMessage::Binary(frame.encode_to_vec().into()))
        .await
        .is_err()
    {
        return;
    }
    loop {
        tokio::select! {
            incoming=socket.recv()=>{match incoming{Some(Ok(AxumMessage::Ping(bytes)))=>{if socket.send(AxumMessage::Pong(bytes)).await.is_err(){return;}},_=>return}},
            notice=notices.recv()=>{
                let frame=match notice {
                    Ok(Notice::Event(event))=>wire::Frame{kind:Some(wire::frame::Kind::Event(event))},
                    Ok(Notice::Close(code))=>wire::Frame{kind:Some(wire::frame::Kind::Close(wire::Close{code,reconnect:false,retry_after:None}))},
                    Ok(Notice::Disconnect)=>{let _=socket.send(AxumMessage::Close(None)).await;return;},
                    Err(_)=>return,
                };
                if socket.send(AxumMessage::Binary(frame.encode_to_vec().into())).await.is_err(){return;}
            }
        }
    }
}
async fn api(
    Path(method): Path<String>,
    State(state): State<Arc<PeerState>>,
    headers: HeaderMap,
    Json(input): Json<Value>,
) -> axum::response::Response {
    assert_eq!(
        headers.get("authorization").unwrap(),
        "Bearer synthetic-token"
    );
    let cursor = headers
        .get("Chatto-Realtime-Minimum-Cursor")
        .map(|value| value.to_str().unwrap().to_owned());
    assert!(
        cursor.is_some(),
        "resource read must be fenced at its realtime boundary"
    );
    state
        .requests
        .lock()
        .unwrap()
        .push((method.clone(), cursor));
    let version = state.version.load(Ordering::SeqCst);
    let room = |id: &str| json!({"room":{"id":id,"name":format!("{id}-{version}"),"kind":"ROOM_KIND_CHANNEL"},"viewerState":{"isMember":true,"hasUnread":version>0}});
    let value = match method.as_str() {
        "chatto.api.v1.ViewerService/GetViewer" => {
            json!({"user":{"profile":{"id":"viewer","displayName":"Synthetic viewer"}}})
        }
        "chatto.api.v1.ServerService/GetServerProfile" => {
            json!({"profile":{"name":format!("Test server {version}")}})
        }
        "chatto.api.v1.RoomDirectoryService/ListRooms" => {
            let mut rooms = vec![room("b")];
            if !state.deny_a.load(Ordering::SeqCst) {
                rooms.insert(0, room("a"));
            }
            json!({"rooms":rooms})
        }
        "chatto.api.v1.RoomDirectoryService/ListRoomGroups" => {
            json!({"groups":[{"id":"team","name":"Team","items":[{"room":room("a")},{"room":room("b")}]}]})
        }
        "chatto.api.v1.NotificationService/ListNotificationOccurrences" => {
            json!({"roomUnreadCounts":[{"roomId":"a","unreadCount":version,"importantUnreadCount":version}]})
        }
        "chatto.api.v1.RoomService/GetRoomEvents" => {
            let id = input["roomId"].as_str().unwrap();
            let denied = state.deny_a.load(Ordering::SeqCst) && id == "a";
            if id == "a" && state.slow_a.load(Ordering::SeqCst) {
                sleep(Duration::from_millis(350)).await;
            }
            if denied {
                return axum::http::StatusCode::FORBIDDEN.into_response();
            }
            if state.paged_history.load(Ordering::SeqCst) {
                let end = input["before"]
                    .as_str()
                    .and_then(|cursor| cursor.parse::<usize>().ok())
                    .unwrap_or(200);
                let limit = input["limit"].as_u64().unwrap_or(50) as usize;
                let start = end.saturating_sub(limit);
                let events: Vec<_> = (start..end).map(|index| json!({"messagePosted":{"message":{"id":format!("history-{index}"),"actorId":"viewer","body":"Synthetic page"}}})).collect();
                json!({"page":{"events":events,"startCursor":start.to_string(),"endCursor":end.to_string(),"hasOlder":start>0}})
            } else {
                json!({"page":{"events":[{"messagePosted":{"message":{"id":format!("message-{id}"),"actorId":"viewer","body":format!("body-{version}")}}}],"includes":{"users":{"viewer":{"id":"viewer","displayName":"Synthetic viewer"}}}}})
            }
        }
        _ => panic!("unexpected test API route"),
    };
    Json(value).into_response()
}
async fn until(
    rx: &mut watch::Receiver<Arc<ServerState>>,
    predicate: impl Fn(&ServerState) -> bool,
) -> Arc<ServerState> {
    timeout(Duration::from_secs(8), async {
        loop {
            let state = rx.borrow_and_update().clone();
            if predicate(&state) {
                return state;
            }
            rx.changed().await.expect("connection still active");
        }
    })
    .await
    .expect("state update timed out")
}
fn room(id: &str) -> wire::Room {
    wire::Room { room_id: id.into() }
}

#[tokio::test]
async fn live_messages_groups_unread_counts_and_fenced_reads() {
    let peer = Peer::new().await;
    let connection = LiveServer::start(peer.provider(), Some("a".into()), 500);
    let mut updates = connection.updates();
    let initial = until(&mut updates, |state| state.status == Status::Connected).await;
    assert_eq!(initial.directory.groups[0].name, "Team");
    assert_eq!(initial.timeline.messages[0].body.as_deref(), Some("body-0"));
    {
        let subscriptions = peer.state.subscriptions.lock().unwrap();
        assert_eq!(subscriptions[0].protocol_version, 4);
        assert_eq!(subscriptions[0].initial_state, 1);
        assert_eq!(
            subscriptions[0].bearer_token.as_deref(),
            Some("synthetic-token")
        );
    }
    peer.state.version.store(1, Ordering::SeqCst);
    peer.event("event-one", wire::event::Kind::Posted(room("a")));
    let next = until(&mut updates, |state| {
        state.status == Status::Connected
            && state
                .timeline
                .messages
                .first()
                .and_then(|message| message.body.as_deref())
                == Some("body-1")
    })
    .await;
    assert!(next.directory.rooms[0].viewer_state.has_unread);
    assert_eq!(next.directory.notifications["a"].important_unread_count, 1);
    assert!(
        peer.state
            .requests
            .lock()
            .unwrap()
            .iter()
            .any(|(path, cursor)| path.ends_with("GetRoomEvents")
                && cursor.as_deref() == Some("event-one"))
    );
}
#[tokio::test]
async fn room_switch_and_permission_reset_cannot_restore_stale_reads() {
    let peer = Peer::new().await;
    let connection = LiveServer::start(peer.provider(), None, 500);
    let mut updates = connection.updates();
    until(&mut updates, |state| state.status == Status::Connected).await;
    peer.state.slow_a.store(true, Ordering::SeqCst);
    connection.select("a".into()).unwrap();
    sleep(Duration::from_millis(40)).await;
    connection.select("b".into()).unwrap();
    until(&mut updates, |state| {
        state.selected.as_deref() == Some("b") && !state.loading
    })
    .await;
    sleep(Duration::from_millis(400)).await;
    assert_eq!(updates.borrow().selected.as_deref(), Some("b"));
    connection.select("a".into()).unwrap();
    sleep(Duration::from_millis(40)).await;
    peer.state.deny_a.store(true, Ordering::SeqCst);
    peer.event("authority", wire::event::Kind::Permissions(wire::Empty {}));
    let reset = until(&mut updates, |state| state.generation > 0).await;
    assert!(reset.timeline.messages.is_empty());
    let next = until(&mut updates, |state| {
        state.status == Status::Connected && state.selected.is_none()
    })
    .await;
    assert!(
        next.directory
            .rooms
            .iter()
            .all(|entry| entry.room.id != "a")
    );
    sleep(Duration::from_millis(400)).await;
    assert!(updates.borrow().timeline.messages.is_empty());
}
#[tokio::test]
async fn reconnect_resumes_only_committed_cursors_and_resync_clears_them() {
    let peer = Peer::new().await;
    let connection = LiveServer::start(peer.provider(), Some("a".into()), 500);
    let mut updates = connection.updates();
    until(&mut updates, |state| state.status == Status::Connected).await;
    peer.state.notices.send(Notice::Disconnect).unwrap();
    until(&mut updates, |state| {
        state.status == Status::Reconnecting && state.directory.rooms.is_empty()
    })
    .await;
    until(&mut updates, |state| state.status == Status::Connected).await;
    assert_eq!(
        peer.state.subscriptions.lock().unwrap()[1]
            .resume_cursor
            .as_deref(),
        Some("ready-1")
    );
    peer.state.version.store(2, Ordering::SeqCst);
    peer.state.notices.send(Notice::Close(6)).unwrap();
    until(&mut updates, |state| {
        state.status == Status::Connected && state.directory.server.name == "Test server 2"
    })
    .await;
    assert!(
        peer.state.subscriptions.lock().unwrap()[2]
            .resume_cursor
            .is_none()
    );
}
#[tokio::test]
async fn session_termination_clears_all_private_state() {
    let peer = Peer::new().await;
    let connection = LiveServer::start(peer.provider(), Some("a".into()), 500);
    let mut updates = connection.updates();
    until(&mut updates, |state| state.status == Status::Connected).await;
    peer.state.notices.send(Notice::Close(7)).unwrap();
    let state = until(&mut updates, |state| state.status == Status::Failed).await;
    assert_eq!(state.error, Some(Error::Http(401)));
    assert!(state.timeline.messages.is_empty());
    assert!(state.directory.rooms.is_empty());
}

#[tokio::test]
async fn continuous_message_events_do_not_starve_slow_resource_reads() {
    let peer = Peer::new().await;
    let connection = LiveServer::start(peer.provider(), Some("a".into()), 500);
    let mut updates = connection.updates();
    until(&mut updates, |state| state.status == Status::Connected).await;
    peer.state.slow_a.store(true, Ordering::SeqCst);
    let notices = peer.state.notices.clone();
    let version = peer.state.clone();
    let producer = tokio::spawn(async move {
        for index in 1..=30 {
            version.version.store(index, Ordering::SeqCst);
            let cursor = format!("busy-{index}");
            let _ = notices.send(Notice::Event(wire::Event {
                id: cursor.clone(),
                cursor: Some(cursor),
                kind: Some(wire::event::Kind::Posted(room("a"))),
            }));
            sleep(Duration::from_millis(40)).await;
        }
    });
    let state = timeout(
        Duration::from_secs(1),
        until(&mut updates, |state| {
            !state.loading
                && state
                    .timeline
                    .messages
                    .first()
                    .and_then(|message| message.body.as_deref())
                    .is_some_and(|body| body != "body-0")
        }),
    )
    .await
    .expect("updates must complete while events are still arriving");
    assert_eq!(state.selected.as_deref(), Some("a"));
    producer.await.unwrap();
}

#[tokio::test]
async fn renewal_close_requests_fresh_credentials_before_reconnecting() {
    let peer = Peer::new().await;
    let renewed = Arc::new(AtomicBool::new(false));
    let observed = renewed.clone();
    let provider = peer.provider();
    let provider: ClientProvider = Arc::new(move |force| {
        if force {
            observed.store(true, Ordering::SeqCst);
        }
        provider(force)
    });
    let connection = LiveServer::start(provider, None, 500);
    let mut updates = connection.updates();
    until(&mut updates, |state| state.status == Status::Connected).await;
    peer.state.notices.send(Notice::Close(5)).unwrap();
    until(&mut updates, |state| state.status == Status::Reconnecting).await;
    until(&mut updates, |state| state.status == Status::Connected).await;
    assert!(renewed.load(Ordering::SeqCst));
    assert_eq!(peer.state.subscriptions.lock().unwrap().len(), 2);
}

#[tokio::test]
async fn older_pages_merge_in_order_and_stop_at_the_retention_limit() {
    let peer = Peer::new().await;
    peer.state.paged_history.store(true, Ordering::SeqCst);
    let connection = LiveServer::start(peer.provider(), Some("a".into()), 120);
    let mut updates = connection.updates();
    let initial = until(&mut updates, |state| {
        state.status == Status::Connected && !state.loading
    })
    .await;
    assert_eq!(initial.timeline.messages.len(), 50);
    assert_eq!(initial.timeline.start_cursor, "150");
    connection.older().unwrap();
    let older = until(&mut updates, |state| {
        !state.loading && state.timeline.messages.len() == 100
    })
    .await;
    assert_eq!(older.timeline.messages.first().unwrap().id, "history-100");
    assert_eq!(older.timeline.messages.last().unwrap().id, "history-199");
    assert_eq!(older.timeline.end_cursor, "200");
    connection.older().unwrap();
    let capped = until(&mut updates, |state| {
        !state.loading && state.timeline.messages.len() == 120
    })
    .await;
    assert_eq!(capped.timeline.messages.first().unwrap().id, "history-80");
    assert!(!capped.timeline.has_older);
    connection.older().unwrap();
    sleep(Duration::from_millis(100)).await;
    assert_eq!(updates.borrow().timeline.messages.len(), 120);
}
