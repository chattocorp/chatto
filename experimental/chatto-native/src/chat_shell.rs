//! Native versions of the web app's shell, server navigation, and account card.
use super::*;
use gpui::assets::IconName;
use gpui::component::{Icon, TitleBar, menu::DropdownMenu};

impl ChattoNative {
    pub(super) fn app_titlebar(&self, cx: &mut Context<Self>) -> impl IntoElement + use<> {
        let name = if self.state.directory.server.name.is_empty() {
            "Chatto Native".to_owned()
        } else {
            self.state.directory.server.name.clone()
        };
        let origin = self.origin.clone();
        let title = div()
            .flex_1()
            .min_w_0()
            .flex()
            .items_center()
            .justify_center()
            .gap_1()
            .text_size(px(12.))
            .text_color(rgb(ui::MUTED))
            .child(name)
            .child(div().child("·"))
            .child(
                Button::new("open-server-browser")
                    .ghost()
                    .h(px(28.))
                    .p_0()
                    .text_size(px(12.))
                    .text_color(rgb(ui::ACTION))
                    .label(origin.clone())
                    .disabled(self.preview)
                    .on_click(move |_, _, cx| {
                        if let Some(url) = media::media_url(&origin, &origin) {
                            cx.open_url(url.as_str());
                        }
                    }),
            );
        TitleBar::new()
            .bg(rgb(ui::SHELL))
            .border_color(rgb(ui::SHELL))
            .child(
                div()
                    .flex()
                    .items_center()
                    .gap_1()
                    .child(
                        chrome_button(
                            "toggle-server-sidebar",
                            IconName::Menu,
                            "Toggle server sidebar",
                        )
                        .on_click(cx.listener(|this, _, _, cx| {
                            this.sidebar_visible = !this.sidebar_visible;
                            cx.notify();
                        })),
                    )
                    .child(
                        chrome_button(
                            "toolbar-search",
                            IconName::Search,
                            "Find a conversation (Command-K)",
                        )
                        .on_click(cx.listener(|this, _, window, cx| this.focus_search(window, cx))),
                    )
                    .child(
                        chrome_button(
                            "toolbar-settings",
                            IconName::Settings,
                            "Settings and servers",
                        )
                        .on_click(cx.listener(|this, _, _, cx| {
                            this.cancel_sign_in(cx);
                            this.managing = !this.managing;
                            this.adding = false;
                            cx.notify();
                        })),
                    ),
            )
            .child(title)
            .child(
                chrome_button("toolbar-overview", IconName::Info, "Server overview").on_click(
                    cx.listener(|this, _, _, cx| {
                        this.overview = !this.overview;
                        cx.notify();
                    }),
                ),
            )
    }
    pub(super) fn focus_search(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.sidebar_visible = true;
        self.search_visible = true;
        self.overview = false;
        // The input can be unmounted while the navigation row is shown.
        if let Some(search) = &self.room_search {
            search.focus_handle(cx).focus(window, cx);
        }
        cx.notify();
    }
    pub(super) fn sidebar_navigation(&self, cx: &mut Context<Self>) -> impl IntoElement + use<> {
        div()
            .w_full()
            .flex()
            .flex_col()
            .gap_1()
            .px_2()
            .py_2()
            .border_b_1()
            .border_color(rgb(ui::BORDER))
            .child(
                ui::navigation_row("server-overview", "Overview", IconName::House)
                    .selected(self.overview)
                    .when(self.overview, |button| {
                        button.bg(rgb(ui::SELECTED)).text_color(rgb(ui::ACTION))
                    })
                    .on_click(cx.listener(|this, _, _, cx| {
                        this.overview = true;
                        cx.notify();
                    })),
            )
            .child(
                ui::navigation_row("server-search", "Search", IconName::Search)
                    .on_click(cx.listener(|this, _, window, cx| this.focus_search(window, cx))),
            )
    }
    pub(super) fn sidebar_artwork(&self, cx: &App) -> gpui::AnyElement {
        if self.preview {
            return div()
                .px_2()
                .pt_2()
                .child(
                    ui::inset_frame().w_full().bg(rgb(0x9072b2)).child(
                        div()
                            .w_full()
                            .aspect_ratio(1.8)
                            .flex()
                            .items_center()
                            .justify_center()
                            .bg(rgb(0xb89acb))
                            .child(
                                gpui::img(media::bundled_logo())
                                    .size(ui::unit(112.))
                                    .object_fit(gpui::ObjectFit::Contain),
                            ),
                    ),
                )
                .into_any_element();
        }
        if let Some(connection) = self
            .active_server
            .as_ref()
            .and_then(|id| self.connections.get(id))
            && let Some(raw) = self.state.directory.server.banner_url.as_deref()
        {
            return div()
                .px_2()
                .pt_2()
                .child(media::server_banner(&connection.public_media, raw, cx))
                .into_any_element();
        }
        div().into_any_element()
    }
    pub(super) fn account_card(&self, cx: &mut Context<Self>) -> impl IntoElement + use<> {
        let viewer = &self.state.directory.viewer;
        let name = sidebar::profile_label(viewer);
        let portrait = if let Some(cache) = &self.media {
            media::avatar(
                cache,
                &name,
                viewer.avatar_url.as_deref(),
                format!("avatar:{}", viewer.id),
                32.,
                false,
                cx,
            )
        } else {
            ui::avatar(&name, 32.).rounded_full().into_any_element()
        };
        let subtitle = if self.state.status == Status::Connected {
            if viewer.login.is_empty() {
                "".into()
            } else {
                format!("@{}", viewer.login)
            }
        } else {
            status_label(self.state.status).into()
        };
        div().p_2().child(
            div()
                .flex()
                .items_center()
                .gap_2()
                .w_full()
                .h(ui::unit(48.))
                .px_2()
                .rounded(ui::unit(10.))
                .border_1()
                .border_color(rgb(ui::SURFACE_EDGE))
                .bg(rgb(ui::SURFACE))
                .child(portrait)
                .child(
                    div()
                        .flex_1()
                        .min_w_0()
                        .child(
                            div()
                                .truncate()
                                .text_size(ui::unit(ui::SECONDARY))
                                .font_weight(gpui::FontWeight::SEMIBOLD)
                                .child(name),
                        )
                        .child(
                            div()
                                .truncate()
                                .text_size(ui::unit(ui::CAPTION))
                                .text_color(rgb(ui::MUTED))
                                .child(subtitle),
                        ),
                )
                .child(
                    ui::icon_button(
                        "account-actions",
                        IconName::EllipsisVertical,
                        "Account and connection",
                    )
                    .dropdown_menu({
                        let view = cx.entity().downgrade();
                        move |menu, _, _| {
                            let refresh = view.clone();
                            let settings = view.clone();
                            let signout = view.clone();
                            menu.item(PopupMenuItem::new("Refresh connection").on_click(
                                move |_, _, cx| {
                                    let _ =
                                        refresh.update(cx, |this, cx| this.start_connection(cx));
                                },
                            ))
                            .item(PopupMenuItem::new("Settings and servers").on_click(
                                move |_, _, cx| {
                                    let _ = settings.update(cx, |this, cx| {
                                        this.managing = true;
                                        cx.notify();
                                    });
                                },
                            ))
                            .separator()
                            .item(
                                PopupMenuItem::new("Sign out and remove server").on_click(
                                    move |_, window, cx| {
                                        let _ = signout
                                            .update(cx, |this, cx| this.sign_out(window, cx));
                                    },
                                ),
                            )
                        }
                    }),
                ),
        )
    }
    pub(super) fn server_overview(&self, cx: &mut Context<Self>) -> impl IntoElement + use<> {
        div()
            .flex_1()
            .min_w_0()
            .h_full()
            .flex()
            .flex_col()
            .child(ui::pane_header("Overview"))
            .child(
                div()
                    .id("overview-content")
                    .flex_1()
                    .min_h_0()
                    .overflow_y_scroll()
                    .p_6()
                    .child(
                        div()
                            .max_w(ui::unit(640.))
                            .flex()
                            .flex_col()
                            .gap_4()
                            .child(
                                div()
                                    .text_size(ui::unit(ui::TITLE))
                                    .font_weight(gpui::FontWeight::SEMIBOLD)
                                    .child(self.state.directory.server.name.clone()),
                            )
                            .child(self.sidebar_artwork(cx))
                            .child(
                                self.state
                                    .directory
                                    .server
                                    .description
                                    .clone()
                                    .unwrap_or_default(),
                            )
                            .child(format!(
                                "{} conversations",
                                self.state.directory.rooms.len()
                            )),
                    ),
            )
    }
}
fn chrome_button(id: &'static str, icon: IconName, label: &'static str) -> Button {
    Button::new(id)
        .ghost()
        .icon(Icon::new(icon).size(px(16.)))
        .tooltip(label)
        .size(px(28.))
        .p_0()
        .text_color(rgb(ui::MUTED))
}

impl ChattoNative {
    /// This is the authorized server directory, not an inferred room membership list.
    pub(super) fn people_panel(
        &mut self,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> impl IntoElement + use<> {
        if self.people_search.is_none() {
            let search = cx.new(|cx| InputState::new(window, cx).placeholder("Search people"));
            self.people_subscription = Some(cx.subscribe(&search, |_, _, event, cx| {
                if matches!(event, InputEvent::Change) {
                    cx.notify();
                }
            }));
            self.people_search = Some(search);
        }
        let search = self.people_search.as_ref().unwrap();
        let query = search.read(cx).value().to_lowercase();
        let mut profiles = self
            .state
            .directory
            .users
            .values()
            .cloned()
            .collect::<Vec<_>>();
        if !profiles
            .iter()
            .any(|profile| profile.id == self.state.directory.viewer.id)
        {
            profiles.push(self.state.directory.viewer.clone());
        }
        profiles.sort_by_key(|profile| sidebar::profile_label(profile).to_lowercase());
        let count = profiles.len();
        let mut rows = div()
            .id("server-people")
            .flex_1()
            .min_h_0()
            .overflow_y_scroll()
            .p_3()
            .flex()
            .flex_col()
            .gap_2();
        for profile in profiles {
            let label = sidebar::profile_label(&profile);
            if !query.is_empty()
                && !label.to_lowercase().contains(&query)
                && !profile.login.to_lowercase().contains(&query)
            {
                continue;
            }
            let portrait = if let Some(media) = &self.media {
                media::avatar(
                    media,
                    &label,
                    profile.avatar_url.as_deref(),
                    format!("avatar:{}", profile.id),
                    32.,
                    false,
                    cx,
                )
            } else {
                ui::avatar(&label, 32.).rounded_full().into_any_element()
            };
            rows = rows.child(
                div()
                    .flex()
                    .items_center()
                    .debug_selector(move || format!("person-{}", profile.id))
                    .min_h(ui::unit(44.))
                    .gap_2()
                    .child(portrait)
                    .child(
                        div()
                            .flex_1()
                            .min_w_0()
                            .child(
                                div()
                                    .truncate()
                                    .font_weight(gpui::FontWeight::SEMIBOLD)
                                    .child(label),
                            )
                            .child(
                                div()
                                    .truncate()
                                    .text_size(ui::unit(ui::SECONDARY))
                                    .text_color(rgb(ui::MUTED))
                                    .child(format!("@{}", profile.login)),
                            ),
                    ),
            );
        }
        let compact = window.viewport_size().width < px(1120. * self.scale);
        div()
            .h_full()
            .flex()
            .flex_col()
            .min_w_0()
            .border_l_1()
            .border_color(rgb(ui::BORDER))
            .when(compact, |view| view.flex_1())
            .when(!compact, |view| view.w(ui::unit(280.)).flex_shrink_0())
            .child(
                ui::pane_header(format!("People ({count})"))
                    .debug_selector(|| "people-header".into())
                    .child(div().flex_1())
                    .child(
                        ui::icon_button("close-people", IconName::X, "Close people").on_click(
                            cx.listener(|this, _, _, cx| {
                                this.people_visible = false;
                                cx.notify();
                            }),
                        ),
                    ),
            )
            .child(rows)
            .child(div().p_2().child(Input::new(search)))
    }
}
