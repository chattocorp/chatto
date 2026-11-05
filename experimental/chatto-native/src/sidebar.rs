//! Host-only sidebar order, labels, filters, and collapsed-section presentation.
use chatto::{Profile, RoomEntry, realtime::Directory};
use std::collections::{BTreeMap, BTreeSet};

#[derive(Clone)]
pub enum Item {
    Room {
        id: String,
        label: String,
        unread: bool,
        important: u32,
        dm: bool,
    },
    Link {
        id: String,
        label: String,
        url: String,
    },
}
#[derive(Clone)]
pub struct Section {
    pub id: String,
    pub label: String,
    pub unread: usize,
    pub items: Vec<Item>,
}

pub fn profile_label(profile: &Profile) -> String {
    if !profile.display_name.is_empty() {
        profile.display_name.clone()
    } else if !profile.login.is_empty() {
        profile.login.clone()
    } else {
        "Deleted user".into()
    }
}
pub fn room_label(entry: &RoomEntry, directory: &Directory) -> String {
    if entry.room.kind != "ROOM_KIND_DM" {
        return format!("#{}", entry.room.name);
    }
    let ids: Vec<_> = entry
        .member_user_ids
        .iter()
        .filter(|id| **id != directory.viewer.id)
        .collect();
    if ids.is_empty() {
        return format!("{} (You)", profile_label(&directory.viewer));
    }
    ids.iter()
        .map(|id| {
            directory
                .users
                .get(*id)
                .map(profile_label)
                .unwrap_or_else(|| "Deleted user".into())
        })
        .collect::<Vec<_>>()
        .join(", ")
}
fn item(entry: &RoomEntry, directory: &Directory) -> Item {
    Item::Room {
        id: entry.room.id.clone(),
        label: room_label(entry, directory),
        unread: entry.viewer_state.has_unread
            || directory
                .notifications
                .get(&entry.room.id)
                .is_some_and(|count| count.unread_count > 0),
        important: directory
            .notifications
            .get(&entry.room.id)
            .map(|count| count.important_unread_count)
            .unwrap_or(0),
        dm: entry.room.kind == "ROOM_KIND_DM",
    }
}
fn section(id: String, label: String, items: Vec<Item>) -> Section {
    let unread = items
        .iter()
        .filter(|item| matches!(item, Item::Room { unread: true, .. }))
        .count();
    Section {
        id,
        label,
        unread,
        items,
    }
}
pub fn sections(directory: &Directory) -> Vec<Section> {
    let rooms: BTreeMap<_, _> = directory
        .rooms
        .iter()
        .filter(|entry| !entry.room.archived)
        .map(|entry| (&entry.room.id, entry))
        .collect();
    let mut used = BTreeSet::new();
    let mut result = Vec::new();
    for group in &directory.groups {
        let mut items = Vec::new();
        for entry in &group.items {
            if let Some(room) = &entry.room {
                if let Some(current) = rooms.get(&room.room.id)
                    && current.room.kind != "ROOM_KIND_DM"
                    && used.insert(current.room.id.clone())
                {
                    items.push(item(current, directory));
                }
            } else if let Some(link) = &entry.sidebar_link {
                items.push(Item::Link {
                    id: link.id.clone(),
                    label: link.label.clone(),
                    url: link.url.clone(),
                });
            }
        }
        if !items.is_empty() {
            result.push(section(
                format!("group:{}", group.id),
                if group.name.trim().is_empty() {
                    "Channels".into()
                } else {
                    group.name.clone()
                },
                items,
            ));
        }
    }
    let mut channels: Vec<_> = rooms
        .values()
        .filter(|entry| entry.room.kind != "ROOM_KIND_DM" && !used.contains(&entry.room.id))
        .copied()
        .collect();
    channels.sort_by_key(|entry| entry.room.name.to_lowercase());
    if !channels.is_empty() {
        result.push(section(
            "channels".into(),
            if result.is_empty() {
                "Channels".into()
            } else {
                "Other channels".into()
            },
            channels
                .into_iter()
                .map(|entry| item(entry, directory))
                .collect(),
        ));
    }
    let mut dms: Vec<_> = rooms
        .values()
        .filter(|entry| entry.room.kind == "ROOM_KIND_DM")
        .copied()
        .collect();
    dms.sort_by_key(|entry| room_label(entry, directory).to_lowercase());
    result.push(section(
        "dms".into(),
        "Direct messages".into(),
        dms.into_iter()
            .map(|entry| item(entry, directory))
            .collect(),
    ));
    result
}

#[cfg(test)]
mod tests {
    use super::*;
    use chatto::{Room, RoomGroup, RoomGroupItem, RoomViewerState};
    fn room(id: &str, name: &str, kind: &str) -> RoomEntry {
        RoomEntry {
            room: Room {
                id: id.into(),
                name: name.into(),
                kind: kind.into(),
                ..Default::default()
            },
            viewer_state: RoomViewerState {
                is_member: true,
                has_unread: true,
            },
            member_user_ids: vec![],
        }
    }
    #[test]
    fn groups_intersect_current_rooms_and_do_not_duplicate() {
        let mut channel = room("channel", "testing", "ROOM_KIND_CHANNEL");
        let mut dm = room("dm", "", "ROOM_KIND_DM");
        dm.member_user_ids = vec!["self".into(), "other".into()];
        let mut directory = Directory {
            viewer: Profile {
                id: "self".into(),
                ..Default::default()
            },
            rooms: vec![channel.clone(), dm.clone()],
            ..Default::default()
        };
        directory.users.insert(
            "other".into(),
            Profile {
                id: "other".into(),
                display_name: "Synthetic colleague".into(),
                ..Default::default()
            },
        );
        directory.groups = vec![RoomGroup {
            id: "group".into(),
            name: "Team".into(),
            items: vec![
                RoomGroupItem {
                    room: Some(channel.clone()),
                    ..Default::default()
                },
                RoomGroupItem {
                    room: Some(channel.clone()),
                    ..Default::default()
                },
                RoomGroupItem {
                    room: Some(room("gone", "gone", "ROOM_KIND_CHANNEL")),
                    ..Default::default()
                },
            ],
        }];
        let result = sections(&directory);
        assert_eq!(result[0].items.len(), 1);
        assert_eq!(result[0].unread, 1);
        assert_eq!(room_label(&channel, &directory), "#testing");
        assert_eq!(room_label(&dm, &directory), "Synthetic colleague");
        channel.room.archived = true;
        directory.rooms[0] = channel;
        assert_eq!(sections(&directory).len(), 1);
    }
    #[test]
    fn deleted_dm_partner_is_not_a_self_dm() {
        let mut dm = room("dm", "", "ROOM_KIND_DM");
        dm.member_user_ids = vec!["self".into(), "deleted".into()];
        let directory = Directory {
            viewer: Profile {
                id: "self".into(),
                display_name: "Synthetic viewer".into(),
                ..Default::default()
            },
            ..Default::default()
        };
        assert_eq!(room_label(&dm, &directory), "Deleted user");
        dm.member_user_ids = vec!["self".into()];
        assert_eq!(room_label(&dm, &directory), "Synthetic viewer (You)");
    }
}
