//! Minimal list changes preserve GPUI's cached row heights and scroll anchor.
use std::ops::Range;

/// Find the changed middle, keeping the unchanged prefix and suffix in place.
/// Inserting an older page must not reset the existing list's measurements.
pub fn change<T: PartialEq>(old: &[T], new: &[T]) -> (Range<usize>, usize) {
    let prefix = old.iter().zip(new).take_while(|(a, b)| a == b).count();
    let suffix = old[prefix..]
        .iter()
        .rev()
        .zip(new[prefix..].iter().rev())
        .take_while(|(a, b)| a == b)
        .count();
    (prefix..old.len() - suffix, new.len() - prefix - suffix)
}

#[cfg(test)]
mod tests {
    use super::*;
    use gpui::{ListAlignment, ListOffset, ListState, px};
    #[test]
    fn prepending_history_keeps_the_visible_message_and_pixel_offset() {
        let old = ["c", "d", "e"];
        let new = ["a", "b", "c", "d", "e"];
        let state = ListState::new(old.len(), ListAlignment::Bottom, px(200.));
        state.scroll_to(ListOffset {
            item_ix: 1,
            offset_in_item: px(17.),
        });
        let (range, count) = change(&old, &new);
        assert_eq!((range.clone(), count), (0..0, 2));
        state.splice(range, count);
        let offset = state.logical_scroll_top();
        assert_eq!(new[offset.item_ix], "d");
        assert_eq!(offset.offset_in_item, px(17.));
    }
    #[test]
    fn append_delete_replace_and_unchanged_ranges() {
        assert_eq!(change(&[1, 2], &[1, 2, 3]), (2..2, 1));
        assert_eq!(change(&[1, 2, 3], &[1, 3]), (1..2, 0));
        assert_eq!(change(&[1, 2, 3], &[1, 4, 3]), (1..2, 1));
        assert_eq!(change(&[1, 2], &[1, 2]), (2..2, 0));
    }
}

#[cfg(test)]
mod rendering_tests {
    use gpui::{
        AppContext, Context, IntoElement, ListAlignment, ListOffset, ListState, Render, Styled,
        TestAppContext, Window, list, point, px, size,
    };
    use std::{cell::Cell, rc::Rc};

    struct LargeTimeline {
        state: ListState,
        rendered: Rc<Cell<usize>>,
    }
    impl Render for LargeTimeline {
        fn render(&mut self, _: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
            let rendered = self.rendered.clone();
            list(self.state.clone(), move |index, _, cx| {
                rendered.set(rendered.get() + 1);
                let source = format!(
                    "**Message {index}**\n\nA paragraph with *emphasis* and `code`.{}",
                    "\nMore text.".repeat(index % 4)
                );
                cx.new(|cx| crate::message_text::MessageText::new(&source, index.to_string(), cx))
                    .into_any_element()
            })
            .w_full()
            .h_full()
        }
    }
    #[gpui::test]
    fn only_visible_markdown_rows_are_built_in_a_large_timeline(cx: &mut TestAppContext) {
        cx.update(gpui::init);
        let cx = cx.add_empty_window();
        let state = ListState::new(10_000, ListAlignment::Bottom, px(100.));
        let rendered = Rc::new(Cell::new(0));
        cx.draw(point(px(0.), px(0.)), size(px(600.), px(500.)), |_, cx| {
            cx.new(|_| LargeTimeline {
                state: state.clone(),
                rendered: rendered.clone(),
            })
            .into_any_element()
        });
        assert!(
            rendered.get() > 0 && rendered.get() < 100,
            "Only the viewport and overdraw should render"
        );
        rendered.set(0);
        state.scroll_to(ListOffset {
            item_ix: 5000,
            offset_in_item: px(0.),
        });
        cx.draw(point(px(0.), px(0.)), size(px(600.), px(500.)), |_, cx| {
            cx.new(|_| LargeTimeline {
                state: state.clone(),
                rendered: rendered.clone(),
            })
            .into_any_element()
        });
        assert!(
            rendered.get() > 0 && rendered.get() < 100,
            "Jumping into history must stay virtualized"
        );
        assert_eq!(state.logical_scroll_top().item_ix, 5000);
    }
}
