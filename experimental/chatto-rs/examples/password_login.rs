//! Check password login and revoke the resulting session without logging user data.
use chatto::{Client, Error, TimelineOptions};

async fn read(client: &Client) -> Result<(), Error> {
    client.viewer().await?;
    let rooms = client.rooms(0, 100).await?;
    println!("Signed in. Read {} rooms.", rooms.rooms.len());
    if let Some(room) = rooms.rooms.first() {
        let page = client
            .timeline(&room.room.id, TimelineOptions::default())
            .await?;
        println!("Read {} messages from the first room.", page.messages.len());
    }
    Ok(())
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let session = Client::login_password(
        &std::env::var("CHATTO_ORIGIN")?,
        &std::env::var("CHATTO_USERNAME")?,
        &std::env::var("CHATTO_PASSWORD")?,
    )
    .await?;
    let result = read(&session.client()).await;
    let logout = session.logout().await;
    result?;
    logout?;
    println!("Signed out.");
    Ok(())
}
