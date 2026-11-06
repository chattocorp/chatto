//! Server rail and settings presentation. Session and persistence rules stay in the host.
use super::*;

impl ChattoNative {
    pub(super) fn server_gutter(&self, cx: &mut Context<Self>) -> impl IntoElement {
        let registrations = self
            .registry
            .as_ref()
            .map(|registry| registry.catalogue().servers)
            .unwrap_or_else(|| {
                if self.preview {
                    vec![
                        registry::Registration {
                            id: "preview".into(),
                            origin: "https://preview.example".into(),
                            name: "Chatto workspace".into(),
                        },
                        registry::Registration {
                            id: "preview-two".into(),
                            origin: "https://design.example".into(),
                            name: "Design lab".into(),
                        },
                    ]
                } else {
                    vec![]
                }
            });
        let disabled = self.mutation_busy() || self.registry.is_none();
        let mut servers = div()
            .id("saved-servers")
            .flex_1()
            .min_h_0()
            .overflow_y_scroll()
            .flex()
            .flex_col()
            .items_center()
            .gap_2();
        for (index, registration) in registrations.into_iter().enumerate() {
            let selected = !self.adding
                && !self.managing
                && self.active_server.as_ref() == Some(&registration.id)
                || self.preview && index == 0;
            let unread = self
                .connections
                .get(&registration.id)
                .map(|connection| {
                    connection
                        .state
                        .directory
                        .notifications
                        .values()
                        .map(|count| count.important_unread_count)
                        .sum::<u32>()
                })
                .unwrap_or(0);
            let needs_login = self
                .connections
                .get(&registration.id)
                .is_some_and(|connection| {
                    connection.state.status == Status::Failed
                        && matches!(connection.state.error, Some(Error::Http(400 | 401 | 403)))
                });
            let portrait = if let Some(connection) = self.connections.get(&registration.id) {
                media::avatar(
                    &connection.public_media,
                    &registration.name,
                    connection.state.directory.server.logo_url.as_deref(),
                    "server-logo".into(),
                    44.,
                    true,
                    cx,
                )
            } else {
                if index == 0 && self.preview {
                    gpui::img(media::bundled_logo())
                        .size(ui::unit(48.))
                        .rounded(ui::unit(12.))
                        .into_any_element()
                } else {
                    ui::avatar(&registration.name, 44.).into_any_element()
                }
            };
            let id = registration.id;
            let mut tile = div()
                .relative()
                .size(ui::unit(48.))
                .flex()
                .items_center()
                .justify_center()
                .rounded(ui::unit(14.))
                .border_2()
                .border_color(rgb(if selected { ui::TEXT } else { ui::SURFACE_EDGE }))
                .bg(rgb(if selected {
                    ui::SURFACE_EMPHASIZED
                } else {
                    ui::SURFACE
                }))
                .child(portrait);
            if needs_login || unread > 0 {
                tile = tile.child(
                    div()
                        .absolute()
                        .right_0()
                        .bottom_0()
                        .px_1()
                        .rounded(ui::unit(7.))
                        .bg(rgb(if needs_login { 0xefabab } else { ui::ACTION }))
                        .text_color(rgb(ui::BACKGROUND))
                        .text_size(ui::unit(ui::BADGE))
                        .child(if needs_login {
                            "!".into()
                        } else if unread > 99 {
                            "99+".into()
                        } else {
                            unread.to_string()
                        }),
                );
            }
            servers = servers.child(
                Button::new(SharedString::from(format!("server:{id}")))
                    .ghost()
                    .size(ui::unit(48.))
                    .p_0()
                    .tooltip(format!("{} — {}", registration.name, registration.origin))
                    .disabled(disabled)
                    .child(tile)
                    .on_click(
                        cx.listener(move |this, _, _, cx| this.switch_server(id.clone(), cx)),
                    ),
            );
        }
        div()
            .w(ui::unit(ui::GUTTER_WIDTH))
            .h_full()
            .flex_shrink_0()
            .flex()
            .flex_col()
            .items_center()
            .gap_2()
            .py_2()
            .border_r_1()
            .border_color(rgb(ui::BORDER))
            .bg(rgb(ui::BACKGROUND))
            .child(servers)
            .child(
                ui::icon_button("add-server", gpui::assets::IconName::Plus, "Add server")
                    .size(ui::unit(48.))
                    .bg(rgb(ui::SURFACE))
                    .disabled(disabled)
                    .on_click(cx.listener(|this, _, _, cx| this.begin_add(cx))),
            )
    }

    pub(super) fn server_settings(&mut self, cx: &mut Context<Self>) -> impl IntoElement {
        let catalogue = self
            .registry
            .as_ref()
            .map(|registry| registry.catalogue())
            .unwrap_or_default();
        let mut rows = div()
            .flex()
            .flex_col()
            .w_full()
            .max_w(ui::unit(600.))
            .gap_3();
        rows = rows.child(
            ui::card()
                .child(
                    div()
                        .font_weight(gpui::FontWeight::SEMIBOLD)
                        .child("UI scale"),
                )
                .child(
                    div()
                        .flex()
                        .items_center()
                        .gap_2()
                        .child(button("scale-down", "Smaller").on_click(cx.listener(
                            |this, _, window, cx| this.change_scale(this.scale - 0.1, window, cx),
                        )))
                        .child(format!("{}%", (self.scale * 100.).round() as u32))
                        .child(button("scale-up", "Larger").on_click(cx.listener(
                            |this, _, window, cx| this.change_scale(this.scale + 0.1, window, cx),
                        )))
                        .child(button("scale-reset", "Reset").on_click(
                            cx.listener(|this, _, window, cx| this.change_scale(1., window, cx)),
                        )),
                ),
        );
        for (index, server) in catalogue.servers.into_iter().enumerate() {
            let id = server.id;
            let up = id.clone();
            let down = id.clone();
            let remove = id.clone();
            rows = rows.child(
                ui::card()
                    .child(
                        div()
                            .flex()
                            .gap_3()
                            .items_center()
                            .child(ui::avatar(&server.name, 36.))
                            .child(
                                div()
                                    .flex_1()
                                    .min_w_0()
                                    .child(
                                        div()
                                            .font_weight(gpui::FontWeight::SEMIBOLD)
                                            .child(server.name),
                                    )
                                    .child(
                                        div()
                                            .text_size(ui::unit(ui::CAPTION))
                                            .text_color(rgb(ui::MUTED))
                                            .child(server.origin),
                                    ),
                            ),
                    )
                    .child(
                        div()
                            .flex()
                            .gap_2()
                            .child(
                                Button::new(SharedString::from(format!("open:{id}")))
                                    .ghost()
                                    .label("Open")
                                    .on_click(cx.listener(move |this, _, _, cx| {
                                        this.switch_server(id.clone(), cx)
                                    })),
                            )
                            .child(
                                Button::new(SharedString::from(format!("up:{up}")))
                                    .ghost()
                                    .label("Move up")
                                    .disabled(index == 0)
                                    .on_click(cx.listener(move |this, _, _, cx| {
                                        if let Some(registry) = &this.registry
                                            && let Err(error) = registry.move_server(&up, -1)
                                        {
                                            this.error = Some(error.to_string());
                                        }
                                        cx.notify();
                                    })),
                            )
                            .child(
                                Button::new(SharedString::from(format!("down:{down}")))
                                    .ghost()
                                    .label("Move down")
                                    .on_click(cx.listener(move |this, _, _, cx| {
                                        if let Some(registry) = &this.registry
                                            && let Err(error) = registry.move_server(&down, 1)
                                        {
                                            this.error = Some(error.to_string());
                                        }
                                        cx.notify();
                                    })),
                            )
                            .child(
                                Button::new(SharedString::from(format!("remove:{remove}")))
                                    .ghost()
                                    .label("Sign out and remove")
                                    .on_click(cx.listener(move |this, _, _, cx| {
                                        this.remove_server(remove.clone(), cx)
                                    })),
                            ),
                    ),
            );
        }
        div()
            .flex()
            .flex_col()
            .flex_1()
            .min_w_0()
            .h_full()
            .child(
                ui::pane_header("Settings and servers").child(
                    ui::icon_button(
                        "close-settings",
                        gpui::assets::IconName::X,
                        "Close settings",
                    )
                    .on_click(cx.listener(|this, _, _, cx| {
                        this.managing = false;
                        if let Some(id) = this.active_server.clone() {
                            this.switch_server(id, cx);
                        } else {
                            this.adding = true;
                        }
                        cx.notify();
                    })),
                ),
            )
            .child(
                div()
                    .id("server-settings-scroll")
                    .flex_1()
                    .min_h_0()
                    .overflow_y_scroll()
                    .p_6()
                    .child(rows),
            )
    }
}
