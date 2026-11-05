//! Private protobuf subset of proto/chatto/realtime/v1/realtime.proto.
//! Tags and wire types must match the public schema. We request LIVE_ONLY and
//! hydrate resources at its boundary, so no snapshot resource decoder is needed.

#[derive(Clone, PartialEq, prost::Message)]
pub(crate) struct Subscribe {
    #[prost(uint32, tag = "1")]
    pub protocol_version: u32,
    #[prost(string, optional, tag = "2")]
    pub bearer_token: Option<String>,
    #[prost(string, optional, tag = "3")]
    pub resume_cursor: Option<String>,
    #[prost(uint32, tag = "4")]
    pub initial_state: u32,
}
#[derive(Clone, PartialEq, prost::Message)]
pub(crate) struct Frame {
    #[prost(oneof = "frame::Kind", tags = "1,2,3,4")]
    pub kind: Option<frame::Kind>,
}
pub(crate) mod frame {
    #[derive(Clone, PartialEq, prost::Oneof)]
    pub enum Kind {
        #[prost(message, tag = "1")]
        Event(super::Event),
        #[prost(message, tag = "2")]
        Heartbeat(super::Heartbeat),
        #[prost(message, tag = "3")]
        Close(super::Close),
        #[prost(message, tag = "4")]
        CaughtUp(super::CaughtUp),
    }
}
#[derive(Clone, PartialEq, prost::Message)]
pub(crate) struct Event {
    #[prost(string, tag = "1")]
    pub id: String,
    #[prost(string, optional, tag = "4")]
    pub cursor: Option<String>,
    #[prost(
        oneof = "event::Kind",
        tags = "10,11,12,13,14,15,18,19,24,25,26,27,28,29,47,48,49,50,52,53,56,57,58,59,62,64,65,66,67,73"
    )]
    pub kind: Option<event::Kind>,
}
pub(crate) mod event {
    #[derive(Clone, PartialEq, prost::Oneof)]
    pub enum Kind {
        #[prost(message, tag = "10")]
        RoomCreated(super::Room),
        #[prost(message, tag = "11")]
        RoomUpdated(super::Room),
        #[prost(message, tag = "12")]
        RoomDeleted(super::Room),
        #[prost(message, tag = "13")]
        RoomArchived(super::Room),
        #[prost(message, tag = "14")]
        RoomUnarchived(super::Room),
        #[prost(message, tag = "15")]
        RoomUniversalChanged(super::Room),
        #[prost(message, tag = "18")]
        Joined(super::Room),
        #[prost(message, tag = "19")]
        Left(super::Room),
        #[prost(message, tag = "24")]
        Posted(super::Room),
        #[prost(message, tag = "25")]
        Edited(super::Room),
        #[prost(message, tag = "26")]
        Retracted(super::Room),
        #[prost(message, tag = "27")]
        Pinned(super::Room),
        #[prost(message, tag = "28")]
        Unpinned(super::Room),
        #[prost(message, tag = "29")]
        Thread(super::Room),
        #[prost(message, tag = "47")]
        Profile(super::Empty),
        #[prost(message, tag = "48")]
        DeletedAccount(super::Empty),
        #[prost(message, tag = "49")]
        ReactionAdded(super::Room),
        #[prost(message, tag = "50")]
        ReactionRemoved(super::Room),
        #[prost(message, tag = "52")]
        ThreadViewer(super::Room),
        #[prost(message, tag = "53")]
        Server(super::Empty),
        #[prost(message, tag = "56")]
        Notifications(super::Empty),
        #[prost(message, tag = "57")]
        Unread(super::Room),
        #[prost(message, tag = "58")]
        Read(super::Room),
        #[prost(message, tag = "59")]
        Layout(super::Empty),
        #[prost(message, tag = "62")]
        RoleDeleted(super::Empty),
        #[prost(message, tag = "64")]
        RoleAssigned(super::Empty),
        #[prost(message, tag = "65")]
        RoleRevoked(super::Empty),
        #[prost(message, tag = "66")]
        RolePermissions(super::Empty),
        #[prost(message, tag = "67")]
        Permissions(super::Empty),
        #[prost(message, tag = "73")]
        Deactivation(super::Empty),
    }
}
#[derive(Clone, PartialEq, prost::Message)]
pub(crate) struct Room {
    #[prost(string, tag = "1")]
    pub room_id: String,
}
#[derive(Clone, PartialEq, prost::Message)]
pub(crate) struct Empty {}
#[derive(Clone, PartialEq, prost::Message)]
pub(crate) struct Heartbeat {
    #[prost(string, optional, tag = "1")]
    pub cursor: Option<String>,
}
#[derive(Clone, PartialEq, prost::Message)]
pub(crate) struct CaughtUp {
    #[prost(string, tag = "1")]
    pub cursor: String,
    #[prost(uint32, tag = "2")]
    pub recovery: u32,
}
#[derive(Clone, PartialEq, prost::Message)]
pub(crate) struct Close {
    #[prost(uint32, tag = "1")]
    pub code: u32,
    #[prost(bool, tag = "3")]
    pub reconnect: bool,
    #[prost(message, optional, tag = "4")]
    pub retry_after: Option<Delay>,
}
#[derive(Clone, PartialEq, prost::Message)]
pub(crate) struct Delay {
    #[prost(int64, tag = "1")]
    pub seconds: i64,
    #[prost(int32, tag = "2")]
    pub nanos: i32,
}
