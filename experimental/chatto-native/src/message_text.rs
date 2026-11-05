//! Synchronous, text-only Markdown projection and lightweight selection state.
//! Message content never creates image requests or executes embedded HTML.
use gpui::{
    App, ClipboardItem, Context, FocusHandle, Focusable, FontStyle, FontWeight, HighlightStyle,
    MouseButton, Render, SharedString, StyledText, Window, actions, combine_highlights, div,
    prelude::*, rgb,
};
use gpui_component::menu::{ContextMenuExt, PopupMenuItem};
use pulldown_cmark::{Event, Options, Parser, Tag, TagEnd};
use std::ops::Range;

actions!(message_text, [Copy, SelectAll]);

#[derive(Default)]
struct Document {
    text: String,
    highlights: Vec<(Range<usize>, HighlightStyle)>,
}
impl Document {
    fn parse(source: &str) -> Self {
        let mut doc = Self::default();
        let (mut strong, mut emphasis, mut code, mut link, mut strike) =
            (0u32, 0u32, 0u32, 0u32, 0u32);
        let mut lists: Vec<Option<u64>> = Vec::new();
        let mut image = 0;
        for event in Parser::new_ext(
            source,
            Options::ENABLE_STRIKETHROUGH | Options::ENABLE_TASKLISTS,
        ) {
            let inline_code = matches!(&event, Event::Code(_));
            match event {
                Event::Start(tag) => match tag {
                    Tag::Strong | Tag::Heading { .. } => strong += 1,
                    Tag::Emphasis => emphasis += 1,
                    Tag::Strikethrough => strike += 1,
                    Tag::CodeBlock(_) => {
                        doc.break_line();
                        code += 1;
                    }
                    Tag::Link { .. } => link += 1,
                    Tag::Image { .. } => {
                        image += 1;
                        doc.text.push_str("[Image: ");
                    }
                    Tag::List(start) => {
                        doc.break_line();
                        lists.push(start);
                    }
                    Tag::Item => {
                        doc.break_line();
                        doc.text
                            .push_str(&"  ".repeat(lists.len().saturating_sub(1)));
                        match lists.last_mut() {
                            Some(Some(number)) => {
                                doc.text.push_str(&format!("{number}. "));
                                *number += 1;
                            }
                            _ => doc.text.push_str("• "),
                        }
                    }
                    Tag::BlockQuote(_) => {
                        doc.break_line();
                        doc.text.push_str("│ ");
                    }
                    _ => {}
                },
                Event::End(tag) => match tag {
                    TagEnd::Strong | TagEnd::Heading(_) => {
                        strong = strong.saturating_sub(1);
                        if matches!(tag, TagEnd::Heading(_)) {
                            doc.break_line();
                        }
                    }
                    TagEnd::Emphasis => emphasis = emphasis.saturating_sub(1),
                    TagEnd::Strikethrough => strike = strike.saturating_sub(1),
                    TagEnd::CodeBlock => {
                        code = code.saturating_sub(1);
                        doc.break_line();
                    }
                    TagEnd::Link => link = link.saturating_sub(1),
                    TagEnd::Image => {
                        image -= 1;
                        doc.text.push(']');
                    }
                    TagEnd::List(_) => {
                        lists.pop();
                        doc.break_line();
                    }
                    TagEnd::Paragraph | TagEnd::Item | TagEnd::BlockQuote(_) => doc.break_line(),
                    _ => {}
                },
                Event::Text(text)
                | Event::Code(text)
                | Event::Html(text)
                | Event::InlineHtml(text) => {
                    let start = doc.text.len();
                    doc.text.push_str(&text);
                    let mut style = HighlightStyle::default();
                    if strong > 0 {
                        style.font_weight = Some(FontWeight::BOLD);
                    }
                    if emphasis > 0 {
                        style.font_style = Some(FontStyle::Italic);
                    }
                    if strike > 0 {
                        style.fade_out = Some(0.5);
                    }
                    if code > 0 || inline_code {
                        style.color = Some(rgb(0xe5c78d).into());
                        style.background_color = Some(rgb(0x292d37).into());
                    }
                    if link > 0 && image == 0 {
                        style.color = Some(rgb(0x9bbff0).into());
                    }
                    if start < doc.text.len() {
                        doc.highlights.push((start..doc.text.len(), style));
                    }
                }
                Event::SoftBreak | Event::HardBreak => doc.text.push('\n'),
                Event::Rule => {
                    doc.break_line();
                    doc.text.push_str("────────────\n");
                }
                Event::TaskListMarker(checked) => {
                    doc.text.push_str(if checked { "☑ " } else { "☐ " })
                }
                _ => {}
            }
        }
        // Keep code whitespace and all byte ranges intact; only remove block separators.
        while doc.text.ends_with('\n') {
            doc.text.pop();
        }
        let len = doc.text.len();
        doc.highlights
            .iter_mut()
            .for_each(|(range, _)| range.end = range.end.min(len));
        doc.highlights.retain(|(range, _)| !range.is_empty());
        doc
    }
    fn break_line(&mut self) {
        if !self.text.is_empty() && !self.text.ends_with('\n') {
            self.text.push('\n');
        }
    }
}

/// Only instantiated for rows requested by the virtual list. Selection belongs
/// to one message; dropping this view discards its parsed content and selection.
pub struct MessageText {
    document: Document,
    source: String,
    id: String,
    focus: FocusHandle,
    anchor: usize,
    head: usize,
    dragging: bool,
}
impl MessageText {
    pub fn new(source: &str, id: String, cx: &mut Context<Self>) -> Self {
        Self {
            document: Document::parse(source),
            source: source.into(),
            id,
            focus: cx.focus_handle(),
            anchor: 0,
            head: 0,
            dragging: false,
        }
    }
    fn selection(&self) -> Range<usize> {
        self.anchor.min(self.head)..self.anchor.max(self.head)
    }
    fn copy(&mut self, _: &Copy, _: &mut Window, cx: &mut Context<Self>) {
        if let Some(text) = self
            .document
            .text
            .get(self.selection())
            .filter(|text| !text.is_empty())
        {
            cx.write_to_clipboard(ClipboardItem::new_string(text.to_owned()));
        }
    }
    fn select_all(&mut self, _: &SelectAll, _: &mut Window, cx: &mut Context<Self>) {
        self.anchor = 0;
        self.head = self.document.text.len();
        cx.notify();
    }
}
impl Focusable for MessageText {
    fn focus_handle(&self, _: &App) -> FocusHandle {
        self.focus.clone()
    }
}
fn word(text: &str, index: usize) -> Range<usize> {
    let index = boundary(text, index);
    let is_word = |c: char| c.is_alphanumeric() || c == '_';
    let start = text[..index]
        .char_indices()
        .rev()
        .take_while(|(_, c)| is_word(*c))
        .last()
        .map(|(ix, _)| ix)
        .unwrap_or(index);
    let end = text[index..]
        .char_indices()
        .take_while(|(_, c)| is_word(*c))
        .last()
        .map(|(ix, c)| index + ix + c.len_utf8())
        .unwrap_or(index);
    start..end
}
fn boundary(text: &str, index: usize) -> usize {
    let mut index = index.min(text.len());
    while !text.is_char_boundary(index) {
        index -= 1;
    }
    index
}
impl Render for MessageText {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let selection = self.selection();
        let selected = (!selection.is_empty()).then(|| {
            (
                selection,
                HighlightStyle {
                    background_color: Some(rgb(0x494678).into()),
                    ..Default::default()
                },
            )
        });
        let text = StyledText::new(SharedString::from(self.document.text.clone())).with_highlights(
            combine_highlights(self.document.highlights.clone(), selected).collect::<Vec<_>>(),
        );
        let down_layout = text.layout().clone();
        let move_layout = down_layout.clone();
        let focus = self.focus.clone();
        let source = self.source.clone();
        let id = self.id.clone();
        let has_selection = self.anchor != self.head;
        div()
            .id("message-text")
            .w_full()
            .whitespace_normal()
            .cursor_text()
            .track_focus(&self.focus)
            .key_context("MessageText")
            .on_action(cx.listener(Self::copy))
            .on_action(cx.listener(Self::select_all))
            .on_mouse_down(
                MouseButton::Left,
                cx.listener(move |this, event: &gpui::MouseDownEvent, window, cx| {
                    window.focus(&this.focus);
                    let ix = boundary(
                        &this.document.text,
                        down_layout
                            .index_for_position(event.position)
                            .unwrap_or_else(|ix| ix),
                    );
                    if event.click_count >= 3 {
                        this.anchor = 0;
                        this.head = this.document.text.len();
                        this.dragging = false;
                    } else if event.click_count == 2 {
                        let range = word(&this.document.text, ix);
                        this.anchor = range.start;
                        this.head = range.end;
                        this.dragging = false;
                    } else {
                        if !event.modifiers.shift {
                            this.anchor = ix;
                        }
                        this.head = ix;
                        this.dragging = true;
                    }
                    cx.notify();
                }),
            )
            .on_mouse_move(
                cx.listener(move |this, event: &gpui::MouseMoveEvent, _, cx| {
                    if this.dragging && event.pressed_button == Some(MouseButton::Left) {
                        let ix = boundary(
                            &this.document.text,
                            move_layout
                                .index_for_position(event.position)
                                .unwrap_or_else(|ix| ix),
                        );
                        if this.head != ix {
                            this.head = ix;
                            cx.notify();
                        }
                    }
                }),
            )
            .on_mouse_up(
                MouseButton::Left,
                cx.listener(|this, _, _, _| this.dragging = false),
            )
            .on_mouse_up_out(
                MouseButton::Left,
                cx.listener(|this, _, _, _| this.dragging = false),
            )
            .child(text)
            .context_menu(move |menu, _, _| {
                let source = source.clone();
                let id = id.clone();
                menu.action_context(focus.clone())
                    .menu_with_enable("Copy", Box::new(Copy), has_selection)
                    .menu("Select All", Box::new(SelectAll))
                    .separator()
                    .item(
                        PopupMenuItem::new("Copy message")
                            .disabled(source.is_empty())
                            .on_click(move |_, _, cx| {
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
    fn projects_markdown_without_loading_remote_content() {
        let doc = Document::parse(
            "# Hello **world**\n\n- *First*\n- `second`\n\n[Label](https://example.test) ![alt](https://example.test/image.png)\n\n<script>alert(1)</script>\n\n```rust\nlet n = 1;\n```\n",
        );
        assert!(doc.text.contains("Hello world\n• First\n• second"));
        assert!(doc.text.contains("Label [Image: alt]"));
        assert!(!doc.text.contains("https://"));
        assert!(doc.text.contains("<script>alert(1)</script>"));
        assert!(doc.text.contains("let n = 1;"));
        assert!(
            doc.highlights
                .iter()
                .any(|(_, style)| style.font_weight == Some(FontWeight::BOLD))
        );
        assert!(
            doc.highlights
                .iter()
                .any(|(_, style)| style.font_style == Some(FontStyle::Italic))
        );
        assert!(
            doc.highlights
                .iter()
                .all(|(range, _)| doc.text.get(range.clone()).is_some())
        );
    }
    #[test]
    fn selection_ranges_are_utf8_safe() {
        let text = "hi café 世界 🦀";
        assert_eq!(&text[word(text, 6)], "café");
        assert_eq!(&text[word(text, 10)], "世界");
        assert!(text.is_char_boundary(boundary(text, text.len() - 1)));
    }
}

#[cfg(test)]
mod interaction_tests {
    use super::*;
    use gpui::{KeyBinding, Modifiers, TestAppContext, point, px};

    #[gpui::test]
    fn keyboard_copy_uses_rendered_markdown_and_right_click_keeps_selection(
        cx: &mut TestAppContext,
    ) {
        cx.update(|cx| {
            gpui_component::init(cx);
            cx.bind_keys([
                KeyBinding::new("cmd-c", Copy, Some("MessageText")),
                KeyBinding::new("cmd-a", SelectAll, Some("MessageText")),
            ]);
        });
        let (view, cx) = cx.add_window_view(|_, cx| {
            MessageText::new("**Hello** café 世界", "synthetic-message".into(), cx)
        });
        cx.simulate_click(point(px(10.), px(10.)), Modifiers::default());
        cx.simulate_keystrokes("cmd-a cmd-c");
        assert_eq!(
            cx.read_from_clipboard()
                .and_then(|item| item.text())
                .as_deref(),
            Some("Hello café 世界")
        );
        cx.simulate_mouse_down(
            point(px(10.), px(10.)),
            MouseButton::Right,
            Modifiers::default(),
        );
        view.read_with(cx, |view, _| {
            assert_eq!(&view.document.text[view.selection()], "Hello café 世界")
        });
    }
}
