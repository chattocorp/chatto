use std::{
    io::{Read, Write},
    net::TcpListener,
    thread,
    time::Duration,
};

use chatto::{Client, Error, TimelineOptions};
use serde_json::{Value, json};

/// A real loopback HTTP peer exercises request headers, JSON, and redirect policy.
fn fixture(responses: Vec<String>) -> (String, thread::JoinHandle<Vec<(String, Value)>>) {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let origin = format!("http://{}", listener.local_addr().unwrap());
    listener.set_nonblocking(true).unwrap();
    let handle = thread::spawn(move || {
        let mut requests = Vec::new();
        for response in responses {
            let start = std::time::Instant::now();
            let mut stream = loop {
                match listener.accept() {
                    Ok((stream, _)) => break stream,
                    Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                        assert!(
                            start.elapsed() < Duration::from_secs(10),
                            "fixture request timed out"
                        );
                        thread::sleep(Duration::from_millis(5));
                    }
                    Err(error) => panic!("fixture accept: {error}"),
                }
            };
            stream.set_nonblocking(false).unwrap();
            stream
                .set_read_timeout(Some(Duration::from_secs(5)))
                .unwrap();
            stream
                .set_write_timeout(Some(Duration::from_secs(1)))
                .unwrap();
            let mut bytes = Vec::new();
            let (header, body) = loop {
                let mut chunk = [0; 4096];
                let count = stream.read(&mut chunk).unwrap();
                assert_ne!(count, 0, "incomplete HTTP request");
                bytes.extend_from_slice(&chunk[..count]);
                if let Some(end) = bytes.windows(4).position(|part| part == b"\r\n\r\n") {
                    let header = String::from_utf8(bytes[..end].to_vec()).unwrap();
                    let length: usize = header
                        .lines()
                        .find_map(|line| {
                            line.to_ascii_lowercase()
                                .strip_prefix("content-length: ")
                                .map(str::to_owned)
                        })
                        .unwrap()
                        .parse()
                        .unwrap();
                    if bytes.len() >= end + 4 + length {
                        break (
                            header,
                            serde_json::from_slice(&bytes[end + 4..end + 4 + length]).unwrap(),
                        );
                    }
                }
            };
            requests.push((header, body));
            // Rejected responses need not be consumed. A bounded write also
            // handles clients that stop reading before closing the connection.
            // Each test still checks the exact client result below.
            let _ = stream.write_all(response.as_bytes());
        }
        requests
    });
    (origin, handle)
}

fn json_response(body: Value) -> String {
    let body = body.to_string();
    format!(
        "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
        body.len()
    )
}

#[test]
fn origin_and_credential_validation_does_not_expose_input() {
    for origin in [
        "https://example.test",
        "http://localhost:4000",
        "http://127.0.0.1:4000",
        "http://[::1]:4000",
    ] {
        let client = Client::new(origin, "synthetic-secret").unwrap();
        assert!(!format!("{client:?}").contains("synthetic-secret"));
        assert!(!format!("{client:?}").contains(origin));
    }
    for origin in [
        "http://example.test",
        "https://user:secret@example.test",
        "https://example.test/path",
        "https://example.test/?token=secret",
        "https://example.test/#secret",
        "file:///secret",
    ] {
        let error = Client::new(origin, "synthetic-secret").unwrap_err();
        assert!(!error.to_string().contains("secret"));
    }
    assert!(Client::new("https://example.test", " ").is_err());
    assert!(Client::new("https://example.test", "secret\r\nheader: injected").is_err());
}

#[tokio::test]
async fn public_json_contract_and_pagination() {
    let (origin, server) = fixture(vec![
        json_response(json!({"user":{"profile":{"id":"self", "displayName":"Synthetic viewer"}}})),
        json_response(
            json!({"rooms":[{"room":{"id":"room", "kind":"ROOM_KIND_DM"}, "viewerState":{"isMember":true}, "memberUserIds":["self","author"]}],"page":{"hasMore":true}}),
        ),
        json_response(json!({"page":{
            "events":[
                {"createdAt":"2026-11-05T12:00:00Z", "messagePosted":{"message":{"id":"own","actorId":"self","body":"","reactions":[{"emoji":"test"}]}}},
                {"messagePosted":{"message":{"id":"deleted","body":"private","deletedAt":"2026-11-05T12:00:00Z"}}},
                {"messagePosted":{"message":{"id":"attachment", "attachments":[{"id":"asset"}],"echoOfEventId":"original"}}},
                {"futureEvent":{"id":"unknown"}}
            ],
            "includes":{"users":{"self":{"id":"self","displayName":"Synthetic viewer"}}},
            "startCursor":"opaque-start", "endCursor":"opaque-end", "hasOlder":true
        }})),
    ]);
    let client = Client::new(&origin, "synthetic-token").unwrap();
    assert_eq!(client.viewer().await.unwrap().id, "self");
    let rooms = client.rooms(100, 50).await.unwrap();
    assert!(rooms.page.has_more);
    assert!(rooms.rooms[0].viewer_state.is_member);
    assert_eq!(rooms.rooms[0].member_user_ids, ["self", "author"]);
    let page = client
        .timeline(
            "room",
            TimelineOptions {
                limit: 50,
                before: Some("opaque-start".into()),
                ..Default::default()
            },
        )
        .await
        .unwrap();
    assert_eq!(page.messages.len(), 2);
    assert_eq!(page.messages[0].body.as_deref(), Some(""));
    assert_eq!(page.messages[0].created_at, "2026-11-05T12:00:00Z");
    assert!(page.messages[0].metadata.contains_key("reactions"));
    assert!(page.messages[1].body.is_none());
    assert_eq!(page.messages[1].metadata["echoOfEventId"], "original");
    assert_eq!(page.users["self"].display_name, "Synthetic viewer");
    assert!(page.has_older);
    assert_eq!(page.start_cursor, "opaque-start");
    let requests = server.join().unwrap();
    assert!(
        requests[0]
            .0
            .starts_with("POST /api/connect/chatto.api.v1.ViewerService/GetViewer ")
    );
    for (headers, _) in &requests {
        let headers = headers.to_ascii_lowercase();
        assert!(headers.contains("authorization: bearer synthetic-token"));
        assert!(headers.contains("connect-protocol-version: 1"));
        assert!(headers.contains("content-type: application/json"));
    }
    assert_eq!(requests[1].1, json!({"page":{"offset":100,"limit":50}}));
    assert!(
        requests[2]
            .0
            .starts_with("POST /api/connect/chatto.api.v1.RoomService/GetRoomEvents ")
    );
    assert_eq!(
        requests[2].1,
        json!({"roomId":"room","limit":50,"before":"opaque-start"})
    );
}

#[tokio::test]
async fn raw_events_preserve_unknown_kinds() {
    let (origin, server) = fixture(vec![json_response(
        json!({"page":{"events":[{"futureEvent":{"value":42}}]}}),
    )]);
    let page = Client::new(&origin, "token")
        .unwrap()
        .room_events("room", TimelineOptions::default())
        .await
        .unwrap();
    assert_eq!(page.events[0]["futureEvent"]["value"], 42);
    server.join().unwrap();
}

#[tokio::test]
async fn redirects_errors_and_malformed_responses_are_private() {
    for (response, expected) in [
        ("HTTP/1.1 302 Found\r\nLocation: https://example.test/secret\r\nContent-Length: 0\r\nConnection: close\r\n\r\n".into(), Error::Http(302)),
        ("HTTP/1.1 401 Unauthorized\r\nContent-Length: 6\r\nConnection: close\r\n\r\nsecret".into(), Error::Http(401)),
        (json_response(json!({"user":{"profile":{}}})), Error::InvalidResponse),
        ("HTTP/1.1 200 OK\r\nContent-Length: 6\r\nConnection: close\r\n\r\nsecret".into(), Error::InvalidResponse),
        (format!("HTTP/1.1 200 OK\r\nContent-Length: 4194305\r\nConnection: close\r\n\r\n{}", "x".repeat(4194305)), Error::ResponseTooLarge),
        (format!("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\nConnection: close\r\n\r\n400001\r\n{}\r\n0\r\n\r\n", "x".repeat(4194305)), Error::ResponseTooLarge),
    ] {
        let (origin, server) = fixture(vec![response]);
        let result = Client::new(&origin, "token").unwrap().viewer().await;
        let error = result.err().expect("expected a private error");
        assert_eq!(error, expected);
        assert!(!error.to_string().contains("secret"));
        server.join().unwrap();
    }
}

#[tokio::test]
async fn invalid_pagination_is_rejected_without_a_request() {
    let client = Client::new("http://127.0.0.1:1", "token").unwrap();
    assert!(matches!(
        client.rooms(0, 101).await,
        Err(Error::InvalidInput(_))
    ));
    assert!(matches!(
        client.rooms(u32::MAX, 50).await,
        Err(Error::InvalidInput(_))
    ));
    assert!(matches!(
        client
            .timeline(
                "room",
                TimelineOptions {
                    before: Some("a".into()),
                    after: Some("b".into()),
                    ..Default::default()
                }
            )
            .await,
        Err(Error::InvalidInput(_))
    ));
    assert!(matches!(
        client.timeline("", TimelineOptions::default()).await,
        Err(Error::InvalidInput(_))
    ));
}

#[tokio::test]
async fn password_login_and_logout_use_the_human_session_contract() {
    let (origin, server) = fixture(vec![
        json_response(
            json!({"token":"synthetic-access", "refreshToken":"synthetic-refresh", "expiresIn":900, "refreshTokenExpiresIn":86400}),
        ),
        json_response(json!({"user":{"profile":{"id":"viewer"}}})),
        json_response(json!({})),
    ]);
    let before = std::time::SystemTime::now();
    let session = Client::login_password(&origin, "synthetic-user", "  Synthetic password 🦀  ")
        .await
        .unwrap();
    assert!(session.access_expires_at > before);
    assert!(session.session_expires_at > session.access_expires_at);
    let client = session.client();
    assert!(!format!("{client:?}").contains("synthetic-access"));
    assert_eq!(client.viewer().await.unwrap().id, "viewer");
    session.logout().await.unwrap();
    let requests = server.join().unwrap();
    assert!(requests[0].0.starts_with("POST /auth/login "));
    assert!(
        !requests[0]
            .0
            .to_ascii_lowercase()
            .contains("authorization:")
    );
    assert_eq!(
        requests[0].1,
        json!({"login":"synthetic-user", "password":"  Synthetic password 🦀  "})
    );
    assert!(
        requests[1]
            .0
            .to_ascii_lowercase()
            .contains("authorization: bearer synthetic-access")
    );
    assert!(requests[2].0.starts_with("POST /auth/logout "));
    assert_eq!(requests[2].1, json!({"refreshToken":"synthetic-refresh"}));
    assert!(
        requests[2]
            .0
            .to_ascii_lowercase()
            .contains("authorization: bearer synthetic-access")
    );
}

#[tokio::test]
async fn password_login_rejections_do_not_disclose_credentials_or_follow_redirects() {
    for (response, expected) in [
        ("HTTP/1.1 401 Unauthorized\r\nContent-Length: 6\r\nConnection: close\r\n\r\nsecret".into(), Error::Http(401)),
        ("HTTP/1.1 403 Forbidden\r\nContent-Length: 6\r\nConnection: close\r\n\r\nsecret".into(), Error::Http(403)),
        ("HTTP/1.1 307 Temporary Redirect\r\nLocation: https://example.test/secret\r\nContent-Length: 0\r\nConnection: close\r\n\r\n".into(), Error::Http(307)),
        (json_response(json!({"token":"secret", "refreshToken":"secret", "expiresIn":0,"refreshTokenExpiresIn":86400})), Error::InvalidResponse),
        (json_response(json!({"token":"secret", "expiresIn":900})), Error::InvalidResponse),
        (json_response(json!({"token":"secret", "refreshToken":"secret", "expiresIn":900,"refreshTokenExpiresIn":-1})), Error::InvalidResponse),
    ] {
        let (origin, server) = fixture(vec![response]);
        let error = Client::login_password(&origin, "synthetic-user", "secret").await.err().expect("login must fail");
        assert_eq!(error, expected);
        assert!(!format!("{error:?} {error}").contains("secret"));
        assert_eq!(server.join().unwrap().len(), 1);
    }
}

#[tokio::test]
async fn password_login_validates_before_sending_credentials() {
    for (origin, username, password) in [
        ("http://example.test", "synthetic-user", "secret"),
        ("https://example.test/path", "synthetic-user", "secret"),
        ("http://127.0.0.1:1", " ", "secret"),
        ("http://127.0.0.1:1", "synthetic-user", ""),
    ] {
        assert!(matches!(
            Client::login_password(origin, username, password).await,
            Err(Error::InvalidInput(_))
        ));
    }
}

#[tokio::test]
async fn prepared_refresh_identity_survives_restart_and_uncertain_failure() {
    let (origin, server) = fixture(vec![
        "HTTP/1.1 503 Service Unavailable\r\nContent-Length: 0\r\nConnection: close\r\n\r\n".into(),
        json_response(
            json!({"access_token":"replacement-access","refresh_token":"replacement-refresh","expires_in":900,"refresh_token_expires_in":3600}),
        ),
    ]);
    let mut session = chatto::HumanSession::restore(chatto::SessionCredentials {
        origin,
        access_token: "original-access".into(),
        refresh_token: "original-refresh".into(),
        refresh_request_id: String::new(),
        access_expires_at: std::time::SystemTime::now(),
        session_expires_at: std::time::SystemTime::now() + std::time::Duration::from_secs(3600),
    })
    .unwrap();
    session.prepare_refresh();
    let saved = serde_json::to_vec(&session.credentials()).unwrap();
    let request_id = session.credentials().refresh_request_id;
    assert!(uuid::Uuid::parse_str(&request_id).is_ok());
    assert!(matches!(session.refresh().await, Err(Error::Http(503))));
    let restored = chatto::HumanSession::restore(serde_json::from_slice(&saved).unwrap()).unwrap();
    let fresh = restored.refresh().await.unwrap();
    assert!(fresh.credentials().refresh_request_id.is_empty());
    assert_eq!(fresh.credentials().access_token, "replacement-access");
    let requests = server.join().unwrap();
    for (headers, input) in &requests {
        assert!(headers.starts_with("POST /oauth/token "));
        assert!(!headers.to_ascii_lowercase().contains("authorization:"));
        assert_eq!(input["refresh_request_id"], request_id);
        assert_eq!(input["refresh_token"], "original-refresh");
        assert_eq!(input["grant_type"], "refresh_token");
    }
}
