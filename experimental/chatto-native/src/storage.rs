//! The host owns persistence: non-secret settings in a private file, credentials
//! in macOS Keychain. A process-held file lock prevents concurrent refreshes by
//! two app instances. Refresh identity is durable before rotation starts.
use chatto::{Client, Error, HumanSession, SessionCredentials, realtime::ClientProvider};
use serde::{Deserialize, Serialize};
use std::{
    collections::BTreeSet,
    fs::{self, File, OpenOptions},
    io::Write,
    os::unix::fs::{DirBuilderExt, OpenOptionsExt, PermissionsExt},
    path::{Path, PathBuf},
    sync::{Arc, Mutex},
    time::{Duration, SystemTime},
};
use tokio::sync::Mutex as AsyncMutex;

#[derive(Clone, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct Settings {
    pub origin: String,
    pub selected: Option<String>,
    pub collapsed: BTreeSet<String>,
}

trait Secrets: Send + Sync {
    fn read(&self) -> Result<Option<Vec<u8>>, Error>;
    fn write(&self, bytes: &[u8]) -> Result<(), Error>;
    fn delete(&self) -> Result<(), Error>;
}
struct Keychain {
    account: String,
}
const SERVICE: &str = "run.chatto.native.experimental.session";
const NOT_FOUND: i32 = -25300;
impl Secrets for Keychain {
    fn read(&self) -> Result<Option<Vec<u8>>, Error> {
        match security_framework::passwords::get_generic_password(SERVICE, &self.account) {
            Ok(bytes) => Ok(Some(bytes)),
            Err(error) if error.code() == NOT_FOUND => Ok(None),
            Err(_) => Err(Error::Persistence),
        }
    }
    fn write(&self, bytes: &[u8]) -> Result<(), Error> {
        security_framework::passwords::set_generic_password(SERVICE, &self.account, bytes)
            .map_err(|_| Error::Persistence)
    }
    fn delete(&self) -> Result<(), Error> {
        match security_framework::passwords::delete_generic_password(SERVICE, &self.account) {
            Ok(()) => Ok(()),
            Err(error) if error.code() == NOT_FOUND => Ok(()),
            Err(_) => Err(Error::Persistence),
        }
    }
}

pub struct Storage {
    session: AsyncMutex<Option<HumanSession>>,
    secret: Arc<dyn Secrets>,
    settings: Mutex<Settings>,
    path: PathBuf,
    _lease: File,
}
impl Storage {
    #[cfg(test)]
    pub fn fixture(dir: PathBuf) -> Arc<Self> {
        Self::open(dir, Arc::new(tests::MemorySecrets::default()))
            .unwrap()
            .0
    }
    /// Runs off the UI thread: Keychain may ask the user for access.
    pub fn open_default() -> Result<(Arc<Self>, bool), Error> {
        let dir = match std::env::var_os("CHATTO_NATIVE_CONFIG_DIR") {
            Some(path) => PathBuf::from(path),
            None => PathBuf::from(std::env::var_os("HOME").ok_or(Error::Persistence)?)
                .join("Library/Application Support/Chatto Native"),
        };
        let secret = Arc::new(Keychain {
            account: dir.to_string_lossy().into_owned(),
        });
        Self::open(dir, secret)
    }
    fn open(dir: PathBuf, secret: Arc<dyn Secrets>) -> Result<(Arc<Self>, bool), Error> {
        if fs::symlink_metadata(&dir).is_ok_and(|m| m.file_type().is_symlink()) {
            return Err(Error::Persistence);
        }
        fs::DirBuilder::new()
            .recursive(true)
            .mode(0o700)
            .create(&dir)
            .map_err(|_| Error::Persistence)?;
        fs::set_permissions(&dir, fs::Permissions::from_mode(0o700))
            .map_err(|_| Error::Persistence)?;
        let lock_path = dir.join("running.lock");
        if fs::symlink_metadata(&lock_path).is_ok_and(|m| m.file_type().is_symlink()) {
            return Err(Error::Persistence);
        }
        let lease = OpenOptions::new()
            .create(true)
            .truncate(false)
            .read(true)
            .write(true)
            .mode(0o600)
            .open(lock_path)
            .map_err(|_| Error::Persistence)?;
        lease.try_lock().map_err(|_| {
            Error::InvalidInput("Another Chatto Native instance is using this settings directory")
        })?;
        let path = dir.join("settings.json");
        let mut settings: Settings = match fs::symlink_metadata(&path) {
            Ok(meta) if meta.file_type().is_symlink() || meta.len() > 1024 * 1024 => {
                return Err(Error::Persistence);
            }
            Ok(_) => serde_json::from_slice(&fs::read(&path).map_err(|_| Error::Persistence)?)
                .map_err(|_| Error::Persistence)?,
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Settings::default(),
            Err(_) => return Err(Error::Persistence),
        };
        let session = match secret.read()? {
            Some(bytes) if bytes.len() <= 64 * 1024 => {
                let saved: SessionCredentials =
                    serde_json::from_slice(&bytes).map_err(|_| Error::Persistence)?;
                if settings.origin != saved.origin {
                    settings = Settings {
                        origin: saved.origin.clone(),
                        ..Default::default()
                    };
                }
                Some(HumanSession::restore(saved).map_err(|_| Error::Persistence)?)
            }
            Some(_) => return Err(Error::Persistence),
            None => None,
        };
        let restored = session.is_some();
        Ok((
            Arc::new(Self {
                session: AsyncMutex::new(session),
                secret,
                settings: Mutex::new(settings),
                path,
                _lease: lease,
            }),
            restored,
        ))
    }
    pub async fn has_session(&self) -> bool {
        self.session.lock().await.is_some()
    }
    pub fn settings(&self) -> Settings {
        self.settings.lock().expect("settings lock").clone()
    }
    pub fn update_settings(&self, change: impl FnOnce(&mut Settings)) -> Result<(), Error> {
        let mut settings = self.settings.lock().map_err(|_| Error::Persistence)?;
        let mut next = settings.clone();
        change(&mut next);
        atomic_write(
            &self.path,
            &serde_json::to_vec(&next).map_err(|_| Error::Persistence)?,
        )?;
        *settings = next;
        Ok(())
    }
    async fn save(&self, session: &HumanSession) -> Result<(), Error> {
        let bytes = serde_json::to_vec(&session.credentials()).map_err(|_| Error::Persistence)?;
        let secret = self.secret.clone();
        tokio::task::spawn_blocking(move || secret.write(&bytes))
            .await
            .map_err(|_| Error::Persistence)?
    }
    pub async fn install(&self, session: HumanSession) -> Result<(), Error> {
        let mut current = self.session.lock().await;
        let origin = session.credentials().origin;
        self.update_settings(|settings| {
            *settings = Settings {
                origin,
                ..Default::default()
            }
        })?;
        self.save(&session).await?;
        *current = Some(session);
        Ok(())
    }
    pub fn provider(self: &Arc<Self>) -> ClientProvider {
        let store = self.clone();
        Arc::new(move |renew| {
            let store = store.clone();
            Box::pin(async move { store.client(renew).await })
        })
    }
    /// Lock across prepare, persist, rotation, and replacement persist. Requests
    /// never receive credentials whose replacement has not reached Keychain.
    pub async fn client(self: &Arc<Self>, force: bool) -> Result<Client, Error> {
        // A dropped resource request must not interrupt a Keychain write or let
        // sign-out race a still-running blocking save. The worker keeps the
        // session lock until rotation and persistence finish.
        let store = self.clone();
        tokio::spawn(async move { store.client_inner(force).await })
            .await
            .map_err(|_| Error::Persistence)?
    }
    async fn client_inner(&self, force: bool) -> Result<Client, Error> {
        let mut current = self.session.lock().await;
        let session = current.as_mut().ok_or(Error::Http(401))?;
        let now = SystemTime::now();
        if now >= session.session_expires_at {
            return Err(Error::Http(401));
        }
        if force
            || !session.credentials().refresh_request_id.is_empty()
            || session
                .access_expires_at
                .duration_since(now)
                .unwrap_or_default()
                < Duration::from_secs(60)
        {
            session.prepare_refresh();
            self.save(session).await?;
            let fresh = session.refresh().await?;
            self.save(&fresh).await?;
            *session = fresh;
        }
        Ok(session.client())
    }
    /// Forget local credentials first. A failed revoke never restores them.
    pub async fn sign_out(&self) -> Result<(), Error> {
        let mut current = self.session.lock().await;
        let secret = self.secret.clone();
        tokio::task::spawn_blocking(move || secret.delete())
            .await
            .map_err(|_| Error::Persistence)??;
        let session = current.take();
        self.update_settings(|settings| {
            settings.selected = None;
            settings.collapsed.clear();
        })?;
        if let Some(session) = session {
            session.logout().await?;
        }
        Ok(())
    }
    pub async fn forget(&self) -> Result<(), Error> {
        let mut current = self.session.lock().await;
        let secret = self.secret.clone();
        tokio::task::spawn_blocking(move || secret.delete())
            .await
            .map_err(|_| Error::Persistence)??;
        *current = None;
        self.update_settings(|settings| {
            settings.selected = None;
            settings.collapsed.clear();
        })
    }
}

fn atomic_write(path: &Path, bytes: &[u8]) -> Result<(), Error> {
    let parent = path.parent().ok_or(Error::Persistence)?;
    let temporary = parent.join(format!(".settings-{}", uuid::Uuid::new_v4()));
    let result = (|| {
        let mut file = OpenOptions::new()
            .write(true)
            .create_new(true)
            .mode(0o600)
            .open(&temporary)?;
        file.write_all(bytes)?;
        file.sync_all()?;
        fs::rename(&temporary, path)?;
        File::open(parent)?.sync_all()
    })();
    if result.is_err() {
        let _ = fs::remove_file(temporary);
    }
    result.map_err(|_| Error::Persistence)
}

#[cfg(test)]
mod tests {
    use super::*;
    #[derive(Default)]
    pub(super) struct MemorySecrets {
        data: Mutex<Option<Vec<u8>>>,
        fail_fresh: std::sync::atomic::AtomicBool,
    }
    impl Secrets for MemorySecrets {
        fn read(&self) -> Result<Option<Vec<u8>>, Error> {
            Ok(self.data.lock().unwrap().clone())
        }
        fn write(&self, bytes: &[u8]) -> Result<(), Error> {
            let value: SessionCredentials = serde_json::from_slice(bytes).unwrap();
            if value.access_token == "synthetic-new-access"
                && self
                    .fail_fresh
                    .swap(false, std::sync::atomic::Ordering::SeqCst)
            {
                return Err(Error::Persistence);
            }
            *self.data.lock().unwrap() = Some(bytes.to_vec());
            Ok(())
        }
        fn delete(&self) -> Result<(), Error> {
            *self.data.lock().unwrap() = None;
            Ok(())
        }
    }
    #[test]
    fn settings_are_private_atomic_and_exclusive() {
        let dir = PathBuf::from("../../.context")
            .join(format!("native-storage-{}", uuid::Uuid::new_v4()));
        let secret = Arc::new(MemorySecrets::default());
        let (store, _) = Storage::open(dir.clone(), secret.clone()).unwrap();
        store
            .update_settings(|settings| {
                settings.origin = "https://example.test".into();
                settings.selected = Some("room".into());
                settings.collapsed.insert("dms".into());
            })
            .unwrap();
        assert!(Storage::open(dir.clone(), secret.clone()).is_err());
        assert_eq!(
            fs::metadata(dir.join("settings.json"))
                .unwrap()
                .permissions()
                .mode()
                & 0o777,
            0o600
        );
        assert_eq!(
            fs::metadata(&dir).unwrap().permissions().mode() & 0o777,
            0o700
        );
        drop(store);
        let (restored, has_session) = Storage::open(dir.clone(), secret).unwrap();
        assert!(!has_session);
        assert_eq!(restored.settings().selected.as_deref(), Some("room"));
        assert!(restored.settings().collapsed.contains("dms"));
        drop(restored);
        fs::remove_dir_all(dir).unwrap();
    }
    #[tokio::test]
    async fn refresh_is_persisted_before_send_and_reuses_identity_after_save_failure() {
        use std::{io::Read, net::TcpListener, thread, time::Instant};
        let dir = PathBuf::from("../../.context")
            .join(format!("native-refresh-{}", uuid::Uuid::new_v4()));
        let secret = Arc::new(MemorySecrets::default());
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let origin = format!("http://{}", listener.local_addr().unwrap());
        listener.set_nonblocking(true).unwrap();
        let peer_secret = secret.clone();
        let (requested, received) = tokio::sync::oneshot::channel();
        let (release, gate) = std::sync::mpsc::channel();
        let peer = thread::spawn(move || {
            let mut requested = Some(requested);
            let mut identities = Vec::new();
            for index in 0..2 {
                let start = Instant::now();
                let mut stream = loop {
                    match listener.accept() {
                        Ok((stream, _)) => break stream,
                        Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                            assert!(start.elapsed() < Duration::from_secs(5));
                            thread::sleep(Duration::from_millis(5));
                        }
                        Err(_) => panic!("test listener failed"),
                    }
                };
                stream.set_nonblocking(false).unwrap();
                stream
                    .set_read_timeout(Some(Duration::from_secs(5)))
                    .unwrap();
                let mut bytes = Vec::new();
                let input: serde_json::Value = loop {
                    let mut buffer = [0; 4096];
                    let count = stream.read(&mut buffer).unwrap();
                    assert!(count > 0);
                    bytes.extend_from_slice(&buffer[..count]);
                    if let Some(end) = bytes.windows(4).position(|part| part == b"\r\n\r\n") {
                        let header = String::from_utf8_lossy(&bytes[..end]);
                        assert!(header.starts_with("POST /oauth/token "));
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
                            break serde_json::from_slice(&bytes[end + 4..end + 4 + length])
                                .unwrap();
                        }
                    }
                };
                let prepared: SessionCredentials =
                    serde_json::from_slice(&peer_secret.read().unwrap().unwrap()).unwrap();
                assert!(!prepared.refresh_request_id.is_empty());
                assert_eq!(input["refresh_request_id"], prepared.refresh_request_id);
                identities.push(prepared.refresh_request_id);
                if index == 1 {
                    requested.take().unwrap().send(()).unwrap();
                    gate.recv_timeout(Duration::from_secs(5)).unwrap();
                }
                let body = r#"{"access_token":"synthetic-new-access","refresh_token":"synthetic-new-refresh","expires_in":900,"refresh_token_expires_in":3600}"#;
                write!(
                    stream,
                    "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                    body.len(),
                    body
                )
                .unwrap();
            }
            assert_eq!(identities[0], identities[1]);
        });
        let (store, _) = Storage::open(dir.clone(), secret.clone()).unwrap();
        store
            .install(
                HumanSession::restore(SessionCredentials {
                    origin,
                    access_token: "synthetic-old-access".into(),
                    refresh_token: "synthetic-old-refresh".into(),
                    refresh_request_id: String::new(),
                    access_expires_at: SystemTime::now(),
                    session_expires_at: SystemTime::now() + Duration::from_secs(3600),
                })
                .unwrap(),
            )
            .await
            .unwrap();
        secret
            .fail_fresh
            .store(true, std::sync::atomic::Ordering::SeqCst);
        assert!(matches!(store.client(false).await, Err(Error::Persistence)));
        let durable: SessionCredentials =
            serde_json::from_slice(&secret.read().unwrap().unwrap()).unwrap();
        assert_eq!(durable.access_token, "synthetic-old-access");
        assert!(!durable.refresh_request_id.is_empty());
        let waiting = {
            let store = store.clone();
            tokio::spawn(async move { store.client(false).await })
        };
        received.await.unwrap();
        waiting.abort();
        // Cancellation of a reader leaves its rotation worker serialized. A new
        // reader waits for the durable replacement instead of rotating again.
        release.send(()).unwrap();
        store.client(false).await.unwrap();
        let durable: SessionCredentials =
            serde_json::from_slice(&secret.read().unwrap().unwrap()).unwrap();
        assert_eq!(durable.access_token, "synthetic-new-access");
        assert!(durable.refresh_request_id.is_empty());
        assert!(
            !fs::read_to_string(dir.join("settings.json"))
                .unwrap()
                .contains("synthetic-new-access")
        );
        peer.join().unwrap();
        drop(store);
        let (restored, has_session) = Storage::open(dir.clone(), secret).unwrap();
        assert!(has_session);
        assert!(restored.client(false).await.is_ok());
        drop(restored);
        fs::remove_dir_all(dir).unwrap();
    }
}
