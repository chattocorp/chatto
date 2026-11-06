//! Kit rich text with host-owned image transport, safe links, and copy actions.
use crate::{
    media::{MediaCache, MediaItem, media_url},
    ui,
};
use gpui::component::{
    menu::{ContextMenuExt, PopupMenuItem},
    text::{SelectionFormat, TextView, TextViewState, TextViewStyle},
};
use gpui::{
    App, ClipboardItem, Context, Entity, FocusHandle, Focusable, IntoElement, Render, Subscription,
    Window, actions, div, prelude::*, rgb,
};
use pulldown_cmark::{Event, Options, Parser, Tag};

actions!(message_text, [Copy, SelectAll]);

/// Treat embedded HTML as literal message content, including HTML image tags.
fn safe_markdown(source: &str) -> String {
    let mut result = String::new();
    let mut cursor = 0;
    for (event, range) in Parser::new_ext(source, Options::all()).into_offset_iter() {
        if matches!(event, Event::Html(_) | Event::InlineHtml(_)) && range.start >= cursor {
            result.push_str(&source[cursor..range.start]);
            result.push_str(
                &source[range.clone()]
                    .replace('&', "&amp;")
                    .replace('<', "&lt;")
                    .replace('>', "&gt;"),
            );
            cursor = range.end;
        }
    }
    result.push_str(&source[cursor..]);
    result
}
fn inline_images(source: &str) -> Vec<MediaItem> {
    Parser::new_ext(source, Options::all())
        .filter_map(|event| {
            if let Event::Start(Tag::Image { dest_url, .. }) = event {
                Some(MediaItem::inline(&dest_url))
            } else {
                None
            }
        })
        .take(8)
        .collect()
}

/// Each virtual row retains its parsed document and selection until it leaves
/// the row cache. It never creates a second scroll viewport inside the timeline.
pub struct MessageText {
    state: Entity<TextViewState>,
    source: String,
    id: String,
    images: Vec<MediaItem>,
    media: Option<Entity<MediaCache>>,
    _media_subscription: Option<Subscription>,
}
impl MessageText {
    pub fn new(source: &str, id: String, cx: &mut Context<Self>) -> Self {
        let safe = safe_markdown(source);
        let state = cx
            .new(|cx| TextViewState::markdown(&safe, cx).selection_format(SelectionFormat::Plain));
        Self {
            state,
            source: source.into(),
            id,
            images: inline_images(&safe),
            media: None,
            _media_subscription: None,
        }
    }
    pub fn set_media(&mut self, media: Entity<MediaCache>, cx: &mut Context<Self>) {
        self._media_subscription = Some(cx.observe(&media, |_, _, cx| cx.notify()));
        self.media = Some(media);
    }
    fn copy(&mut self, _: &Copy, _: &mut Window, cx: &mut Context<Self>) {
        let text = self.state.read(cx).selected_text();
        if !text.is_empty() {
            cx.write_to_clipboard(ClipboardItem::new_string(text));
        }
    }
    fn select_all(&mut self, _: &SelectAll, _: &mut Window, cx: &mut Context<Self>) {
        self.state.update(cx, |state, cx| state.select_all(cx));
        cx.notify();
    }
}
impl Focusable for MessageText {
    fn focus_handle(&self, cx: &App) -> FocusHandle {
        self.state.read(cx).focus_handle().clone()
    }
}
impl Render for MessageText {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let media = self.media.clone();
        let images = self.images.clone();
        let origin = self
            .media
            .as_ref()
            .map(|media| media.read(cx).origin().to_owned())
            .unwrap_or_default();
        let mut body = div()
            .id("message-text")
            .w_full()
            .min_w_0()
            .flex()
            .flex_col()
            .gap_2()
            .text_size(ui::unit(ui::BODY))
            .track_focus(&self.focus_handle(cx))
            .key_context("MessageText")
            .on_action(cx.listener(Self::copy))
            .on_action(cx.listener(Self::select_all))
            .child(
                TextView::new(&self.state)
                    .selectable(true)
                    .scrollable(false)
                    .w_full()
                    .text_size(ui::unit(ui::BODY))
                    .style(
                        TextViewStyle::default()
                            .paragraph_gap(ui::unit(8.))
                            .code_block(
                                gpui::StyleRefinement::default()
                                    .bg(rgb(ui::SURFACE))
                                    .p_3()
                                    .font_family("IBM Plex Mono")
                                    .text_size(ui::unit(ui::CODE)),
                            ),
                    )
                    .on_link_click(move |raw, _, _, cx| {
                        if let Some(url) = media_url(raw, &origin) {
                            cx.open_url(url.as_str());
                        }
                    })
                    .image_source(move |raw| {
                        if let Some(cache) = &media
                            && let Some(item) = images.iter().find(|item| item.url == raw.as_ref())
                        {
                            return MediaCache::source(cache, item.clone());
                        }
                        // This is authoritative for all image types, including data,
                        // file, and HTML sources. GPUI must never fetch a fallback.
                        gpui::ImageSource::Custom(std::sync::Arc::new(|_, _| None))
                    }),
            );
        if let Some(media) = &self.media {
            for item in &self.images {
                if media.read(cx).requires_consent(item) || media.read(cx).failed(&item.key) {
                    body = body.child(crate::media::image_view(media, item.clone(), cx));
                }
            }
        }
        let state = self.state.clone();
        let source = self.source.clone();
        let id = self.id.clone();
        body.context_menu(move |menu, _, cx| {
            let source = source.clone();
            let id = id.clone();
            menu.action_context(state.read(cx).focus_handle().clone())
                .menu_with_enable(
                    "Copy",
                    Box::new(Copy),
                    !state.read(cx).selected_text().is_empty(),
                )
                .menu("Select All", Box::new(SelectAll))
                .separator()
                .item(
                    PopupMenuItem::new("Copy message").on_click(move |_, _, cx| {
                        cx.write_to_clipboard(ClipboardItem::new_string(source.clone()))
                    }),
                )
                .item(
                    PopupMenuItem::new("Copy message ID").on_click(move |_, _, cx| {
                        cx.write_to_clipboard(ClipboardItem::new_string(id.clone()))
                    }),
                )
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn html_is_literal_and_only_bounded_markdown_images_are_candidates() {
        let safe = safe_markdown(
            "<img src=\"file:///private\">\n\n**Hello** ![alt](https://example.test/image.png)",
        );
        assert!(safe.contains("&lt;img"));
        let images = inline_images(&safe);
        assert_eq!(images.len(), 1);
        assert_eq!(images[0].url, "https://example.test/image.png");
        assert_eq!(
            inline_images(&"![a](https://example.test/image.png)\n".repeat(100)).len(),
            8
        );
    }
    #[gpui::test]
    fn rich_text_selection_copies_rendered_markdown_and_keeps_original_source(
        cx: &mut gpui::TestAppContext,
    ) {
        cx.update(gpui::init);
        let view =
            cx.new(|cx| MessageText::new("**Hello** *world* and `code`", "message".into(), cx));
        let (root, cx) =
            cx.add_window_view(|window, cx| gpui::component::Root::new(view.clone(), window, cx));
        view.update(cx, |view, cx| {
            view.state.update(cx, |state, cx| state.select_all(cx));
        });
        cx.update(|window, cx| view.update(cx, |view, cx| view.copy(&Copy, window, cx)));
        cx.update(|_, cx| {
            assert_eq!(
                cx.read_from_clipboard().unwrap().text().unwrap(),
                "Hello world and code\n"
            )
        });
        view.read_with(cx, |view, _| {
            assert_eq!(view.source, "**Hello** *world* and `code`")
        });
        drop(root);
    }
}
