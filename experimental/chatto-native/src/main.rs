mod message_text;
mod sidebar;
mod storage;
mod timeline;

use chatto::{
    Client, Error,
    realtime::{LiveServer, ServerState, Status},
};
use gpui::{
    App, Application, Bounds, ClipboardItem, Context, Entity, Focusable, KeyBinding, ListAlignment,
    ListOffset, ListState, ScrollHandle, SharedString, Subscription, Task, Window, WindowBounds,
    WindowOptions, actions, div, list, prelude::*, px, rgb, size,
};
use gpui_component::{
    Root, Selectable, Sizable, Theme, ThemeMode,
    button::{Button, ButtonVariants},
    input::{Input, InputEvent, InputState},
    menu::{ContextMenuExt, PopupMenuItem},
};
use std::{collections::BTreeMap, future::Future, sync::Arc};
use storage::Storage;
use tokio::{runtime::Runtime, task::AbortHandle};

actions!(chatto_native, [Quit]);

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
    username: Entity<InputState>,
    password: Entity<InputState>,
    _submit: Subscription,
}

struct ChattoNative {
    runtime: Arc<Runtime>,
    origin: String,
    store: Option<Arc<Storage>>,
    saved_session: bool,
    login_form: Option<LoginForm>,
    auth_status: Option<&'static str>,
    request: Option<Request>,
    server: Option<Arc<LiveServer>>,
    stream: Option<Task<()>>,
    state: Arc<ServerState>,
    error: Option<String>,
    room_scroll: ScrollHandle,
    message_list: ListState,
    message_views: BTreeMap<String, Entity<message_text::MessageText>>,
    older_pending: bool,
}
impl ChattoNative {
    fn new(runtime: Arc<Runtime>) -> Self {
        let port = std::env::var("CHATTO_DEV_CHATTO_PORT")
            .ok()
            .and_then(|value| value.parse::<u16>().ok())
            .unwrap_or(4000);
        Self {
            runtime,
            origin: format!("http://127.0.0.1:{port}"),
            store: None,
            saved_session: false,
            login_form: None,
            auth_status: None,
            request: None,
            server: None,
            stream: None,
            state: Arc::new(ServerState::default()),
            error: None,
            room_scroll: ScrollHandle::new(),
            message_list: ListState::new(0, ListAlignment::Bottom, px(240.)),
            message_views: BTreeMap::new(),
            older_pending: false,
        }
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
        self.auth_status = Some("Restoring your connection…");
        self.error = None;
        self.request = Some(self.request(
            async {
                tokio::task::spawn_blocking(Storage::open_default)
                    .await
                    .map_err(|_| Error::Persistence)?
            },
            cx,
            |this, result, cx| {
                this.auth_status = None;
                match result {
                    Ok((store, saved)) => {
                        let settings = store.settings();
                        if !settings.origin.is_empty() {
                            this.origin = settings.origin;
                        }
                        this.store = Some(store);
                        this.saved_session = saved;
                        if saved {
                            this.start_connection(cx);
                        }
                    }
                    Err(error) => this.error = Some(error.to_string()),
                }
            },
        ));
    }
    fn clear_connection(&mut self) {
        self.stream = None;
        self.server = None;
        self.request = None;
        self.login_form = None;
        self.state = Arc::new(ServerState::default());
        self.auth_status = None;
        self.error = None;
        self.room_scroll = ScrollHandle::new();
        self.message_list.reset(0);
        self.message_views.clear();
        self.older_pending = false;
    }
    fn connect(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let (Some(form), Some(store)) = (self.login_form.as_ref(), self.store.clone()) else {
            return;
        };
        let origin = form.origin.read(cx).value().trim().to_owned();
        let username = form.username.read(cx).value().trim().to_owned();
        let password = form.password.read(cx).value().to_string();
        if origin.is_empty() || username.is_empty() || password.is_empty() {
            self.error = Some("Enter the server URL, username, and password".into());
            cx.notify();
            return;
        }
        window.blur();
        self.clear_connection();
        self.origin = origin.clone();
        self.auth_status = Some("Signing in…");
        self.request = Some(self.request(
            async move {
                let session = Client::login_password(&origin, &username, &password).await?;
                if let Err(error) = store.install(session.clone()).await {
                    let _ = session.logout().await;
                    return Err(error);
                }
                Ok(())
            },
            cx,
            |this, result, cx| {
                this.auth_status = None;
                match result {
                    Ok(()) => {
                        this.saved_session = true;
                        this.start_connection(cx);
                    }
                    Err(error) => {
                        this.error = Some(match error {
                            Error::Http(401) => "Incorrect username or password".into(),
                            Error::Http(403) => {
                                "Password sign-in is not enabled or permitted on this server".into()
                            }
                            _ => error.to_string(),
                        })
                    }
                }
            },
        ));
        cx.notify();
    }
    fn start_connection(&mut self, cx: &mut Context<Self>) {
        let Some(store) = self.store.clone() else {
            return;
        };
        self.clear_connection();
        self.saved_session = true;
        let selected = store.settings().selected;
        let server = {
            let _enter = self.runtime.enter();
            Arc::new(LiveServer::start(store.provider(), selected, 500))
        };
        let mut updates = server.updates();
        self.server = Some(server);
        self.stream = Some(cx.spawn(async move |this, cx| {
            while updates.changed().await.is_ok() {
                let state = updates.borrow_and_update().clone();
                if this
                    .update(cx, |this, cx| {
                        this.accept(state, cx);
                        cx.notify();
                    })
                    .is_err()
                {
                    return;
                }
            }
            let _ = this.update(cx, |this, cx| {
                this.message_views.clear();
                this.message_list.reset(0);
                this.older_pending = false;
                this.state = Arc::new(ServerState {
                    status: Status::Failed,
                    error: Some(Error::Realtime),
                    ..Default::default()
                });
                cx.notify();
            });
        }));
        cx.notify();
    }
    fn accept(&mut self, state: Arc<ServerState>, cx: &mut Context<Self>) {
        if state.status == Status::Failed
            && matches!(state.error, Some(Error::Http(400 | 401 | 403)))
        {
            self.clear_connection();
            self.auth_status = Some("Clearing the ended session…");
            if let Some(store) = self.store.clone() {
                self.request = Some(self.request(
                    async move { store.forget().await },
                    cx,
                    |this, result, _cx| {
                        this.auth_status = None;
                        match result {
                            Ok(()) => {
                                this.saved_session = false;
                                this.error =
                                    Some("Your saved session ended. Sign in again.".into());
                            }
                            Err(error) => this.error = Some(error.to_string()),
                        }
                    },
                ));
            }
            return;
        }
        let reset =
            state.generation != self.state.generation || state.selected != self.state.selected;
        if reset || self.state.timeline.messages.is_empty() {
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
    fn sign_out(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let Some(store) = self.store.clone() else {
            return;
        };
        window.blur();
        self.clear_connection();
        self.auth_status = Some("Signing out…");
        self.request = Some(self.request(
            async move {
                let result = store.sign_out().await;
                let retained = store.has_session().await;
                Ok((result, retained))
            },
            cx,
            |this, result, _cx| {
                this.auth_status = None;
                match result {
                    Ok((result, retained)) => {
                        this.saved_session = retained;
                        if result.is_err() {
                            this.error = Some(
                                if retained {
                                    "Could not clear the saved session. Retry sign-out."
                                } else {
                                    "Signed out locally. The server session could not be revoked."
                                }
                                .into(),
                            );
                        }
                    }
                    Err(error) => this.error = Some(error.to_string()),
                }
            },
        ));
        cx.notify();
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
        let state = self.state.clone();
        let directory = &state.directory;
        let Some(message) = state.timeline.messages.get(index) else {
            return div().into_any_element();
        };
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
                cx.new(|cx| message_text::MessageText::new(&source, message.id.clone(), cx))
            })
            .clone();
        let copy_body = message.body.clone().unwrap_or_default();
        let copy_id = message.id.clone();
        div()
            .id(SharedString::from(format!("message:{}", message.id)))
            .px_5()
            .py_3()
            .border_b_1()
            .border_color(rgb(0x242832))
            .child(
                div()
                    .flex()
                    .gap_3()
                    .mb_1()
                    .child(div().font_weight(gpui::FontWeight::SEMIBOLD).child(author))
                    .child(
                        div()
                            .text_sm()
                            .text_color(rgb(0xa1a1aa))
                            .child(message.created_at.clone()),
                    )
                    .context_menu(move |menu, _, _| {
                        let body = copy_body.clone();
                        let id = copy_id.clone();
                        menu.item(
                            PopupMenuItem::new("Copy message")
                                .disabled(body.is_empty())
                                .on_click(move |_, _, cx| {
                                    cx.write_to_clipboard(ClipboardItem::new_string(body.clone()))
                                }),
                        )
                        .item(
                            PopupMenuItem::new("Copy message ID").on_click(move |_, _, cx| {
                                cx.write_to_clipboard(ClipboardItem::new_string(id.clone()))
                            }),
                        )
                    }),
            )
            .track_focus(&text.focus_handle(cx))
            .child(text)
            .into_any_element()
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
            let username = cx.new(|cx| InputState::new(window, cx).placeholder("Username"));
            let password = cx.new(|cx| {
                InputState::new(window, cx)
                    .placeholder("Password")
                    .masked(true)
            });
            let submit = cx.subscribe_in(&password, window, |this, _, event, window, cx| {
                if matches!(event, InputEvent::PressEnter { .. }) {
                    this.connect(window, cx);
                }
            });
            self.login_form = Some(LoginForm {
                origin,
                username,
                password,
                _submit: submit,
            });
        }
        let form = self.login_form.as_ref().expect("login form initialized");
        div()
            .flex()
            .flex_col()
            .w_full()
            .max_w(px(460.))
            .gap_3()
            .child(div().text_3xl().mb_3().child("Sign in to Chatto"))
            .child("Server URL")
            .child(Input::new(&form.origin).tab_index(0))
            .child("Username")
            .child(Input::new(&form.username).tab_index(1))
            .child("Password")
            .child(Input::new(&form.password).tab_index(2))
            .child(
                Button::new("sign-in")
                    .primary()
                    .label("Sign in")
                    .tab_index(3)
                    .on_click(cx.listener(|this, _, window, cx| this.connect(window, cx))),
            )
            .child(div().text_sm().text_color(rgb(0xa1a1aa)).child(
                "Your password goes only to this server. Your session is saved in macOS Keychain.",
            ))
    }
}
fn button(id: &'static str, label: impl Into<SharedString>) -> Button {
    Button::new(id).ghost().label(label)
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
        let mut root = div()
            .size_full()
            .flex()
            .flex_col()
            .bg(rgb(0x17191f))
            .text_color(rgb(0xf4f4f5))
            .text_base();
        if let Some(error) = self
            .error
            .clone()
            .or_else(|| self.state.error.as_ref().map(ToString::to_string))
        {
            root = root.child(div().p_3().bg(rgb(0x572b35)).child(error));
        }
        if let Some(status) = self.auth_status {
            return root.justify_center().items_center().child(status);
        }
        if self.store.is_none() {
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
        if self.server.is_none() {
            if self.saved_session {
                return root
                    .justify_center()
                    .items_center()
                    .gap_4()
                    .child("Saved connection")
                    .child(
                        button("retry-saved", "Reconnect")
                            .on_click(cx.listener(|this, _, _, cx| this.start_connection(cx))),
                    )
                    .child(
                        button("forget-saved", "Sign out")
                            .on_click(cx.listener(|this, _, window, cx| this.sign_out(window, cx))),
                    );
            }
            return root
                .justify_center()
                .items_center()
                .p_8()
                .child(self.connection_form(window, cx));
        }
        let state = self.state.clone();
        let directory = &state.directory;
        let settings = self.store.as_ref().expect("storage opened").settings();
        let mut sidebar = div()
            .w(px(272.))
            .h_full()
            .flex_shrink_0()
            .flex()
            .flex_col()
            .bg(rgb(0x20242d))
            .border_r_1()
            .border_color(rgb(0x303541));
        sidebar = sidebar.child(
            div()
                .p_4()
                .border_b_1()
                .border_color(rgb(0x303541))
                .child(
                    div()
                        .text_lg()
                        .font_weight(gpui::FontWeight::SEMIBOLD)
                        .child(if directory.server.name.is_empty() {
                            "Chatto".into()
                        } else {
                            directory.server.name.clone()
                        }),
                )
                .child(
                    div()
                        .mt_1()
                        .text_sm()
                        .text_color(rgb(if state.status == Status::Connected {
                            0xa7cdb4
                        } else {
                            0xa1a1aa
                        }))
                        .child(status_label(state.status)),
                ),
        );
        let mut rows = div()
            .id("room-directory")
            .flex_1()
            .min_h_0()
            .overflow_y_scroll()
            .track_scroll(&self.room_scroll)
            .px_2()
            .py_3();
        for section in sidebar::sections(directory) {
            let id = section.id.clone();
            let collapsed = settings.collapsed.contains(&section.id);
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
                            .text_color(rgb(0xa1a1aa))
                            .child(if collapsed { "▸" } else { "▾" })
                            .child(format!("{}{suffix}", section.label)),
                    )
                    .on_click(cx.listener(move |this, _, _, cx| this.toggle(id.clone(), cx))),
            );
            if !collapsed {
                if section.items.is_empty() {
                    rows = rows.child(
                        div()
                            .px_3()
                            .py_2()
                            .text_sm()
                            .text_color(rgb(0x8b919e))
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
                                .child(div().w(px(8.)).text_color(rgb(0xa1a1aa)).child(if dm {
                                    "●"
                                } else {
                                    ""
                                }))
                                .child(
                                    div()
                                        .flex_1()
                                        .overflow_hidden()
                                        .font_weight(if unread {
                                            gpui::FontWeight::SEMIBOLD
                                        } else {
                                            gpui::FontWeight::NORMAL
                                        })
                                        .child(label),
                                );
                            if important > 0 {
                                row = row.child(
                                    div()
                                        .px_2()
                                        .rounded_md()
                                        .bg(rgb(0x986c99))
                                        .text_sm()
                                        .child(important.to_string()),
                                );
                            } else if unread {
                                row =
                                    row.child(div().size(px(6.)).rounded_full().bg(rgb(0xe4e4e7)));
                            }
                            rows = rows.child(
                                Button::new(SharedString::from(format!("room:{id}")))
                                    .ghost()
                                    .w_full()
                                    .h(px(36.))
                                    .justify_start()
                                    .selected(selected)
                                    .bg(rgb(if selected { 0x3c3955 } else { 0x20242d }))
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
                rows = rows.child(div().h(px(8.)));
            }
        }
        sidebar = sidebar.child(rows).child(
            div()
                .p_3()
                .border_t_1()
                .border_color(rgb(0x303541))
                .child(
                    div()
                        .mb_2()
                        .text_sm()
                        .child(if directory.viewer.id.is_empty() {
                            "Loading account…".into()
                        } else {
                            sidebar::profile_label(&directory.viewer)
                        }),
                )
                .child(
                    div()
                        .flex()
                        .gap_2()
                        .child(
                            button(
                                "retry-connection",
                                if state.status == Status::Failed {
                                    "Retry"
                                } else {
                                    "Refresh"
                                },
                            )
                            .on_click(cx.listener(|this, _, _, cx| {
                                if this.state.status == Status::Failed {
                                    this.start_connection(cx);
                                } else if let Some(server) = &this.server
                                    && let Err(error) = server.refresh()
                                {
                                    this.error = Some(error.to_string());
                                }
                                cx.notify();
                            })),
                        )
                        .child(button("sign-out", "Sign out").on_click(
                            cx.listener(|this, _, window, cx| this.sign_out(window, cx)),
                        )),
                ),
        );
        let mut content = div().flex_1().min_w_0().h_full().flex().flex_col();
        if let Some(room) = &state.selected {
            let label = directory
                .rooms
                .iter()
                .find(|entry| entry.room.id == *room)
                .map(|entry| sidebar::room_label(entry, directory))
                .unwrap_or_else(|| "Conversation".into());
            content = content.child(
                div()
                    .flex()
                    .items_center()
                    .gap_3()
                    .px_5()
                    .py_4()
                    .border_b_1()
                    .border_color(rgb(0x303541))
                    .child(div().flex_1().text_xl().child(label))
                    .child(
                        button("mark-read", "Mark read")
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
                    // GPUI 0.2.2 reports the previous painted range here and still
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
            content = content.child(
                div()
                    .px_5()
                    .py_1()
                    .text_sm()
                    .text_color(rgb(0xa1a1aa))
                    .child(if state.loading {
                        "Updating…"
                    } else if state.timeline.messages.len() >= 500 {
                        "History limit reached (500 messages)"
                    } else if state.timeline.has_older {
                        "Scroll up to load older messages"
                    } else {
                        "Beginning of loaded history"
                    }),
            );
            if state.timeline.messages.is_empty() && !state.loading {
                content = content.child(div().p_5().child("No messages in this page"));
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
                .child(div().text_xl().child(if state.status == Status::Connected {
                    "Select a conversation"
                } else {
                    status_label(state.status)
                }))
                .child(
                    div()
                        .text_color(rgb(0xa1a1aa))
                        .child("Your channels and direct messages appear in the sidebar."),
                );
        }
        root.child(
            div()
                .flex()
                .flex_1()
                .min_h_0()
                .child(sidebar)
                .child(content),
        )
    }
}
fn main() {
    let check_fonts = std::env::args().any(|arg| arg == "--check-fonts");
    let runtime = Arc::new(
        tokio::runtime::Builder::new_multi_thread()
            .worker_threads(2)
            .enable_all()
            .build()
            .expect("failed to start the network runtime"),
    );
    Application::new().run(move |cx: &mut App| {
        if check_fonts {
            // GPUI's dummy backend lists fallback names and gives every ASCII
            // glyph the same advance. The native proportional font must give
            // W more space than i. Exercise the backend selected by this binary.
            let text = cx.text_system();
            let font = text.resolve_font(&gpui::font(".SystemUIFont"));
            let wide = text.advance(font, px(16.), 'W');
            let narrow = text.advance(font, px(16.), 'i');
            if !matches!((wide, narrow), (Ok(wide), Ok(narrow)) if wide.width > narrow.width) {
                eprintln!("GPUI has no native font metrics; enable its font-kit feature");
                std::process::exit(1);
            }
            cx.quit();
            return;
        }
        gpui_component::init(cx);
        Theme::change(ThemeMode::Dark, None, cx);
        cx.on_action(|_: &Quit, cx| cx.quit());
        cx.bind_keys([
            KeyBinding::new("cmd-q", Quit, None),
            KeyBinding::new("cmd-c", message_text::Copy, Some("MessageText")),
            KeyBinding::new("cmd-a", message_text::SelectAll, Some("MessageText")),
        ]);
        cx.on_window_closed(|cx| {
            if cx.windows().is_empty() {
                cx.quit();
            }
        })
        .detach();
        let bounds = Bounds::centered(None, size(px(1100.0), px(740.0)), cx);
        cx.open_window(
            WindowOptions {
                titlebar: Some(gpui::TitlebarOptions {
                    title: Some("Chatto Native".into()),
                    ..Default::default()
                }),
                window_bounds: Some(WindowBounds::Windowed(bounds)),
                window_min_size: Some(size(px(720.0), px(480.0))),
                ..Default::default()
            },
            |window, cx| {
                let view = cx.new(|cx| {
                    let mut app = ChattoNative::new(runtime);
                    app.restore(cx);
                    app
                });
                cx.new(|cx| Root::new(view, window, cx))
            },
        )
        .expect("failed to open the Chatto Native window");
        cx.activate(true);
    });
}

#[cfg(test)]
mod timeline_ui_tests {
    use super::*;
    use gpui::{AppContext, ScrollDelta, ScrollWheelEvent, TestAppContext, point};

    #[gpui::test]
    fn scrolling_the_real_timeline_requests_history_once_and_keeps_rows_virtual(
        cx: &mut TestAppContext,
    ) {
        cx.update(gpui_component::init);
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
                |window, cx| cx.new(|cx| Root::new(view.clone(), window, cx)),
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
        cx.simulate_event(ScrollWheelEvent {
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
        cx.simulate_event(ScrollWheelEvent {
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
