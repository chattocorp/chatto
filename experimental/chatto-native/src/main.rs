mod browser_auth;
mod chat_shell;
mod media;
mod message_text;
mod preview;
mod registry;
mod server_ui;
mod sidebar;
mod storage;
mod timeline;
mod ui;

use chatto::{
    Error,
    realtime::{LiveServer, ServerState, Status},
};
use gpui::component::{
    Disableable, Root, Selectable, Sizable,
    button::{Button, ButtonVariants},
    input::{Input, InputEvent, InputState},
    menu::{ContextMenuExt, PopupMenuItem},
};
use gpui::{
    App, Bounds, ClipboardItem, Context, Entity, Focusable, KeyBinding, ListAlignment, ListOffset,
    ListState, ScrollHandle, SharedString, Subscription, Task, Window, WindowBounds, WindowOptions,
    actions, div, list, prelude::*, px, rgb, size,
};
use std::{collections::BTreeMap, future::Future, sync::Arc};
use storage::Storage;
use tokio::{runtime::Runtime, task::AbortHandle};

actions!(
    chatto_native,
    [Quit, FindConversation, ZoomIn, ZoomOut, ResetZoom]
);

/// Cancellation covers both the background operation and its UI delivery.
struct Request {
    abort: AbortHandle,
    _delivery: Task<()>,
}
impl Drop for Request {
    fn drop(&mut self) {
        self.abort.abort();
    }
}
struct LoginForm {
    origin: Entity<InputState>,
    _submit: Subscription,
}

/// Each saved server keeps its own authorized projection and subscription.
struct Connection {
    server: Arc<LiveServer>,
    state: Arc<ServerState>,
    _stream: Task<()>,
    public_media: Entity<media::MediaCache>,
    _media_subscription: Subscription,
}

struct ChattoNative {
    runtime: Arc<Runtime>,
    persistence_gate: Arc<tokio::sync::Mutex<()>>,
    preview: bool,
    scale: f32,
    focus: Option<gpui::FocusHandle>,
    origin: String,
    store: Option<Arc<Storage>>,
    registry: Option<Arc<registry::Registry>>,
    active_server: Option<String>,
    connections: BTreeMap<String, Connection>,
    adding: bool,
    managing: bool,
    sidebar_visible: bool,
    search_visible: bool,
    overview: bool,
    people_visible: bool,
    people_search: Option<Entity<InputState>>,
    people_subscription: Option<Subscription>,
    login_form: Option<LoginForm>,
    auth_status: Option<&'static str>,
    request: Option<Request>,
    server: Option<Arc<LiveServer>>,
    state: Arc<ServerState>,
    error: Option<String>,
    room_scroll: ScrollHandle,
    room_search: Option<Entity<InputState>>,
    search_subscription: Option<Subscription>,
    message_list: ListState,
    message_views: BTreeMap<String, Entity<message_text::MessageText>>,
    older_pending: bool,
    media: Option<Entity<media::MediaCache>>,
    media_subscription: Option<Subscription>,
}
impl ChattoNative {
    fn new(runtime: Arc<Runtime>) -> Self {
        let port = std::env::var("CHATTO_DEV_CHATTO_PORT")
            .ok()
            .and_then(|value| value.parse::<u16>().ok())
            .unwrap_or(4000);
        Self {
            runtime,
            persistence_gate: Arc::new(tokio::sync::Mutex::new(())),
            preview: false,
            scale: 1.,
            focus: None,
            origin: format!("http://127.0.0.1:{port}"),
            store: None,
            registry: None,
            active_server: None,
            connections: BTreeMap::new(),
            adding: true,
            managing: false,
            sidebar_visible: true,
            search_visible: false,
            overview: false,
            people_visible: false,
            people_search: None,
            people_subscription: None,
            login_form: None,
            auth_status: None,
            request: None,
            server: None,
            state: Arc::new(ServerState::default()),
            error: None,
            room_scroll: ScrollHandle::new(),
            room_search: None,
            search_subscription: None,
            message_list: ListState::new(0, ListAlignment::Bottom, px(240.)),
            message_views: BTreeMap::new(),
            older_pending: false,
            media: None,
            media_subscription: None,
        }
    }
    fn mutation_busy(&self) -> bool {
        matches!(
            self.auth_status,
            Some("Saving your session…" | "Removing server…")
        )
    }
    /// Reflow cached list rows while keeping the current message and relative offset.
    fn change_scale(&mut self, value: f32, window: &mut Window, cx: &mut Context<Self>) {
        let next = ui::normalize_scale(value);
        if next == self.scale {
            return;
        }
        self.scale = next;
        window.set_rem_size(px(16. * next));
        ui::scale_theme(next, cx);
        // Kit preserves the message anchor and offset ratio during reflow.
        self.message_list.remeasure();
        if let Some(registry) = &self.registry
            && let Err(error) = registry.update(|catalogue| catalogue.ui_scale = next)
        {
            self.error = Some(error.to_string());
        }
        window.refresh();
        cx.notify();
    }

    fn request<T: Send + 'static>(
        &self,
        future: impl Future<Output = Result<T, Error>> + Send + 'static,
        cx: &mut Context<Self>,
        finish: impl FnOnce(&mut Self, Result<T, Error>, &mut Context<Self>) + 'static,
    ) -> Request {
        let worker = self.runtime.spawn(future);
        let abort = worker.abort_handle();
        let delivery = cx.spawn(async move |this, cx| {
            let result = worker.await.unwrap_or(Err(Error::Transport));
            let _ = this.update(cx, |this, cx| {
                finish(this, result, cx);
                cx.notify();
            });
        });
        Request {
            abort,
            _delivery: delivery,
        }
    }
    fn restore(&mut self, cx: &mut Context<Self>) {
        self.auth_status = Some("Restoring your servers…");
        self.error = None;
        self.request = Some(self.request(
            async {
                tokio::task::spawn_blocking(registry::Registry::open_default)
                    .await
                    .map_err(|_| Error::Persistence)?
            },
            cx,
            |this, result, cx| {
                this.auth_status = None;
                match result {
                    Ok(registry) => {
                        let catalogue = registry.catalogue();
                        this.scale = ui::normalize_scale(catalogue.ui_scale);
                        this.registry = Some(registry);
                        for registration in catalogue.servers {
                            this.connect_server(registration.id, cx);
                        }
                        if let Some(id) = catalogue.active {
                            this.switch_server(id, cx);
                        }
                    }
                    Err(error) => this.error = Some(error.to_string()),
                }
            },
        ));
    }
    fn clear_connection(&mut self) {
        self.server = None;
        self.request = None;
        self.login_form = None;
        self.overview = false;
        self.search_visible = false;
        self.people_search = None;
        self.people_subscription = None;
        self.state = Arc::new(ServerState::default());
        self.auth_status = None;
        self.error = None;
        self.room_scroll = ScrollHandle::new();
        self.room_search = None;
        self.search_subscription = None;
        self.message_list.reset(0);
        self.message_views.clear();
        self.media = None;
        self.media_subscription = None;
        self.older_pending = false;
    }
    fn connect(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let (Some(form), Some(registry)) = (self.login_form.as_ref(), self.registry.clone()) else {
            return;
        };
        let origin = form.origin.read(cx).value().trim().to_owned();
        if origin.is_empty() {
            self.error = Some("Enter the server URL".into());
            cx.notify();
            return;
        }
        window.blur(cx);
        self.origin = origin.clone();
        self.auth_status = Some("Continue sign-in in your browser…");
        self.error = None;
        self.request = Some(self.request(
            async move { browser_auth::sign_in(&origin).await },
            cx,
            move |this, result, cx| {
                match result {
                    Ok((session, name)) => {
                        let origin = url::Url::parse(&session.credentials().origin)
                            .ok()
                            .map(|url| url.origin().ascii_serialization());
                        if let Some(id) = registry
                            .catalogue()
                            .servers
                            .iter()
                            .find(|server| Some(&server.origin) == origin.as_ref())
                            .map(|server| server.id.clone())
                        {
                            this.connections.remove(&id);
                            if this.active_server.as_ref() == Some(&id) {
                                this.clear_connection();
                            }
                        }
                        this.auth_status = Some("Saving your session…");
                        // Once consent has issued credentials, finish persistence even if
                        // the window closes. All server switching is disabled in this phase.
                        let lease = this
                            .persistence_gate
                            .clone()
                            .try_lock_owned()
                            .expect("one credential mutation at a time");
                        let worker = this.runtime.spawn(async move {
                            let _lease = lease;
                            registry.install(session, name).await
                        });
                        this.request = Some(this.request(
                            async move { worker.await.map_err(|_| Error::Persistence)? },
                            cx,
                            |this, result, cx| {
                                this.auth_status = None;
                                match result {
                                    Ok(id) => {
                                        this.connections.remove(&id);
                                        this.connect_server(id.clone(), cx);
                                        this.switch_server(id, cx);
                                        cx.activate(true);
                                    }
                                    Err(error) => this.error = Some(error.to_string()),
                                }
                            },
                        ));
                    }
                    Err(error) => {
                        this.auth_status = None;
                        this.error = Some(error.to_string());
                    }
                }
            },
        ));
    }
    fn connect_server(&mut self, id: String, cx: &mut Context<Self>) {
        let Some(store) = self
            .registry
            .as_ref()
            .and_then(|registry| registry.store(&id))
        else {
            return;
        };
        let server = {
            let _enter = self.runtime.enter();
            Arc::new(LiveServer::start(
                store.provider(),
                store.settings().selected,
                500,
            ))
        };
        let mut updates = server.updates();
        let key = id.clone();
        let stream = cx.spawn(async move |this, cx| {
            while updates.changed().await.is_ok() {
                let state = updates.borrow_and_update().clone();
                if this
                    .update(cx, |this, cx| this.accept_server(&key, state, cx))
                    .is_err()
                {
                    return;
                }
            }
        });
        let runtime = self.runtime.clone();
        let origin = store.settings().origin;
        let public_media = cx.new(|_| media::MediaCache::new(runtime, origin, None, String::new()));
        let media_subscription = cx.observe(&public_media, |_, _, cx| cx.notify());
        self.connections.insert(
            id,
            Connection {
                public_media,
                _media_subscription: media_subscription,
                server,
                state: Arc::new(ServerState::default()),
                _stream: stream,
            },
        );
    }
    fn start_connection(&mut self, cx: &mut Context<Self>) {
        if let Some(id) = self.active_server.clone() {
            self.connections.remove(&id);
            self.connect_server(id.clone(), cx);
            self.switch_server(id, cx);
        }
    }
    fn switch_server(&mut self, id: String, cx: &mut Context<Self>) {
        if self.mutation_busy() {
            return;
        }
        let Some(registry) = self.registry.clone() else {
            return;
        };
        let Some(registration) = registry
            .catalogue()
            .servers
            .into_iter()
            .find(|server| server.id == id)
        else {
            return;
        };
        self.clear_connection();
        self.active_server = Some(id.clone());
        self.origin = registration.origin;
        self.store = registry.store(&id);
        self.adding = false;
        self.managing = false;
        if !self.connections.contains_key(&id) {
            self.connect_server(id.clone(), cx);
        }
        if let Some(connection) = self.connections.get(&id) {
            self.server = Some(connection.server.clone());
            self.state = connection.state.clone();
            self.message_list.reset(self.state.timeline.messages.len());
        }
        if let Err(error) = registry.update(|catalogue| catalogue.active = Some(id)) {
            self.error = Some(error.to_string());
        }
        cx.notify();
    }
    fn accept_server(&mut self, id: &str, state: Arc<ServerState>, cx: &mut Context<Self>) {
        let Some(connection) = self.connections.get_mut(id) else {
            return;
        };
        connection.state = state.clone();
        if let Some(store) = self
            .registry
            .as_ref()
            .and_then(|registry| registry.store(id))
            && state.status == Status::Connected
            && store.settings().selected != state.selected
            && let Err(error) =
                store.update_settings(|settings| settings.selected = state.selected.clone())
        {
            self.error = Some(error.to_string());
        }
        if self.active_server.as_deref() == Some(id) && !self.adding && !self.managing {
            self.accept(state, cx);
        }
        cx.notify();
    }
    fn begin_add(&mut self, cx: &mut Context<Self>) {
        if self.mutation_busy() {
            return;
        }
        self.clear_connection();
        self.adding = true;
        self.managing = false;
        self.origin = "https://".into();
        cx.notify();
    }
    fn cancel_sign_in(&mut self, cx: &mut Context<Self>) {
        if self.mutation_busy() {
            return;
        }
        self.request = None;
        self.auth_status = None;
        cx.notify();
    }
    fn accept(&mut self, state: Arc<ServerState>, _cx: &mut Context<Self>) {
        let reset =
            state.generation != self.state.generation || state.selected != self.state.selected;
        if reset || self.state.timeline.messages.is_empty() {
            self.media = None;
            self.media_subscription = None;
            self.message_views.clear();
            self.message_list.reset(state.timeline.messages.len());
        } else {
            let old = &self.state.timeline.messages;
            let new = &state.timeline.messages;
            let offset = self.message_list.logical_scroll_top();
            let anchor = old.get(offset.item_ix).map(|message| message.id.clone());
            let old_ids: Vec<_> = old.iter().map(|message| &message.id).collect();
            let new_ids: Vec<_> = new.iter().map(|message| &message.id).collect();
            let (range, count) = timeline::change(&old_ids, &new_ids);
            if !range.is_empty() || count > 0 {
                self.message_list.splice(range, count);
            }
            for (index, message) in new.iter().enumerate() {
                if old
                    .iter()
                    .any(|previous| previous.id == message.id && previous.body != message.body)
                {
                    self.message_views.remove(&message.id);
                    self.message_list.splice(index..index + 1, 1);
                }
            }
            if let Some(anchor) = anchor {
                let index = new
                    .iter()
                    .position(|message| message.id == anchor)
                    .unwrap_or(offset.item_ix.min(new.len().saturating_sub(1)));
                self.message_list.scroll_to(ListOffset {
                    item_ix: index,
                    offset_in_item: offset.offset_in_item,
                });
            }
            self.message_views.retain(|id, _| new_ids.contains(&id));
        }
        if reset || !state.loading && (self.state.loading || state.error.is_some()) {
            self.older_pending = false;
        }
        if state.status == Status::Connected
            && let Some(store) = &self.store
            && store.settings().selected != state.selected
            && let Err(error) =
                store.update_settings(|settings| settings.selected = state.selected.clone())
        {
            self.error = Some(error.to_string());
        }
        self.state = state;
    }
    fn remove_server(&mut self, id: String, cx: &mut Context<Self>) {
        if self.mutation_busy() {
            return;
        }
        let Some(registry) = self.registry.clone() else {
            return;
        };
        self.connections.remove(&id);
        if self.active_server.as_ref() == Some(&id) {
            self.clear_connection();
        }
        self.auth_status = Some("Removing server…");
        let lease = self
            .persistence_gate
            .clone()
            .try_lock_owned()
            .expect("one credential mutation at a time");
        let worker = self.runtime.spawn(async move {
            let _lease = lease;
            registry.remove(&id).await
        });
        self.request = Some(self.request(
            async move { worker.await.map_err(|_| Error::Persistence)? },
            cx,
            |this, result, cx| {
                this.auth_status = None;
                if let Some(id) = this
                    .registry
                    .as_ref()
                    .and_then(|registry| registry.catalogue().active)
                {
                    this.switch_server(id, cx);
                } else {
                    this.store = None;
                    this.active_server = None;
                    this.adding = true;
                    this.managing = false;
                }
                match result {
                    Ok(true) => {}
                    Ok(false) => {
                        this.error =
                            Some("Removed locally. The server session could not be revoked.".into())
                    }
                    Err(error) => this.error = Some(error.to_string()),
                }
            },
        ));
    }
    fn sign_out(&mut self, _: &mut Window, cx: &mut Context<Self>) {
        if let Some(id) = self.active_server.clone() {
            self.remove_server(id, cx);
        }
    }
    fn load_older(&mut self, cx: &mut Context<Self>) {
        if self.older_pending
            || self.state.loading
            || !self.state.timeline.has_older
            || self.state.timeline.messages.len() >= 500
            || self.state.error.is_some()
        {
            return;
        }
        if let Some(server) = &self.server {
            match server.older() {
                Ok(()) => self.older_pending = true,
                Err(error) => self.error = Some(error.to_string()),
            }
            cx.notify();
        }
    }
    fn render_message(
        &mut self,
        index: usize,
        _window: &mut Window,
        cx: &mut Context<Self>,
    ) -> gpui::AnyElement {
        if self.media.is_none() {
            let runtime = self.runtime.clone();
            let origin = self.origin.clone();
            let store = self.store.clone();
            let room = self.state.selected.clone().unwrap_or_default();
            let preview = self.preview;
            let media = cx.new(|_| {
                let mut cache = media::MediaCache::new(runtime, origin, store, room);
                if preview {
                    cache.seed_preview();
                }
                cache
            });
            self.media_subscription = Some(cx.observe(&media, |this, _, cx| {
                this.message_list.remeasure();
                cx.notify();
            }));
            self.media = Some(media);
        }
        let media = self.media.as_ref().expect("room media initialized").clone();
        let state = self.state.clone();
        let directory = &state.directory;
        let Some(message) = state.timeline.messages.get(index) else {
            return div().into_any_element();
        };
        let avatar_url = state
            .timeline
            .users
            .get(&message.actor_id)
            .or_else(|| directory.users.get(&message.actor_id))
            .and_then(|profile| profile.avatar_url.clone());
        let author = state
            .timeline
            .users
            .get(&message.actor_id)
            .or_else(|| directory.users.get(&message.actor_id))
            .map(sidebar::profile_label)
            .unwrap_or_else(|| "Deleted user".into());
        let body = match message.body.as_deref() {
            Some("") | None => "[No text content]".to_owned(),
            Some(body) => body.to_owned(),
        };
        let source = body.clone();
        let text = self
            .message_views
            .entry(message.id.clone())
            .or_insert_with(|| {
                cx.new(|cx| {
                    let mut text = message_text::MessageText::new(&source, message.id.clone(), cx);
                    text.set_media(media.clone(), cx);
                    text
                })
            })
            .clone();
        let copy_body = message.body.clone().unwrap_or_default();
        let copy_id = message.id.clone();
        let mut message_body = div()
            .flex()
            .flex_col()
            .gap_3()
            .w_full()
            .min_w_0()
            .child(text.clone());
        for attachment in media::attachments(message) {
            message_body = message_body.child(media::image_view(&media, attachment, cx));
        }
        if let Some(preview) = message.metadata.get("linkPreview") {
            message_body = message_body.child(media::link_preview(&media, preview, cx));
        }
        let initials = author.clone();
        let day = ui::message_day(&message.created_at);
        let previous_day = index
            .checked_sub(1)
            .and_then(|previous| state.timeline.messages.get(previous))
            .map(|message| ui::message_day(&message.created_at))
            .unwrap_or_default();
        let row = div()
            .id(SharedString::from(format!("message:{}", message.id)))
            .debug_selector(move || format!("message-row-{index}"))
            .w_full()
            .min_w_0()
            .px_4()
            .py_1()
            .flex()
            .gap(ui::unit(ui::MESSAGE_GAP))
            .hover(|style| style.bg(rgb(0x1c1c1c)))
            .child(media::avatar(
                &media,
                &initials,
                avatar_url.as_deref(),
                format!("avatar:{}", message.actor_id),
                ui::MESSAGE_AVATAR,
                false,
                cx,
            ))
            .child(
                div()
                    .flex_1()
                    .min_w_0()
                    .child(
                        div()
                            .flex()
                            .w_full()
                            .items_center()
                            .gap_3()
                            .mb_1()
                            .child(
                                div()
                                    .flex_shrink_0()
                                    .font_weight(gpui::FontWeight::SEMIBOLD)
                                    .child(author),
                            )
                            .child(
                                div()
                                    .text_size(ui::unit(ui::CAPTION))
                                    .text_color(rgb(ui::MUTED))
                                    .child(ui::message_time(&message.created_at)),
                            )
                            .context_menu(move |menu, _, _| {
                                let body = copy_body.clone();
                                let id = copy_id.clone();
                                menu.item(
                                    PopupMenuItem::new("Copy message")
                                        .disabled(body.is_empty())
                                        .on_click(move |_, _, cx| {
                                            cx.write_to_clipboard(ClipboardItem::new_string(
                                                body.clone(),
                                            ))
                                        }),
                                )
                                .item(
                                    PopupMenuItem::new("Copy message ID").on_click(
                                        move |_, _, cx| {
                                            cx.write_to_clipboard(ClipboardItem::new_string(
                                                id.clone(),
                                            ))
                                        },
                                    ),
                                )
                            }),
                    )
                    .track_focus(&text.focus_handle(cx))
                    .child(message_body),
            )
            .into_any_element();
        if !day.is_empty() && day != previous_day {
            div()
                .w_full()
                .child(ui::date_divider(day))
                .child(row)
                .into_any_element()
        } else {
            row
        }
    }
    fn mark_read(&mut self, cx: &mut Context<Self>) {
        let (Some(room), Some(store)) = (self.state.selected.clone(), self.store.clone()) else {
            return;
        };
        self.request = Some(self.request(
            async move { store.client(false).await?.mark_room_read(&room).await },
            cx,
            |this, result, _cx| {
                if let Err(error) = result {
                    this.error = Some(error.to_string());
                }
                if let Some(server) = &this.server {
                    let _ = server.refresh();
                }
            },
        ));
    }
    fn select(&mut self, room: String, cx: &mut Context<Self>) {
        self.overview = false;
        if self.preview {
            Arc::make_mut(&mut self.state).selected = Some(room);
            cx.notify();
            return;
        }
        if let Some(server) = &self.server
            && let Err(error) = server.select(room)
        {
            self.error = Some(error.to_string());
        }
        cx.notify();
    }
    fn toggle(&mut self, id: String, cx: &mut Context<Self>) {
        if let Some(store) = &self.store
            && let Err(error) = store.update_settings(|settings| {
                if !settings.collapsed.remove(&id) {
                    settings.collapsed.insert(id);
                }
            })
        {
            self.error = Some(error.to_string());
        }
        cx.notify();
    }
    fn connection_form(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        if self.login_form.is_none() {
            let origin = cx.new(|cx| {
                InputState::new(window, cx)
                    .placeholder("https://chat.example.com")
                    .default_value(self.origin.clone())
            });
            let submit = cx.subscribe_in(&origin, window, |this, _, event, window, cx| {
                if matches!(event, InputEvent::PressEnter { .. }) {
                    this.connect(window, cx);
                }
            });
            self.login_form = Some(LoginForm {
                origin,
                _submit: submit,
            });
        }
        let form = self.login_form.as_ref().expect("login form initialized");
        div()
            .flex()
            .flex_col()
            .w_full()
            .max_w(ui::unit(380.))
            .flex_shrink_0()
            .gap_3()
            .child(
                div()
                    .mb_3()
                    .flex()
                    .items_center()
                    .gap_3()
                    .child(ui::avatar("Chatto", 44.))
                    .child(
                        div()
                            .text_size(ui::unit(ui::TITLE))
                            .font_weight(gpui::FontWeight::SEMIBOLD)
                            .text_color(rgb(ui::TOP))
                            .child("Chatto"),
                    ),
            )
            .child(
                div()
                    .mb_3()
                    .text_color(rgb(ui::MUTED))
                    .child("Sign in to your server to pick up the conversation."),
            )
            .child("Server URL")
            .child(Input::new(&form.origin).tab_index(0))
            .child(
                Button::new("sign-in")
                    .primary()
                    .label("Continue in browser")
                    .tab_index(1)
                    .on_click(cx.listener(|this, _, window, cx| this.connect(window, cx))),
            )
            .child(div().text_size(ui::unit(ui::CAPTION)).text_color(rgb(ui::MUTED)).child(
                "Sign in with your server’s normal login flow. Access and refresh tokens are saved in macOS Keychain.",
            ))
    }
}
fn button(id: &'static str, label: impl Into<SharedString>) -> Button {
    ui::text_button(id, label)
}
fn status_label(status: Status) -> &'static str {
    match status {
        Status::Connecting => "Connecting…",
        Status::Synchronizing => "Synchronizing…",
        Status::Connected => "Connected",
        Status::Reconnecting => "Reconnecting…",
        Status::Failed => "Connection paused",
    }
}

impl Render for ChattoNative {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        if window.rem_size() != px(16. * self.scale) {
            window.set_rem_size(px(16. * self.scale));
            ui::scale_theme(self.scale, cx);
        }
        let focus = self.focus.get_or_insert_with(|| cx.focus_handle()).clone();
        if window.focused(cx).is_none() {
            window.focus(&focus, cx);
        }
        let titlebar = self.app_titlebar(cx);
        let body = self.render_body(window, cx);
        div()
            .track_focus(&focus)
            .size_full()
            .flex()
            .flex_col()
            .bg(rgb(ui::SHELL))
            .font_family("IBM Plex Sans")
            .text_size(ui::unit(ui::BODY))
            .text_color(rgb(ui::TEXT))
            .on_action(cx.listener(|this, _: &ZoomIn, window, cx| {
                this.change_scale(this.scale + 0.1, window, cx)
            }))
            .on_action(cx.listener(|this, _: &ZoomOut, window, cx| {
                this.change_scale(this.scale - 0.1, window, cx)
            }))
            .on_action(
                cx.listener(|this, _: &ResetZoom, window, cx| this.change_scale(1., window, cx)),
            )
            .on_action(cx.listener(|this, _: &FindConversation, window, cx| {
                this.focus_search(window, cx);
            }))
            .child(titlebar)
            .child(
                div()
                    .flex_1()
                    .min_h_0()
                    .min_w_0()
                    .px(ui::unit(ui::OUTER_INSET))
                    .pb(ui::unit(ui::OUTER_INSET))
                    .child(ui::work_plane(body)),
            )
    }
}
impl ChattoNative {
    fn render_body(
        &mut self,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> impl IntoElement + use<> {
        let mut root = div().size_full().flex().flex_col().bg(rgb(ui::BACKGROUND));
        if let Some(error) = self
            .error
            .clone()
            .or_else(|| self.state.error.as_ref().map(ToString::to_string))
        {
            root = root.child(div().p_3().bg(rgb(0x572b35)).child(error));
        }
        if let Some(status) = self.auth_status {
            let mut pending = div()
                .flex_1()
                .h_full()
                .flex()
                .flex_col()
                .items_center()
                .justify_center()
                .gap_4()
                .child(status);
            if status == "Continue sign-in in your browser…" {
                pending = pending.child(
                    button("cancel-sign-in", "Cancel")
                        .on_click(cx.listener(|this, _, _, cx| this.cancel_sign_in(cx))),
                );
            }
            return root.child(
                div()
                    .flex()
                    .flex_1()
                    .min_h_0()
                    .child(self.server_gutter(cx))
                    .child(pending),
            );
        }
        if self.registry.is_none() && !self.preview {
            return root
                .justify_center()
                .items_center()
                .gap_4()
                .child("Could not open saved connection settings.")
                .child(
                    button("retry-storage", "Retry")
                        .on_click(cx.listener(|this, _, _, cx| this.restore(cx))),
                );
        }
        if self.managing {
            return root.child(
                div()
                    .flex()
                    .flex_1()
                    .min_h_0()
                    .child(self.server_gutter(cx))
                    .child(self.server_settings(cx)),
            );
        }
        let reauth = self.state.status == Status::Failed
            && matches!(self.state.error, Some(Error::Http(400 | 401 | 403)));
        if (self.adding || self.server.is_none() || reauth) && !self.preview {
            return root.child(
                div()
                    .flex()
                    .flex_1()
                    .min_h_0()
                    .child(self.server_gutter(cx))
                    .child(
                        div()
                            .id("sign-in-scroll")
                            .flex_1()
                            .h_full()
                            .min_h_0()
                            .overflow_y_scroll()
                            .child(
                                div()
                                    .w_full()
                                    .min_h_full()
                                    .flex()
                                    .flex_col()
                                    .justify_center()
                                    .items_center()
                                    .p_8()
                                    .child(self.connection_form(window, cx)),
                            ),
                    ),
            );
        }
        let state = self.state.clone();
        let directory = &state.directory;
        let settings = self
            .store
            .as_ref()
            .map(|store| store.settings())
            .unwrap_or_default();
        let mut sidebar = div()
            .w(ui::unit(ui::SIDEBAR_WIDTH))
            .h_full()
            .flex_shrink_0()
            .flex()
            .flex_col()
            .bg(rgb(ui::BACKGROUND))
            .border_r_1()
            .border_color(rgb(ui::BORDER));
        sidebar = sidebar.child(
            ui::pane_header(if directory.server.name.is_empty() {
                "Chatto".to_owned()
            } else {
                directory.server.name.clone()
            })
            .debug_selector(|| "sidebar-header".into()),
        );
        if self.room_search.is_none() {
            let input = cx.new(|cx| InputState::new(window, cx).placeholder("Find a conversation"));
            self.search_subscription = Some(cx.subscribe(&input, |_, _, event, cx| {
                if matches!(event, InputEvent::Change) {
                    cx.notify();
                }
            }));
            self.room_search = Some(input);
        }
        let search = self.room_search.as_ref().expect("search initialized");
        let query = search.read(cx).value().trim().to_lowercase();
        sidebar = sidebar
            .child(self.sidebar_artwork(cx))
            .child(self.sidebar_navigation(cx));
        if self.search_visible {
            sidebar = sidebar.child(
                div()
                    .px_2()
                    .py_2()
                    .child(Input::new(search).prefix(gpui::component::IconName::Search)),
            );
        }
        let mut rows = div()
            .id("room-directory")
            .flex_1()
            .min_h_0()
            .overflow_y_scroll()
            .track_scroll(&self.room_scroll)
            .px_2()
            .py_3();
        let mut has_matches = false;
        for mut section in sidebar::sections(directory) {
            if !query.is_empty() {
                section.items.retain(|item| match item {
                    sidebar::Item::Room { label, .. } | sidebar::Item::Link { label, .. } => {
                        label.to_lowercase().contains(&query)
                    }
                });
                if section.items.is_empty() {
                    continue;
                }
            }
            has_matches |= !section.items.is_empty();
            let id = section.id.clone();
            let collapsed = query.is_empty() && settings.collapsed.contains(&section.id);
            let suffix = if section.unread > 0 {
                format!("  {}", section.unread)
            } else {
                String::new()
            };
            rows = rows.child(
                Button::new(SharedString::from(format!("section:{id}")))
                    .ghost()
                    .w_full()
                    .justify_start()
                    .small()
                    .child(
                        div()
                            .w_full()
                            .flex()
                            .gap_2()
                            .text_color(rgb(ui::MUTED))
                            .child(if collapsed {
                                gpui::component::IconName::ChevronRight
                            } else {
                                gpui::component::IconName::ChevronDown
                            })
                            .text_size(ui::unit(ui::CAPTION))
                            .font_weight(gpui::FontWeight::SEMIBOLD)
                            .child(format!("{}{suffix}", section.label.to_uppercase())),
                    )
                    .on_click(cx.listener(move |this, _, _, cx| this.toggle(id.clone(), cx))),
            );
            if !collapsed {
                if section.items.is_empty() {
                    rows = rows.child(
                        div()
                            .px_3()
                            .py_2()
                            .text_size(ui::unit(ui::CAPTION))
                            .text_color(rgb(ui::MUTED))
                            .child("No conversations yet"),
                    );
                }
                for item in section.items {
                    match item {
                        sidebar::Item::Room {
                            id,
                            label,
                            unread,
                            important,
                            dm,
                        } => {
                            let selected = state.selected.as_ref() == Some(&id);
                            let mut row = div()
                                .flex()
                                .items_center()
                                .gap_2()
                                .w_full()
                                .min_w_0()
                                .child(
                                    div()
                                        .w(ui::unit(18.))
                                        .text_color(rgb(if selected {
                                            ui::ACTION
                                        } else {
                                            ui::MUTED
                                        }))
                                        .child(if dm { "@" } else { "#" }),
                                )
                                .child(
                                    div()
                                        .flex_1()
                                        .overflow_hidden()
                                        .font_weight(if unread {
                                            gpui::FontWeight::SEMIBOLD
                                        } else {
                                            gpui::FontWeight::NORMAL
                                        })
                                        .child(if dm {
                                            label
                                        } else {
                                            label.trim_start_matches('#').to_owned()
                                        }),
                                );
                            if important > 0 {
                                row = row.child(
                                    div()
                                        .px_2()
                                        .rounded_md()
                                        .bg(rgb(ui::ACTION))
                                        .text_color(rgb(ui::BACKGROUND))
                                        .text_size(ui::unit(ui::BADGE))
                                        .child(important.to_string()),
                                );
                            } else if unread {
                                row = row.child(
                                    div().size(ui::unit(6.)).rounded_full().bg(rgb(0xe4e4e7)),
                                );
                            }
                            rows = rows.child(
                                Button::new(SharedString::from(format!("room:{id}")))
                                    .ghost()
                                    .w_full()
                                    .h(ui::unit(32.))
                                    .justify_start()
                                    .selected(selected)
                                    .bg(rgb(if selected {
                                        ui::SELECTED
                                    } else {
                                        ui::BACKGROUND
                                    }))
                                    .child(row)
                                    .on_click(cx.listener(move |this, _, _, cx| {
                                        this.select(id.clone(), cx)
                                    })),
                            );
                        }
                        sidebar::Item::Link { id, label, url } => {
                            let destination = url::Url::parse(&self.origin)
                                .and_then(|base| base.join(&url))
                                .ok()
                                .filter(|url| {
                                    matches!(url.scheme(), "https" | "http")
                                        && url.username().is_empty()
                                        && url.password().is_none()
                                });
                            if let Some(destination) = destination {
                                rows = rows.child(
                                    Button::new(SharedString::from(format!("link:{id}")))
                                        .ghost()
                                        .w_full()
                                        .justify_start()
                                        .label(format!("{label} ↗"))
                                        .on_click(move |_, _, cx| {
                                            cx.open_url(destination.as_str())
                                        }),
                                );
                            }
                        }
                    }
                }
                rows = rows.child(div().h(ui::unit(8.)));
            }
        }
        if !query.is_empty() && !has_matches {
            rows = rows.child(
                div()
                    .px_3()
                    .py_2()
                    .text_color(rgb(ui::MUTED))
                    .child("No conversations found"),
            );
        }
        sidebar = sidebar.child(rows).child(self.account_card(cx));
        let mut content = div().flex_1().min_w_0().h_full().flex().flex_col();
        if let Some(room) = &state.selected {
            let label = directory
                .rooms
                .iter()
                .find(|entry| entry.room.id == *room)
                .map(|entry| sidebar::room_label(entry, directory))
                .unwrap_or_else(|| "Conversation".into());
            let description = directory
                .rooms
                .iter()
                .find(|entry| entry.room.id == *room)
                .map(|entry| entry.room.description.clone())
                .unwrap_or_default();
            content = content.child(
                ui::pane_header(
                    label
                        .strip_prefix('#')
                        .map(|name| format!("# {name}"))
                        .unwrap_or(label),
                )
                .debug_selector(|| "channel-header".into())
                .child(
                    div()
                        .flex_1()
                        .min_w_0()
                        .overflow_hidden()
                        .text_size(ui::unit(ui::BODY))
                        .text_color(rgb(ui::MUTED))
                        .child(description),
                )
                .child(
                    ui::icon_button("toggle-people", gpui::assets::IconName::Users, "People")
                        .selected(self.people_visible)
                        .when(self.people_visible, |button| {
                            button.bg(rgb(ui::SELECTED)).text_color(rgb(ui::ACTION))
                        })
                        .on_click(cx.listener(|this, _, _, cx| {
                            this.people_visible = !this.people_visible;
                            cx.notify();
                        })),
                )
                .child(
                    ui::icon_button(
                        "room-overview",
                        gpui::assets::IconName::Info,
                        "Server overview",
                    )
                    .on_click(cx.listener(|this, _, _, cx| {
                        this.overview = true;
                        cx.notify();
                    })),
                )
                .child(
                    ui::icon_button("mark-read", gpui::assets::IconName::Check, "Mark read")
                        .small()
                        .disabled(self.preview)
                        .on_click(cx.listener(|this, _, _, cx| this.mark_read(cx))),
                ),
            );
            let view = cx.entity().downgrade();
            self.message_list
                .set_scroll_handler(move |event, window, cx| {
                    let view = view.clone();
                    let visible_start = event.visible_range.start;
                    let visible_count = event.visible_range.len();
                    let scrolled = event.is_scrolled;
                    // GPUI reports the previous painted range here and still
                    // borrows ListState. Defer so we can read the new scroll anchor.
                    window.defer(cx, move |window, cx| {
                        let _ = view.update(cx, |this, cx| {
                            let top = if scrolled {
                                this.message_list.logical_scroll_top().item_ix
                            } else {
                                visible_start
                            };
                            if top <= 2 {
                                this.load_older(cx);
                            }
                            let start = top.saturating_sub(20);
                            let end = top.saturating_add(visible_count).saturating_add(20);
                            this.message_views.retain(|id, view| {
                                view.focus_handle(cx).is_focused(window)
                                    || this
                                        .state
                                        .timeline
                                        .messages
                                        .iter()
                                        .position(|message| &message.id == id)
                                        .is_some_and(|index| (start..end).contains(&index))
                            });
                        });
                    });
                });
            if state.loading {
                content = content.child(
                    div()
                        .px_5()
                        .py_1()
                        .text_size(ui::unit(ui::CAPTION))
                        .text_color(rgb(ui::MUTED))
                        .child("Loading messages…"),
                );
            }
            if state.timeline.messages.is_empty() && !state.loading {
                content = content.child(
                    div()
                        .p_5()
                        .child("No messages yet. This is the start of the conversation."),
                );
            }
            content = content.child(
                list(
                    self.message_list.clone(),
                    cx.processor(|this, index, window, cx| this.render_message(index, window, cx)),
                )
                .flex_1()
                .min_h_0()
                .w_full()
                .pb_5(),
            );
        } else {
            content = content
                .justify_center()
                .items_center()
                .gap_2()
                .child(div().text_size(ui::unit(ui::TITLE)).child(
                    if state.status == Status::Connected {
                        "Select a conversation"
                    } else {
                        status_label(state.status)
                    },
                ))
                .child(
                    div()
                        .text_color(rgb(ui::MUTED))
                        .child("Your channels and direct messages appear in the sidebar."),
                );
        }
        let people = self.people_visible.then(|| self.people_panel(window, cx));
        let compact_people =
            self.people_visible && window.viewport_size().width < px(1120. * self.scale);
        root.child(
            div()
                .flex()
                .flex_1()
                .min_h_0()
                .child(self.server_gutter(cx))
                .when(self.sidebar_visible, |view| view.child(sidebar))
                .when(!compact_people, |view| {
                    view.child(if self.overview {
                        self.server_overview(cx).into_any_element()
                    } else {
                        content.into_any_element()
                    })
                })
                .when_some(people, |view, panel| view.child(panel)),
        )
    }
}
/// Keep native actions identical in the app and headless interaction checks.
fn bind_keys(cx: &mut App) {
    cx.bind_keys([
        KeyBinding::new("cmd-q", Quit, None),
        KeyBinding::new("cmd-k", FindConversation, None),
        KeyBinding::new("cmd-=", ZoomIn, None),
        KeyBinding::new("cmd-+", ZoomIn, None),
        KeyBinding::new("cmd-shift-=", ZoomIn, None),
        KeyBinding::new("cmd--", ZoomOut, None),
        KeyBinding::new("cmd-0", ResetZoom, None),
        KeyBinding::new("ctrl-=", ZoomIn, None),
        KeyBinding::new("ctrl-+", ZoomIn, None),
        KeyBinding::new("ctrl--", ZoomOut, None),
        KeyBinding::new("ctrl-0", ResetZoom, None),
        KeyBinding::new("cmd-c", message_text::Copy, Some("MessageText")),
        KeyBinding::new("cmd-a", message_text::SelectAll, Some("MessageText")),
    ]);
}

fn main() {
    let preview = std::env::args().any(|arg| arg == "--preview");
    let check_fonts = std::env::args().any(|arg| arg == "--check-fonts");
    let runtime = Arc::new(
        tokio::runtime::Builder::new_multi_thread()
            .worker_threads(2)
            .enable_all()
            .build()
            .expect("failed to start the network runtime"),
    );
    let app_runtime = runtime.clone();
    let persistence_gate = Arc::new(tokio::sync::Mutex::new(()));
    let app_gate = persistence_gate.clone();
    gpui::application()
        .with_assets(gpui::assets::AllAssets)
        .run(move |cx: &mut App| {
            if check_fonts {
                // GPUI's dummy backend lists fallback names and gives every ASCII
                // glyph the same advance. The native proportional font must give
                // W more space than i. Exercise the backend selected by this binary.
                ui::init(cx);
                let text = cx.text_system();
                let font = text.resolve_font(&gpui::font("IBM Plex Sans"));
                let wide = text.advance(font, px(16.), 'W');
                let narrow = text.advance(font, px(16.), 'i');
                if !matches!((wide, narrow), (Ok(wide), Ok(narrow)) if wide.width > narrow.width) {
                    eprintln!("GPUI has no native font metrics; enable its font-kit feature");
                    std::process::exit(1);
                }
                cx.quit();
                return;
            }
            ui::init(cx);
            // mise and terminal interrupts must use the same credential and
            // URL-handler cleanup path as closing the native window.
            let signal = app_runtime.spawn(async {
                let mut terminate = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate()).map_err(|_| ())?;
                tokio::select! { result = tokio::signal::ctrl_c() => result.map_err(|_| ()), _ = terminate.recv() => Ok(()) }
            });
            cx.spawn(async move |cx| { if matches!(signal.await, Ok(Ok(()))) { cx.update(|cx| cx.quit()); } }).detach();
            cx.on_action(|_: &Quit, cx| cx.quit());
            bind_keys(cx);
            cx.on_window_closed(|cx, _| {
                if cx.windows().is_empty() {
                    cx.quit();
                }
            })
            .detach();
            let bounds = Bounds::centered(None, size(px(1280.0), px(800.0)), cx);
            cx.open_window(
                WindowOptions {
                    window_bounds: Some(WindowBounds::Windowed(bounds)),
                    window_min_size: Some(size(px(900.0), px(600.0))),
                    ..gpui::component::TitleBar::window_options()
                },
                |window, cx| {
                    #[cfg(feature = "preview-capture")]
                    if preview && let Some(path) = std::env::var_os("CHATTO_NATIVE_PREVIEW_CAPTURE")
                    {
                        // Draw explicitly: occluded macOS windows can pause display-link
                        // callbacks, but an offline capture must not depend on visibility.
                        window.spawn(cx, async move |cx| {
                            for _ in 0..3 {
                                cx.background_executor().timer(std::time::Duration::from_millis(100)).await;
                                cx.update(|window, cx| {
                                    window.refresh();
                                    let _ = window.draw(cx);
                                }).expect("draw native preview");
                            }
                            cx.update(|window, cx| {
                                window.render_to_image().expect("render native preview")
                                    .save(path).expect("save native preview");
                                cx.quit();
                            }).expect("capture native preview");
                        }).detach();
                    }
                    let view = cx.new(|cx| {
                        let mut app = ChattoNative::new(app_runtime.clone());
                        app.persistence_gate = app_gate.clone();
                        if preview {
                            app.preview = true;
                            app.people_visible = std::env::var_os("CHATTO_NATIVE_PREVIEW_PEOPLE").is_some();
                            app.scale = ui::normalize_scale(
                                std::env::var("CHATTO_NATIVE_PREVIEW_SCALE")
                                    .ok()
                                    .and_then(|value| value.parse().ok())
                                    .unwrap_or(1.),
                            );
                            app.state = Arc::new(preview::state());
                            app.message_list.reset(app.state.timeline.messages.len());
                        } else {
                            app.restore(cx);
                        }
                        app
                    });
                    cx.new(|cx| Root::new(view, window, cx))
                },
            )
            .expect("failed to open the Chatto Native window");
            cx.activate(true);
        });
    runtime.block_on(async {
        let _lease = persistence_gate.lock().await;
        browser_auth::drain().await;
    });
}

#[cfg(test)]
mod timeline_ui_tests {
    use super::*;
    use gpui::{AppContext, ScrollDelta, ScrollEvent, TestAppContext, point};

    #[gpui::test]
    fn switching_servers_keeps_live_snapshots_and_scopes_rooms_and_private_views(
        cx: &mut TestAppContext,
    ) {
        cx.update(gpui::init);
        let runtime = Arc::new(tokio::runtime::Runtime::new().unwrap());
        let directory = std::env::current_dir()
            .unwrap()
            .join("../../.context")
            .join(format!("native-multi-ui-{}", uuid::Uuid::new_v4()));
        let opener = Storage::test_opener();
        let registry = registry::Registry::open_with(directory.clone(), opener.clone()).unwrap();
        let make_session = |origin: &str| {
            chatto::HumanSession::restore(chatto::SessionCredentials {
                origin: origin.into(),
                access_token: "synthetic-multi-access".into(),
                refresh_token: "synthetic-multi-refresh".into(),
                refresh_request_id: String::new(),
                oauth_client_id: chatto::oauth::DESKTOP_CLIENT_ID.into(),
                access_expires_at: std::time::SystemTime::now()
                    + std::time::Duration::from_secs(900),
                session_expires_at: std::time::SystemTime::now()
                    + std::time::Duration::from_secs(3600),
            })
            .unwrap()
        };
        let first = runtime
            .block_on(registry.install(make_session("http://127.0.0.1:1"), "First".into()))
            .unwrap();
        let second = runtime
            .block_on(registry.install(make_session("http://127.0.0.1:2"), "Second".into()))
            .unwrap();
        let view = cx.new(|cx| {
            let mut app = ChattoNative::new(runtime.clone());
            app.registry = Some(registry.clone());
            for (id, room) in [(&first, "first-room"), (&second, "second-room")] {
                let server = {
                    let _enter = runtime.enter();
                    Arc::new(LiveServer::start(
                        Arc::new(|_| Box::pin(std::future::pending())),
                        None,
                        500,
                    ))
                };
                let mut state = preview::state();
                state.selected = Some(room.into());
                state.timeline.messages[0].id = room.into();
                let public_media = cx.new(|_| {
                    media::MediaCache::new(
                        runtime.clone(),
                        registry.store(id).unwrap().settings().origin,
                        None,
                        String::new(),
                    )
                });
                let subscription = cx.observe(&public_media, |_, _, cx| cx.notify());
                app.connections.insert(
                    id.clone(),
                    Connection {
                        server,
                        state: Arc::new(state),
                        _stream: cx.spawn(async |_, _| std::future::pending().await),
                        public_media,
                        _media_subscription: subscription,
                    },
                );
            }
            app.switch_server(first.clone(), cx);
            app
        });
        view.update(cx, |app, cx| {
            let mut background = app.connections[&second].state.as_ref().clone();
            background.timeline.messages[0].id = "second-updated".into();
            app.accept_server(&second, Arc::new(background), cx);
            assert_eq!(app.state.selected.as_deref(), Some("first-room"));
            let text = cx.new(|cx| {
                message_text::MessageText::new(
                    "Private first-server selection",
                    "first-message".into(),
                    cx,
                )
            });
            app.message_views.insert("first-message".into(), text);
            app.media = Some(cx.new(|_| {
                media::MediaCache::new(
                    runtime.clone(),
                    "http://127.0.0.1:1".into(),
                    app.store.clone(),
                    "first-room".into(),
                )
            }));
            app.switch_server(second.clone(), cx);
            assert!(
                app.message_views.is_empty() && app.media.is_none(),
                "private text selection and images must not cross server boundaries"
            );
            assert_eq!(app.state.timeline.messages[0].id, "second-updated");
            assert!(Arc::ptr_eq(
                app.store.as_ref().unwrap(),
                &registry.store(&second).unwrap()
            ));
            assert_eq!(
                app.connections.len(),
                2,
                "switching must keep both realtime projections alive"
            );
            app.switch_server(first.clone(), cx);
            assert_eq!(app.state.selected.as_deref(), Some("first-room"));
            assert_eq!(app.state.timeline.messages[0].id, "first-room");
            app.clear_connection();
            app.connections.clear();
            app.registry = None;
            app.store = None;
        });
        drop(registry);
        let restored = registry::Registry::open_with(directory.clone(), opener).unwrap();
        assert_eq!(restored.catalogue().servers.len(), 2);
        assert_eq!(restored.catalogue().active.as_deref(), Some(first.as_str()));
        assert_eq!(
            restored
                .store(&second)
                .unwrap()
                .settings()
                .selected
                .as_deref(),
            Some("second-room")
        );
        drop(restored);
        std::fs::remove_dir_all(directory).unwrap();
    }
    #[gpui::test]
    fn long_messages_wrap_within_the_scaled_pane(cx: &mut TestAppContext) {
        cx.update(gpui::init);
        let runtime = Arc::new(
            tokio::runtime::Builder::new_multi_thread()
                .worker_threads(1)
                .enable_all()
                .build()
                .unwrap(),
        );
        let view = cx.new(|_| {
            let mut app = ChattoNative::new(runtime);
            app.preview = true;
            let mut state = preview::state();
            state.timeline.messages.truncate(1);
            state.timeline.messages[0].body =
                Some("A longer message needs to wrap within the conversation pane. ".repeat(8));
            app.state = Arc::new(state);
            app.message_list.reset(1);
            app
        });
        let handle = cx.open_window(size(px(1100.), px(740.)), |window, cx| {
            Root::new(view.clone(), window, cx)
        });
        let mut visual = gpui::VisualTestContext::from_window(handle.into(), cx);
        let cx = &mut visual;
        cx.run_until_parked();
        let mut baseline = px(0.);
        for scale in [1., 1.5] {
            cx.update(|window, cx| view.update(cx, |app, cx| app.change_scale(scale, window, cx)));
            cx.draw(
                point(px(0.), px(0.)),
                size(px(1100.), px(740.)),
                |window, cx| {
                    cx.new(|cx| Root::new(view.clone(), window, cx))
                        .into_any_element()
                },
            );
            let row = cx.debug_bounds("message-row-0").expect("visible message");
            assert_eq!(
                row.size.width,
                px(1100.
                    - (ui::GUTTER_WIDTH + ui::SIDEBAR_WIDTH + ui::OUTER_INSET * 2.) * scale
                    - 2.)
            );
            if scale == 1. {
                baseline = row.size.height;
            } else {
                assert!(
                    row.size.height > baseline * scale,
                    "larger text in a narrower pane must wrap onto more lines"
                );
            }
        }
    }

    #[gpui::test]
    fn pane_headers_align_and_find_shortcut_focuses_the_filter(cx: &mut TestAppContext) {
        cx.update(gpui::init);
        cx.update(bind_keys);
        let runtime = Arc::new(
            tokio::runtime::Builder::new_multi_thread()
                .worker_threads(1)
                .enable_all()
                .build()
                .unwrap(),
        );
        let view = cx.new(|_| {
            let mut app = ChattoNative::new(runtime);
            app.preview = true;
            app.state = Arc::new(preview::state());
            app.message_list.reset(app.state.timeline.messages.len());
            app
        });
        let (_, cx) = cx.add_window_view(|window, cx| Root::new(view.clone(), window, cx));
        for (width, scale) in [
            (1100., 1.),
            (900., 0.5),
            (900., 1.5),
            (1100., 2.),
            (900., 1.),
        ] {
            cx.update(|window, cx| view.update(cx, |app, cx| app.change_scale(scale, window, cx)));
            cx.draw(
                point(px(0.), px(0.)),
                size(px(width), px(600.)),
                |window, cx| {
                    cx.new(|cx| Root::new(view.clone(), window, cx))
                        .into_any_element()
                },
            );
            let sidebar = cx.debug_bounds("sidebar-header").expect("sidebar header");
            let channel = cx.debug_bounds("channel-header").expect("channel header");
            assert_eq!(sidebar.origin.y, channel.origin.y);
            assert_eq!(sidebar.size.height, channel.size.height);
            assert_eq!(sidebar.size.height, px(ui::HEADER_HEIGHT * scale));
            // Header occupies the sidebar content box, inside its 1px border.
            assert_eq!(sidebar.size.width, px(ui::SIDEBAR_WIDTH * scale - 1.));
        }
        cx.simulate_keystrokes("cmd-k");
        cx.simulate_input("design");
        cx.simulate_keystrokes("cmd-=");
        view.read_with(cx, |app, _| assert_eq!(app.scale, 1.1));
        cx.simulate_keystrokes("cmd--");
        view.read_with(cx, |app, _| assert_eq!(app.scale, 1.));
        cx.simulate_keystrokes("cmd-+ cmd-0");
        view.read_with(cx, |app, _| assert_eq!(app.scale, 1.));
        cx.simulate_keystrokes(&vec!["cmd-="; 25].join(" "));
        view.read_with(cx, |app, _| assert_eq!(app.scale, 2.));
        cx.simulate_keystrokes(&vec!["cmd--"; 25].join(" "));
        view.read_with(cx, |app, _| assert_eq!(app.scale, 0.5));
        cx.simulate_keystrokes("cmd-0");
        view.read_with(cx, |app, cx| {
            assert_eq!(
                app.room_search.as_ref().unwrap().read(cx).value().as_ref(),
                "design"
            );
            assert_eq!(
                app.state.selected.as_deref(),
                Some("general"),
                "filtering must not switch conversations"
            );
            assert!(
                app.store.is_none() && app.server.is_none(),
                "offline preview must not open private state or a connection"
            );
        });
    }

    #[gpui::test]
    fn people_pane_aligns_filters_and_reflows_without_changing_the_room(cx: &mut TestAppContext) {
        cx.update(gpui::init);
        let runtime = Arc::new(
            tokio::runtime::Builder::new_multi_thread()
                .worker_threads(1)
                .enable_all()
                .build()
                .unwrap(),
        );
        let view = cx.new(|_| {
            let mut app = ChattoNative::new(runtime);
            app.preview = true;
            app.people_visible = true;
            app.state = Arc::new(preview::state());
            app.message_list.reset(app.state.timeline.messages.len());
            app
        });
        let (_, cx) = cx.add_window_view(|window, cx| Root::new(view.clone(), window, cx));
        cx.simulate_resize(size(px(1280.), px(800.)));
        cx.draw(
            point(px(0.), px(0.)),
            size(px(1280.), px(800.)),
            |window, cx| {
                cx.new(|cx| Root::new(view.clone(), window, cx))
                    .into_any_element()
            },
        );
        let sidebar = cx.debug_bounds("sidebar-header").unwrap();
        let channel = cx.debug_bounds("channel-header").unwrap();
        let people = cx.debug_bounds("people-header").unwrap();
        assert_eq!(sidebar.origin.y, people.origin.y);
        assert_eq!(channel.size.height, people.size.height);
        let search = view.read_with(cx, |app, _| app.people_search.as_ref().unwrap().clone());
        assert!(cx.debug_bounds("person-maya").is_some());
        cx.update(|window, cx| {
            search.update(cx, |search, cx| {
                search.set_value("no-such-person", window, cx)
            })
        });
        cx.simulate_resize(size(px(900.), px(600.)));
        cx.update(|window, cx| {
            window.refresh();
            let _ = window.draw(cx);
        });
        cx.draw(
            point(px(0.), px(0.)),
            size(px(900.), px(600.)),
            |window, cx| {
                cx.new(|cx| Root::new(view.clone(), window, cx))
                    .into_any_element()
            },
        );
        assert!(cx.debug_bounds("people-header").is_some());
        assert!(cx.debug_bounds("channel-header").is_none());
        assert!(cx.debug_bounds("person-maya").is_none());
        view.read_with(cx, |app, _| {
            assert_eq!(app.state.selected.as_deref(), Some("general"))
        });
    }

    #[gpui::test]
    fn scrolling_the_real_timeline_requests_history_once_and_keeps_rows_virtual(
        cx: &mut TestAppContext,
    ) {
        cx.update(gpui::component::init);
        let runtime = Arc::new(
            tokio::runtime::Builder::new_multi_thread()
                .worker_threads(1)
                .enable_all()
                .build()
                .unwrap(),
        );
        let dir = std::path::PathBuf::from("../../.context")
            .join(format!("timeline-ui-{}", uuid::Uuid::new_v4()));
        let store = Storage::fixture(dir.clone());
        let server = {
            let _enter = runtime.enter();
            Arc::new(LiveServer::start(
                Arc::new(|_| Box::pin(std::future::pending())),
                Some("room".into()),
                500,
            ))
        };
        let view = cx.new(|_| {
            let mut app = ChattoNative::new(runtime.clone());
            app.store = Some(store.clone());
            app.preview = true;
            app.adding = false;
            app.server = Some(server.clone());
            let mut state = ServerState {
                status: Status::Connected,
                selected: Some("room".into()),
                ..Default::default()
            };
            state.timeline.messages = (0..100)
                .map(|index| chatto::Message {
                    id: format!("message-{index}"),
                    actor_id: String::new(),
                    body: Some(format!(
                        "**Message {index}**\n\nA paragraph with *emphasis*.{}",
                        "\nMore text.".repeat(index % 4)
                    )),
                    created_at: String::new(),
                    deleted_at: None,
                    metadata: BTreeMap::new(),
                })
                .collect();
            state.timeline.has_older = true;
            state.timeline.start_cursor = "older-boundary".into();
            app.message_list.reset(100);
            app.state = Arc::new(state);
            app
        });
        let cx = cx.add_empty_window();
        let paint = |cx: &mut gpui::VisualTestContext| {
            cx.draw(
                point(px(0.), px(0.)),
                size(px(1100.), px(740.)),
                |window, cx| {
                    cx.new(|cx| Root::new(view.clone(), window, cx))
                        .into_any_element()
                },
            );
        };
        paint(cx);
        view.read_with(cx, |app, _| {
            assert!(
                app.message_views.len() < 50,
                "Offscreen rows must not create views"
            )
        });
        view.update(cx, |app, _| {
            app.message_list.scroll_to(ListOffset {
                item_ix: 3,
                offset_in_item: px(0.),
            })
        });
        paint(cx);
        cx.update(|window, cx| view.update(cx, |app, cx| app.change_scale(1.5, window, cx)));
        paint(cx);
        view.read_with(cx, |app, _| {
            assert_eq!(
                app.message_list.logical_scroll_top().item_ix,
                3,
                "zoom must keep the visible message"
            )
        });
        cx.simulate_event(ScrollEvent {
            position: point(px(800.), px(400.)),
            delta: ScrollDelta::Pixels(point(px(0.), px(10_000.))),
            ..Default::default()
        });
        view.read_with(cx, |app, _| {
            assert!(
                app.older_pending,
                "A scroll at the top must request the preceding page"
            )
        });
        // The next wheel event must keep the existing request pending.
        paint(cx);
        cx.simulate_event(ScrollEvent {
            position: point(px(800.), px(400.)),
            delta: ScrollDelta::Pixels(point(px(0.), px(100.))),
            ..Default::default()
        });
        view.read_with(cx, |app, _| assert!(app.older_pending));
        view.update(cx, |app, _| app.clear_connection());
        drop(store);
        drop(server);
        std::fs::remove_dir_all(dir).unwrap();
    }
}
