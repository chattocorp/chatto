//! Observe a live room without printing credentials or server content.
use chatto::{
    Client, Error,
    realtime::{ClientProvider, LiveServer, Status},
};
use std::{sync::Arc, time::Duration};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let client = Client::new(
        &std::env::var("CHATTO_ORIGIN")?,
        &std::env::var("CHATTO_TOKEN")?,
    )?;
    let room = std::env::var("CHATTO_ROOM").ok();
    let provider: ClientProvider = Arc::new(move |force| {
        let client = client.clone();
        Box::pin(async move {
            if force {
                Err(Error::Http(401))
            } else {
                Ok(client)
            }
        })
    });
    let server = LiveServer::start(provider, room, 500);
    let mut updates = server.updates();
    let watch = async {
        loop {
            updates.changed().await?;
            let state = updates.borrow_and_update().clone();
            if state.status == Status::Failed {
                return Err::<(), Box<dyn std::error::Error>>(
                    state.error.clone().unwrap_or(Error::Transport).into(),
                );
            }
            if state.status == Status::Connected && !state.loading {
                println!(
                    "Connected. rooms={} groups={} messages={}",
                    state.directory.rooms.len(),
                    state.directory.groups.len(),
                    state.timeline.messages.len()
                );
            }
        }
    };
    match tokio::time::timeout(Duration::from_secs(20), watch).await {
        Ok(result) => result,
        Err(_) => Ok(()),
    }
}
