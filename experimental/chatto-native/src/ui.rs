//! Chatto presentation tokens and shared native UI building blocks.
//! Fonts and icons are bundled; presentation does not make network requests.
use gpui::component::{Theme, ThemeMode};
use gpui::{App, Div, FontWeight, IntoElement, Styled, div, prelude::*, px, rgb};
use std::borrow::Cow;

pub const SHELL: u32 = 0x242424;
pub const SURFACE_EDGE: u32 = 0x383838;
pub const SURFACE_EMPHASIZED: u32 = 0x2c2c2c;
pub const BACKGROUND: u32 = 0x171717;
pub const SURFACE: u32 = 0x242424;
pub const BORDER: u32 = 0x2a2a2a;
pub const TEXT: u32 = 0xd1d1d1;
pub const TOP: u32 = 0xeeeeee;
pub const MUTED: u32 = 0x969696;
pub const ACTION: u32 = 0x00b8e5;
pub const SELECTED: u32 = 0x172d33;
pub const CODE: f32 = 14.;
pub const BODY: f32 = 16.;
pub const SECONDARY: f32 = 14.;
pub const CAPTION: f32 = 12.;
pub const HEADING: f32 = 16.;
pub const TITLE: f32 = 24.;
pub const BADGE: f32 = 10.;

/// All application dimensions use rems, so window zoom changes the whole UI.
pub fn unit(logical_pixels: f32) -> gpui::Rems {
    gpui::rems(logical_pixels / 16.)
}

/// Match the GPUix experiment's supported zoom range.
pub fn normalize_scale(value: f32) -> f32 {
    if value.is_finite() {
        (value.clamp(0.5, 2.) * 100.).round() / 100.
    } else {
        1.
    }
}

/// Kit controls and Base overlays share the same scaled metrics as app content.
pub fn scale_theme(scale: f32, cx: &mut App) {
    let theme = Theme::global_mut(cx);
    theme.font_size = px(BODY * scale);
    theme.mono_font_size = px(CODE * scale);
    theme.radius = px(RADIUS * scale);
    theme.radius_lg = px(RADIUS * 2. * scale);
    Theme::sync_base(cx);
}

pub const HEADER_HEIGHT: f32 = 56.;
pub const GUTTER_WIDTH: f32 = 64.;
pub const SIDEBAR_WIDTH: f32 = 256.;
pub const OUTER_INSET: f32 = 8.;
pub const PANE_RADIUS: f32 = 16.;
pub const MESSAGE_AVATAR: f32 = 44.;
pub const MESSAGE_GAP: f32 = 16.;
pub const RADIUS: f32 = 5.;

/// Initialize Kit before constructing controls and project the same theme to Base.
pub fn init(cx: &mut App) {
    gpui::init(cx);
    cx.text_system()
        .add_fonts(vec![
            Cow::Borrowed(include_bytes!("../assets/IBMPlexSans-Regular.ttf")),
            Cow::Borrowed(include_bytes!("../assets/IBMPlexSans-SemiBold.ttf")),
            Cow::Borrowed(include_bytes!("../assets/IBMPlexMono-Regular.ttf")),
        ])
        .expect("bundled IBM Plex fonts must be valid");
    Theme::change(ThemeMode::Dark, None, cx);
    let theme = Theme::global_mut(cx);
    theme.font_family = "IBM Plex Sans".into();
    theme.mono_font_family = "IBM Plex Mono".into();
    theme.font_size = px(BODY);
    theme.radius = px(5.);
    theme.radius_lg = px(10.);
    theme.colors.background = rgb(BACKGROUND).into();
    theme.colors.foreground = rgb(TEXT).into();
    theme.colors.border = rgb(BORDER).into();
    theme.colors.input = rgb(0x383838).into();
    theme.colors.primary = rgb(ACTION).into();
    theme.colors.primary_hover = rgb(0x20c8ef).into();
    theme.colors.primary_active = rgb(0x009bbf).into();
    theme.colors.primary_foreground = rgb(BACKGROUND).into();
    theme.colors.ring = rgb(ACTION).into();
    theme.colors.accent = rgb(SELECTED).into();
    theme.colors.accent_foreground = rgb(TOP).into();
    theme.colors.muted = rgb(SURFACE).into();
    theme.colors.muted_foreground = rgb(MUTED).into();
    Theme::sync_base(cx);
}

/// Every pane has the same fixed header height, insets, and title typography.
pub fn pane_header(title: impl IntoElement) -> Div {
    div()
        .h(unit(HEADER_HEIGHT))
        .flex_shrink_0()
        .w_full()
        .flex()
        .items_center()
        .gap_3()
        .px_4()
        .border_b_1()
        .border_color(rgb(BORDER))
        .bg(rgb(BACKGROUND))
        .child(
            div()
                .flex_shrink(1.)
                .min_w_0()
                .truncate()
                .text_size(unit(HEADING))
                .font_weight(FontWeight::SEMIBOLD)
                .text_color(rgb(TOP))
                .child(title),
        )
}

/// Initials provide a local, deterministic fallback for missing profile images.
pub fn avatar(label: &str, diameter: f32) -> Div {
    let initials: String = label
        .split_whitespace()
        .take(2)
        .filter_map(|word| word.chars().next())
        .flat_map(char::to_uppercase)
        .collect();
    div()
        .size(unit(diameter))
        .flex_shrink_0()
        .rounded(unit(9.))
        .flex()
        .items_center()
        .justify_center()
        .bg(rgb(SURFACE_EMPHASIZED))
        .text_color(rgb(TEXT))
        .text_size(unit(SECONDARY))
        .font_weight(FontWeight::SEMIBOLD)
        .child(if initials.is_empty() {
            "?".to_owned()
        } else {
            initials
        })
}

/// Show a compact local time while leaving the original timestamp in the model.
pub fn message_time(value: &str) -> String {
    chrono::DateTime::parse_from_rfc3339(value)
        .map(|date| {
            date.with_timezone(&chrono::Local)
                .format("%H:%M")
                .to_string()
        })
        .unwrap_or_default()
}

/// Quiet square action with a named tooltip and a consistent keyboard hitbox.
pub fn icon_button(
    id: &'static str,
    icon: gpui::assets::IconName,
    label: &'static str,
) -> gpui::component::button::Button {
    use gpui::component::button::ButtonVariants;
    gpui::component::button::Button::new(id)
        .ghost()
        .icon(icon)
        .tooltip(label)
        .w(unit(40.))
        .h(unit(40.))
        .text_color(rgb(MUTED))
        .rounded_lg()
}

/// Shared attachment, link-preview, and settings surface.
pub fn card() -> Div {
    div()
        .w_full()
        .flex()
        .flex_col()
        .gap_2()
        .p_3()
        .rounded(unit(RADIUS * 2.))
        .border_1()
        .border_color(rgb(BORDER))
        .bg(rgb(SURFACE))
}

/// Date boundaries stay in the virtual row so paging preserves one anchor.
pub fn date_divider(label: String) -> Div {
    div()
        .w_full()
        .flex()
        .items_center()
        .gap_3()
        .px_5()
        .py_3()
        .child(div().flex_1().h(px(1.)).bg(rgb(BORDER)))
        .child(
            div()
                .text_size(unit(CAPTION))
                .text_color(rgb(MUTED))
                .child(label),
        )
        .child(div().flex_1().h(px(1.)).bg(rgb(BORDER)))
}
pub fn message_day(value: &str) -> String {
    chrono::DateTime::parse_from_rfc3339(value)
        .map(|date| {
            date.with_timezone(&chrono::Local)
                .format("%B %-d, %Y")
                .to_string()
        })
        .unwrap_or_default()
}

/// Text actions share the icon-button hitbox and stay aligned to their content.
pub fn text_button(
    id: impl Into<gpui::ElementId>,
    label: impl Into<gpui::SharedString>,
) -> gpui::component::button::Button {
    use gpui::component::{Sizable, button::ButtonVariants};
    gpui::component::button::Button::new(id)
        .ghost()
        .small()
        .label(label)
        .h(unit(30.))
        .text_size(unit(BODY))
}

/// The web app's inset work plane, shared by all native routes and panels.
pub fn work_plane(content: impl IntoElement) -> Div {
    div()
        .size_full()
        .flex()
        .flex_col()
        .min_h_0()
        .min_w_0()
        .rounded(unit(PANE_RADIUS))
        .border_1()
        .border_color(rgb(BORDER))
        .bg(rgb(BACKGROUND))
        .overflow_hidden()
        .child(content)
}
/// A framed media edge; images keep their own size instead of becoming cards.
pub fn inset_frame() -> Div {
    div()
        .self_start()
        .max_w_full()
        .min_w_0()
        .p_1()
        .rounded(unit(8.))
        .border_1()
        .border_color(rgb(SURFACE_EDGE))
        .bg(rgb(SURFACE))
}
/// Navigation and room rows share one hit area, text size, and corner radius.
pub fn navigation_row(
    id: impl Into<gpui::ElementId>,
    label: impl Into<gpui::SharedString>,
    icon: gpui::assets::IconName,
) -> gpui::component::button::Button {
    use gpui::component::button::ButtonVariants;
    gpui::component::button::Button::new(id)
        .ghost()
        .child(
            div()
                .w_full()
                .flex()
                .items_center()
                .gap_3()
                .child(gpui::component::Icon::new(icon).size(unit(20.)))
                .child(div().text_size(unit(BODY)).child(label.into())),
        )
        .w_full()
        .h(unit(36.))
        .justify_start()
        .text_size(unit(BODY))
        .rounded_md()
}
