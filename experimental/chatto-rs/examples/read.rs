//! Read one server page without logging credentials, profiles, or message bodies.
use chatto::{Client, TimelineOptions};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let origin = std::env::var("CHATTO_ORIGIN")?;
    let token = std::env::var("CHATTO_TOKEN")?;
    let client = Client::new(&origin, &token)?;
    client.viewer().await?;
    let page = client.rooms(0, 100).await?;
    println!("Authenticated. Read {} rooms.", page.rooms.len());
    if let Some(entry) = page.rooms.first() {
        let timeline = client
            .timeline(&entry.room.id, TimelineOptions::default())
            .await?;
        println!(
            "Read {} messages from the first room.",
            timeline.messages.len()
        );
    }
    Ok(())
}
