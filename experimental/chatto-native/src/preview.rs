//! Offline design fixture. It never opens storage or connects to a server.
use chatto::{
    Message, Profile, Room, RoomEntry,
    realtime::{ServerState, Status},
};
use std::collections::BTreeMap;

pub fn state() -> ServerState {
    let mut state = ServerState {
        status: Status::Connected,
        selected: Some("general".into()),
        ..Default::default()
    };
    state.directory.server.name = "Chatto workspace".into();
    state.directory.server.description =
        Some("A place for our team to build, share, and stay in touch.".into());
    state.directory.viewer = Profile {
        id: "maya".into(),
        display_name: "Maya Chen".into(),
        login: "maya".into(),
        ..Default::default()
    };
    for (id, name) in [
        ("maya", "Maya Chen"),
        ("jules", "Jules Rivera"),
        ("sam", "Sam Patel"),
    ] {
        state.directory.users.insert(
            id.into(),
            Profile {
                id: id.into(),
                display_name: name.into(),
                login: id.into(),
                ..Default::default()
            },
        );
    }
    for (id, name, description) in [
        ("general", "general", "A place for the whole team"),
        ("design", "design", "Ideas, details, and work in progress"),
        ("engineering", "engineering", "Building Chatto together"),
        ("random", "random", "Everything else"),
    ] {
        state.directory.rooms.push(RoomEntry {
            room: Room {
                id: id.into(),
                name: name.into(),
                description: description.into(),
                kind: "ROOM_KIND_CHANNEL".into(),
                ..Default::default()
            },
            viewer_state: Default::default(),
            member_user_ids: vec![],
        });
    }
    state.directory.rooms.push(RoomEntry {
        room: Room {
            id: "jules".into(),
            kind: "ROOM_KIND_DM".into(),
            ..Default::default()
        },
        viewer_state: Default::default(),
        member_user_ids: vec!["maya".into(), "jules".into()],
    });
    for (index, (actor, body)) in [
        ("maya", "Morning everyone! I've been working on the native app. It’s starting to feel like Chatto."),
        ("jules", "The quieter sidebar is a big improvement. Love having a little more room for the conversation."),
        ("sam", "A few things to try today:\n\n- Scroll back through a long conversation\n- Select and copy a message\n- Check the keyboard navigation"),
        ("maya", "**Small details matter.** Consistent type, spacing, and headers make the whole app feel more settled."),
        ("jules", "And Markdown still works: *a little emphasis*, **something important**, and `inline code`.\n\n> Keep the conversation at the center."),
        ("sam", "Looking good. Let’s keep iterating from here."),
    ].into_iter().enumerate() {
        state.timeline.messages.push(Message { id:format!("preview-{index}"), actor_id:actor.into(), body:Some(body.into()), created_at:format!("2026-11-05T09:{:02}:00Z", 30 + index * 2), deleted_at:None, metadata:BTreeMap::new() });
    }
    state.timeline.messages[4].body = Some("## A more native conversation\n\nMarkdown now has proper paragraphs, lists, quotes, tables, and code blocks.\n\n```rust\nlet rooms = client.rooms(0, 50).await?;\n```".into());
    state.timeline.messages[5].body =
        Some("A little color for the conversation. **Images are native too.**".into());
    state.timeline.messages[5].metadata.insert("attachments".into(), serde_json::json!([{
        "id":"preview-image", "filename":"color-study.png", "contentType":"image/png", "width":256, "height":256,
        "assetUrl":{"url":"/preview.png"}
    }]));
    state
}
