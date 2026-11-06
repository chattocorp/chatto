//! Private server catalogue. Credentials stay in independent Keychain items.
//! The root lease and per-server leases prevent concurrent token rotations.
use crate::storage::{Storage, StoreOpener, atomic_write};
use chatto::{Client, Error, HumanSession};
use serde::{Deserialize, Serialize};
use std::{
    collections::{BTreeMap, BTreeSet},
    fs::{self, File, OpenOptions},
    os::unix::fs::{DirBuilderExt, OpenOptionsExt, PermissionsExt},
    path::PathBuf,
    sync::{Arc, Mutex},
};

#[derive(Clone, Serialize, Deserialize)]
pub struct Registration {
    pub id: String,
    pub origin: String,
    pub name: String,
}
#[derive(Clone, Serialize, Deserialize)]
#[serde(default)]
pub struct Catalogue {
    pub version: u32,
    pub servers: Vec<Registration>,
    pub active: Option<String>,
    pub ui_scale: f32,
}
impl Default for Catalogue {
    fn default() -> Self {
        Self {
            version: 1,
            servers: vec![],
            active: None,
            ui_scale: 1.,
        }
    }
}
pub struct Registry {
    directory: PathBuf,
    opener: StoreOpener,
    catalogue: Mutex<Catalogue>,
    stores: Mutex<BTreeMap<String, Arc<Storage>>>,
    mutation: tokio::sync::Mutex<()>,
    _lease: File,
}
impl Registry {
    /// Open and lock the catalogue off the UI thread. Existing single-server
    /// state is adopted in place, so migration never copies or deletes tokens.
    pub fn open_default() -> Result<Arc<Self>, Error> {
        Self::open_with(Storage::default_dir()?, Arc::new(Storage::open_at))
    }
    pub(crate) fn open_with(directory: PathBuf, opener: StoreOpener) -> Result<Arc<Self>, Error> {
        if fs::symlink_metadata(&directory).is_ok_and(|meta| meta.file_type().is_symlink()) {
            return Err(Error::Persistence);
        }
        fs::DirBuilder::new()
            .recursive(true)
            .mode(0o700)
            .create(&directory)
            .map_err(|_| Error::Persistence)?;
        fs::set_permissions(&directory, fs::Permissions::from_mode(0o700))
            .map_err(|_| Error::Persistence)?;
        let legacy_directory = directory.clone();
        let directory = directory.canonicalize().map_err(|_| Error::Persistence)?;
        let lock = directory.join("catalogue.lock");
        if fs::symlink_metadata(&lock).is_ok_and(|meta| meta.file_type().is_symlink()) {
            return Err(Error::Persistence);
        }
        let lease = OpenOptions::new()
            .create(true)
            .truncate(false)
            .read(true)
            .write(true)
            .mode(0o600)
            .open(lock)
            .map_err(|_| Error::Persistence)?;
        lease.try_lock().map_err(|_| {
            Error::InvalidInput("Another Chatto Native instance is using this settings directory")
        })?;
        let path = directory.join("catalogue.json");
        let catalogue_existed = path.exists();
        let mut catalogue: Catalogue = match fs::symlink_metadata(&path) {
            Ok(meta) if meta.file_type().is_symlink() || meta.len() > 1024 * 1024 => {
                return Err(Error::Persistence);
            }
            Ok(_) => serde_json::from_slice(&fs::read(&path).map_err(|_| Error::Persistence)?)
                .map_err(|_| Error::Persistence)?,
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Catalogue::default(),
            Err(_) => return Err(Error::Persistence),
        };
        if catalogue.version != 1 || catalogue.servers.len() > 32 {
            return Err(Error::Persistence);
        }
        let mut ids = BTreeSet::new();
        let mut origins = BTreeSet::new();
        for server in &catalogue.servers {
            if (server.id != "legacy" && uuid::Uuid::parse_str(&server.id).is_err())
                || !ids.insert(&server.id)
                || !origins.insert(normalize_origin(&server.origin)?)
            {
                return Err(Error::Persistence);
            }
        }
        let servers_directory = directory.join("servers");
        if fs::symlink_metadata(&servers_directory).is_ok_and(|meta| meta.file_type().is_symlink())
        {
            return Err(Error::Persistence);
        }
        fs::DirBuilder::new()
            .recursive(true)
            .mode(0o700)
            .create(&servers_directory)
            .map_err(|_| Error::Persistence)?;
        let mut stores = BTreeMap::new();
        if !catalogue_existed
            && catalogue.servers.is_empty()
            && directory.join("settings.json").exists()
        {
            let (store, _) = opener(legacy_directory.clone())?;
            let settings = store.settings();
            if !settings.origin.is_empty() {
                let origin = normalize_origin(&settings.origin)?;
                catalogue.servers.push(Registration {
                    id: "legacy".into(),
                    name: url::Url::parse(&origin)
                        .ok()
                        .and_then(|url| url.host_str().map(str::to_owned))
                        .unwrap_or_else(|| "Chatto".into()),
                    origin,
                });
                catalogue.active = Some("legacy".into());
                catalogue.ui_scale = crate::ui::normalize_scale(settings.ui_scale.unwrap_or(1.));
                stores.insert("legacy".into(), store);
            }
        }
        for server in &catalogue.servers {
            if stores.contains_key(&server.id) {
                continue;
            }
            let dir = if server.id == "legacy" {
                legacy_directory.clone()
            } else {
                directory.join("servers").join(&server.id)
            };
            let (store, _) = opener(dir)?;
            if !store.settings().origin.is_empty()
                && normalize_origin(&store.settings().origin)? != normalize_origin(&server.origin)?
            {
                return Err(Error::Persistence);
            }
            stores.insert(server.id.clone(), store);
        }
        if !catalogue
            .servers
            .iter()
            .any(|server| Some(&server.id) == catalogue.active.as_ref())
        {
            catalogue.active = catalogue.servers.first().map(|server| server.id.clone());
        }
        atomic_write(
            &path,
            &serde_json::to_vec(&catalogue).map_err(|_| Error::Persistence)?,
        )?;
        Ok(Arc::new(Self {
            directory,
            opener,
            catalogue: Mutex::new(catalogue),
            stores: Mutex::new(stores),
            mutation: tokio::sync::Mutex::new(()),
            _lease: lease,
        }))
    }
    pub fn catalogue(&self) -> Catalogue {
        self.catalogue.lock().expect("catalogue lock").clone()
    }
    pub fn store(&self, id: &str) -> Option<Arc<Storage>> {
        self.stores.lock().expect("stores lock").get(id).cloned()
    }
    pub fn update(&self, change: impl FnOnce(&mut Catalogue)) -> Result<(), Error> {
        let mut current = self.catalogue.lock().map_err(|_| Error::Persistence)?;
        let mut next = current.clone();
        change(&mut next);
        atomic_write(
            &self.directory.join("catalogue.json"),
            &serde_json::to_vec(&next).map_err(|_| Error::Persistence)?,
        )?;
        *current = next;
        Ok(())
    }
    /// Save credentials before registering a server. A failed catalogue write
    /// cannot leave a newly issued session usable only in process memory.
    pub async fn install(&self, session: HumanSession, name: String) -> Result<String, Error> {
        let rollback = session.clone();
        let result = self.install_inner(session, name).await;
        if result.is_err() {
            let _ = rollback.logout().await;
        }
        result
    }
    async fn install_inner(&self, session: HumanSession, name: String) -> Result<String, Error> {
        let _guard = self.mutation.lock().await;
        let origin = normalize_origin(&session.credentials().origin)?;
        let existing = self
            .catalogue()
            .servers
            .iter()
            .find(|server| server.origin == origin)
            .cloned();
        if existing.is_none() && self.catalogue().servers.len() >= 32 {
            return Err(Error::InvalidInput(
                "The experiment supports up to 32 servers",
            ));
        }
        let id = existing
            .as_ref()
            .map(|server| server.id.clone())
            .unwrap_or_else(|| uuid::Uuid::new_v4().to_string());
        let store = if let Some(store) = self.store(&id) {
            store
        } else {
            let directory = self.directory.join("servers").join(&id);
            let opener = self.opener.clone();
            tokio::task::spawn_blocking(move || opener(directory))
                .await
                .map_err(|_| Error::Persistence)??
                .0
        };
        if let Err(error) = store.install(session.clone()).await {
            let _ = session.logout().await;
            return Err(error);
        }
        if let Err(error) = self.update(|catalogue| {
            if let Some(server) = catalogue.servers.iter_mut().find(|server| server.id == id) {
                server.name = name.clone();
            } else {
                catalogue.servers.push(Registration {
                    id: id.clone(),
                    origin,
                    name,
                });
            }
            catalogue.active = Some(id.clone());
        }) {
            if existing.is_none() {
                let _ = store.sign_out().await;
            }
            return Err(error);
        }
        self.stores
            .lock()
            .map_err(|_| Error::Persistence)?
            .insert(id.clone(), store);
        Ok(id)
    }
    /// Clear only this server's credentials. Return false if server revocation
    /// failed after local deletion; other registrations and sessions remain live.
    pub async fn remove(&self, id: &str) -> Result<bool, Error> {
        let _guard = self.mutation.lock().await;
        let store = self.store(id).ok_or(Error::Persistence)?;
        let revoked = store.sign_out().await.is_ok();
        if store.has_session().await {
            return Err(Error::Persistence);
        }
        self.update(|catalogue| {
            catalogue.servers.retain(|server| server.id != id);
            if catalogue.active.as_deref() == Some(id) {
                catalogue.active = catalogue.servers.first().map(|server| server.id.clone());
            }
        })?;
        self.stores
            .lock()
            .map_err(|_| Error::Persistence)?
            .remove(id);
        Ok(revoked)
    }
    pub fn move_server(&self, id: &str, direction: isize) -> Result<(), Error> {
        self.update(|catalogue| {
            if let Some(index) = catalogue.servers.iter().position(|server| server.id == id) {
                let target = index
                    .saturating_add_signed(direction)
                    .min(catalogue.servers.len().saturating_sub(1));
                catalogue.servers.swap(index, target);
            }
        })
    }
}
fn normalize_origin(value: &str) -> Result<String, Error> {
    Client::new(value, "validation")?;
    Ok(url::Url::parse(value)
        .map_err(|_| Error::Persistence)?
        .origin()
        .ascii_serialization())
}

#[cfg(test)]
mod tests {
    use super::*;
    use chatto::SessionCredentials;
    use std::time::{Duration, SystemTime};
    fn session(origin: &str) -> HumanSession {
        HumanSession::restore(SessionCredentials {
            origin: origin.into(),
            access_token: "synthetic-registry-access".into(),
            refresh_token: "synthetic-registry-refresh".into(),
            refresh_request_id: String::new(),
            oauth_client_id: chatto::oauth::DESKTOP_CLIENT_ID.into(),
            access_expires_at: SystemTime::now() + Duration::from_secs(600),
            session_expires_at: SystemTime::now() + Duration::from_secs(3600),
        })
        .unwrap()
    }
    #[tokio::test]
    async fn registry_restores_separate_sessions_and_never_remigrates_a_removed_legacy_server() {
        let directory = std::env::current_dir()
            .unwrap()
            .join("../../.context")
            .join(format!("native-registry-{}", uuid::Uuid::new_v4()));
        let opener = Storage::test_opener();
        let (legacy, _) = opener(directory.clone()).unwrap();
        legacy.install(session("http://127.0.0.1:1")).await.unwrap();
        legacy
            .update_settings(|settings| {
                settings.selected = Some("legacy-room".into());
                settings.ui_scale = Some(1.37);
            })
            .unwrap();
        drop(legacy);
        let registry = Registry::open_with(directory.clone(), opener.clone()).unwrap();
        assert!(
            Registry::open_with(directory.clone(), opener.clone()).is_err(),
            "catalogue access must be exclusive"
        );
        assert_eq!(registry.catalogue().active.as_deref(), Some("legacy"));
        assert_eq!(registry.catalogue().ui_scale, 1.37);
        assert!(registry.store("legacy").unwrap().has_session().await);
        assert_eq!(
            registry
                .store("legacy")
                .unwrap()
                .settings()
                .selected
                .as_deref(),
            Some("legacy-room")
        );
        let second = registry
            .install(session("http://127.0.0.1:2"), "Second server".into())
            .await
            .unwrap();
        registry
            .store(&second)
            .unwrap()
            .update_settings(|settings| settings.selected = Some("second-room".into()))
            .unwrap();
        registry
            .update(|catalogue| catalogue.active = Some("legacy".into()))
            .unwrap();
        registry.move_server(&second, -1).unwrap();
        let bytes = fs::read(directory.join("catalogue.json")).unwrap();
        assert!(!String::from_utf8_lossy(&bytes).contains("synthetic-registry"));
        assert_eq!(
            fs::metadata(directory.join("catalogue.json"))
                .unwrap()
                .permissions()
                .mode()
                & 0o777,
            0o600
        );
        drop(registry);
        let registry = Registry::open_with(directory.clone(), opener.clone()).unwrap();
        assert_eq!(registry.catalogue().servers[0].id, second);
        assert_eq!(registry.catalogue().active.as_deref(), Some("legacy"));
        assert_eq!(
            registry
                .store(&second)
                .unwrap()
                .settings()
                .selected
                .as_deref(),
            Some("second-room")
        );
        assert_eq!(
            registry
                .store(&second)
                .unwrap()
                .client(false)
                .await
                .unwrap()
                .attachment_urls("", &[])
                .await,
            Err(Error::InvalidInput(
                "A room and one to 32 asset IDs are required"
            ))
        );
        assert_eq!(
            registry
                .install(session("http://127.0.0.1:2"), "Second server again".into())
                .await
                .unwrap(),
            second,
            "a server origin must not be registered twice"
        );
        assert!(
            !registry.remove("legacy").await.unwrap(),
            "local deletion succeeds even if the offline server cannot revoke"
        );
        assert!(registry.store(&second).unwrap().has_session().await);
        registry.remove(&second).await.unwrap();
        drop(registry);
        let registry = Registry::open_with(directory.clone(), opener).unwrap();
        assert!(
            registry.catalogue().servers.is_empty(),
            "removed legacy settings must not be imported again"
        );
        drop(registry);
        fs::remove_dir_all(directory).unwrap();
    }
}
