//! System-browser sign-in. The callback is relayed in an HTTP body; no auth code
//! or token enters a command argument, a temporary file, or a diagnostic.
use axum::{
    Router,
    extract::{DefaultBodyLimit, State},
    http::{HeaderMap, StatusCode},
    routing::post,
};
use chatto::{
    Error, HumanSession,
    oauth::{self, Flow},
};
use std::{
    fs,
    io::Read,
    os::unix::fs::{DirBuilderExt, OpenOptionsExt},
    path::{Path, PathBuf},
    process::{Command, Stdio},
    sync::{Arc, Mutex},
    time::{Duration, Instant},
};
use tokio::{net::TcpListener, sync::oneshot};

static ATTEMPT: tokio::sync::Mutex<()> = tokio::sync::Mutex::const_new(());

const LSREGISTER: &str = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister";
const HANDLER: &str = r#"ObjC.import('CoreServices'); function run(args) {
if (args[0] === 'set') { if ($.LSSetDefaultHandlerForURLScheme($('chatto'), $(args[1])) !== 0) throw Error('URL handler failed'); return ''; }
const value = $.LSCopyDefaultHandlerForURLScheme($('chatto')); return value ? ObjC.unwrap(ObjC.castRefToObject(value)) : '';
}"#;
const STOP: &str = "ObjC.import('AppKit'); function run(args) { const apps = $.NSRunningApplication.runningApplicationsWithBundleIdentifier($(args[0])); for (let i = 0; i < apps.count; i++) apps.objectAtIndex(i).forceTerminate; }";

/// Bounded OS calls. Their stderr can contain URLs, so it is never forwarded.
fn command(program: &str, args: &[&str]) -> Result<String, Error> {
    let mut child = Command::new(program)
        .args(args)
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .spawn()
        .map_err(|_| Error::InvalidInput("Could not open browser sign-in"))?;
    let start = Instant::now();
    loop {
        if let Some(status) = child.try_wait().map_err(|_| Error::Transport)? {
            if !status.success() {
                return Err(Error::InvalidInput("Could not open browser sign-in"));
            }
            let mut output = String::new();
            child
                .stdout
                .take()
                .ok_or(Error::Transport)?
                .take(4096)
                .read_to_string(&mut output)
                .map_err(|_| Error::Transport)?;
            return Ok(output.trim().to_owned());
        }
        if start.elapsed() > Duration::from_secs(20) {
            let _ = child.kill();
            let _ = child.wait();
            return Err(Error::Transport);
        }
        std::thread::sleep(Duration::from_millis(20));
    }
}
fn handler(operation: &str, id: &str) -> Result<String, Error> {
    command(
        "/usr/bin/osascript",
        &["-l", "JavaScript", "-e", HANDLER, operation, id],
    )
}

/// A temporary LaunchServices handler, with ownership-based cleanup on cancel.
struct Relay {
    _attempt: tokio::sync::MutexGuard<'static, ()>,
    directory: PathBuf,
    app: PathBuf,
    id: String,
    previous: String,
    registered: bool,
}
impl Relay {
    fn register(
        endpoint: &str,
        attempt: tokio::sync::MutexGuard<'static, ()>,
    ) -> Result<Self, Error> {
        let previous = handler("read", "")?;
        let cache = PathBuf::from(std::env::var_os("HOME").ok_or(Error::Persistence)?)
            .join("Library/Caches/Chatto Native");
        fs::DirBuilder::new()
            .recursive(true)
            .mode(0o700)
            .create(&cache)
            .map_err(|_| Error::Persistence)?;
        let directory = cache.join(format!("oauth-relay-{}", uuid::Uuid::new_v4()));
        fs::DirBuilder::new()
            .mode(0o700)
            .create(&directory)
            .map_err(|_| Error::Persistence)?;
        let app = directory.join("Chatto Native Sign-in.app");
        let mut relay = Self {
            _attempt: attempt,
            directory,
            app,
            id: format!("run.chatto.native.relay.{}", uuid::Uuid::new_v4()),
            previous,
            registered: false,
        };
        relay.install(endpoint)?;
        Ok(relay)
    }
    fn install(&mut self, endpoint: &str) -> Result<(), Error> {
        use std::io::Write;
        let source = self.directory.join("relay.applescript");
        let script = format!(
            r#"use framework "Foundation"
use scripting additions
on open location incoming
set targetURL to current application's NSURL's URLWithString:{endpoint:?}
set request to current application's NSMutableURLRequest's requestWithURL:targetURL
request's setHTTPMethod:"POST"
request's setTimeoutInterval:5
set bodyString to current application's NSString's stringWithString:incoming
request's setHTTPBody:(bodyString's dataUsingEncoding:(current application's NSUTF8StringEncoding))
current application's NSURLConnection's sendSynchronousRequest:request returningResponse:(missing value) |error|:(missing value)
end open location
"#
        );
        fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .mode(0o600)
            .open(&source)
            .and_then(|mut file| file.write_all(script.as_bytes()))
            .map_err(|_| Error::Persistence)?;
        let app = path(&self.app)?;
        command("/usr/bin/osacompile", &["-s", "-o", app, path(&source)?])?;
        let plist = self.app.join("Contents/Info.plist");
        let plist = path(&plist)?;
        command(
            "/usr/bin/plutil",
            &["-replace", "CFBundleIdentifier", "-string", &self.id, plist],
        )?;
        command(
            "/usr/bin/plutil",
            &["-insert", "LSUIElement", "-bool", "YES", plist],
        )?;
        let types =
            serde_json::json!([{"CFBundleURLName":self.id,"CFBundleURLSchemes":["chatto"]}])
                .to_string();
        command(
            "/usr/bin/plutil",
            &["-insert", "CFBundleURLTypes", "-json", &types, plist],
        )?;
        command(
            "/usr/bin/plutil",
            &[
                "-insert",
                "NSAppTransportSecurity",
                "-json",
                "{\"NSAllowsArbitraryLoads\":true}",
                plist,
            ],
        )?;
        command("/usr/bin/codesign", &["--force", "--sign", "-", app])?;
        command(LSREGISTER, &["-f", app])?;
        self.registered = true;
        command("/usr/bin/open", &["-g", "-a", app])?;
        handler("set", &self.id)?;
        Ok(())
    }
}
impl Drop for Relay {
    fn drop(&mut self) {
        if self.registered {
            if handler("read", "").ok().as_deref() == Some(&self.id) && !self.previous.is_empty() {
                let _ = handler("set", &self.previous);
            }
            if let Ok(app) = path(&self.app) {
                let _ = command(LSREGISTER, &["-u", app]);
            }
            let _ = command(
                "/usr/bin/osascript",
                &["-l", "JavaScript", "-e", STOP, &self.id],
            );
        }
        let _ = fs::remove_dir_all(&self.directory);
    }
}
fn path(path: &Path) -> Result<&str, Error> {
    path.to_str().ok_or(Error::Persistence)
}
#[derive(Clone)]
struct Callback {
    validate: Arc<dyn Fn(&str) -> bool + Send + Sync>,
    host: String,
    sender: Arc<Mutex<Option<oneshot::Sender<String>>>>,
}
async fn receive(
    State(state): State<Callback>,
    headers: HeaderMap,
    body: String,
) -> (StatusCode, [(&'static str, &'static str); 3], &'static str) {
    let accepted = headers.get("host").and_then(|v| v.to_str().ok()) == Some(state.host.as_str())
        && (state.validate)(&body);
    let sender = if accepted {
        state
            .sender
            .lock()
            .ok()
            .and_then(|mut sender| sender.take())
    } else {
        None
    };
    let success = sender.is_some_and(|sender| sender.send(body).is_ok());
    (
        if success {
            StatusCode::OK
        } else {
            StatusCode::BAD_REQUEST
        },
        [
            ("Cache-Control", "no-store"),
            ("Referrer-Policy", "no-referrer"),
            (
                "Content-Security-Policy",
                "default-src 'none'; frame-ancestors 'none'",
            ),
        ],
        if success {
            "Sign-in received. Return to Chatto Native."
        } else {
            "Invalid sign-in callback"
        },
    )
}
struct ListenerTask(tokio::task::JoinHandle<()>);
impl Drop for ListenerTask {
    fn drop(&mut self) {
        self.0.abort();
    }
}

/// Use server-owned discovery and consent. Cancellation drops the callback
/// listener and restores the previous chatto: handler, including on timeout.
pub async fn sign_in(origin: &str) -> Result<(HumanSession, String), Error> {
    sign_in_with(origin, |authorization| async move {
        tokio::task::spawn_blocking(move || command("/usr/bin/open", &[authorization.as_str()]))
            .await
            .map_err(|_| Error::Transport)??;
        Ok(())
    })
    .await
}

pub(crate) async fn sign_in_with<F, Fut>(
    origin: &str,
    open: F,
) -> Result<(HumanSession, String), Error>
where
    F: FnOnce(url::Url) -> Fut,
    Fut: std::future::Future<Output = Result<(), Error>>,
{
    let attempt = ATTEMPT.lock().await;
    let info = oauth::discover(origin).await?;
    let listener = TcpListener::bind("127.0.0.1:0")
        .await
        .map_err(|_| Error::Transport)?;
    let address = listener.local_addr().map_err(|_| Error::Transport)?;
    let endpoint = format!("http://{address}/servers/callback?mode=popup");
    let _relay = tokio::task::spawn_blocking(move || Relay::register(&endpoint, attempt))
        .await
        .map_err(|_| Error::Transport)??;
    let flow = Flow::new(
        origin,
        &info.login.authorize_url,
        oauth::DESKTOP_CLIENT_ID,
        oauth::DESKTOP_CALLBACK,
    )?;
    let (sender, receiver) = oneshot::channel();
    let state = Callback {
        validate: Arc::new(flow.callback_validator()),
        host: address.to_string(),
        sender: Arc::new(Mutex::new(Some(sender))),
    };
    let router = Router::new()
        .route("/servers/callback", post(receive))
        .layer(DefaultBodyLimit::max(8192))
        .with_state(state);
    let task = ListenerTask(tokio::spawn(async move {
        let _ = axum::serve(listener, router).await;
    }));
    open(flow.authorization_url().clone()).await?;
    let callback = tokio::time::timeout(Duration::from_secs(300), receiver)
        .await
        .map_err(|_| Error::InvalidInput("Browser sign-in timed out"))?
        .map_err(|_| Error::Transport)?;
    task.0.abort();
    // The listener has only routing/state validation, never the PKCE verifier.
    let mut task = task;
    let _ = (&mut task.0).await;
    let session = flow.exchange(&callback).await?;
    Ok((session, info.profile.name))
}

/// Wait for any registered handler to be restored before the runtime exits.
pub async fn drain() {
    let _guard = ATTEMPT.lock().await;
}

#[cfg(test)]
mod tests {
    use super::*;
    use axum::{Json, routing::get};
    use serde_json::{Value, json};
    use std::io::Write;
    fn deliver_callback(callback: &str) -> Result<(), Error> {
        // Even synthetic auth codes travel on stdin, not in process arguments.
        let script = r#"ObjC.import('Foundation'); ObjC.import('AppKit');
const data = $.NSFileHandle.fileHandleWithStandardInput.readDataToEndOfFile;
const text = $.NSString.alloc.initWithDataEncoding(data, $.NSUTF8StringEncoding).js;
const value = JSON.parse(text);
if (!$.NSWorkspace.sharedWorkspace.openURL($.NSURL.URLWithString($(value.url)))) throw Error('Callback delivery failed');"#;
        let mut child = Command::new("/usr/bin/osascript")
            .args(["-l", "JavaScript", "-e", script])
            .stdin(Stdio::piped())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .spawn()
            .map_err(|_| Error::Transport)?;
        child
            .stdin
            .take()
            .unwrap()
            .write_all(json!({"url":callback}).to_string().as_bytes())
            .map_err(|_| Error::Transport)?;
        if child.wait().map_err(|_| Error::Transport)?.success() {
            Ok(())
        } else {
            Err(Error::Transport)
        }
    }
    #[tokio::test]
    async fn real_desktop_callback_exchanges_code_and_restores_handler_on_success_and_cancellation()
    {
        let previous = handler("read", "").unwrap();
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let origin = format!("http://{}", listener.local_addr().unwrap());
        let router = Router::new()
            .route("/api/connect/chatto.discovery.v1.ServerDiscoveryService/GetServer", post(|| async { Json(json!({"profile":{"name":"Synthetic server"}, "login":{"authorizeUrl":"/authorize"}})) }))
            .route("/authorize", get(|| async { "Synthetic browser fixture" }))
            .route("/oauth/token", post(|Json(body):Json<Value>| async move {
                assert_eq!(body["client_id"], oauth::DESKTOP_CLIENT_ID);
                assert_eq!(body["redirect_uri"], oauth::DESKTOP_CALLBACK);
                assert_eq!(body["code"], "native-fixture-code");
                assert!(body["code_verifier"].as_str().unwrap().len() >= 43);
                Json(json!({"access_token":"native-fixture-access", "refresh_token":"native-fixture-refresh", "expires_in":900, "refresh_token_expires_in":3600, "token_type":"Bearer"}))
            }));
        let server = tokio::spawn(async move {
            axum::serve(listener, router).await.unwrap();
        });
        let result = tokio::time::timeout(
            Duration::from_secs(60),
            sign_in_with(&origin, |authorization| async move {
                let state = authorization
                    .query_pairs()
                    .find(|(key, _)| key == "state")
                    .unwrap()
                    .1
                    .into_owned();
                let mut callback = url::Url::parse(oauth::DESKTOP_CALLBACK).unwrap();
                callback
                    .query_pairs_mut()
                    .extend_pairs([("state", state.as_str()), ("code", "native-fixture-code")]);
                tokio::task::spawn_blocking(move || deliver_callback(callback.as_str()))
                    .await
                    .map_err(|_| Error::Transport)?
            }),
        )
        .await
        .unwrap()
        .unwrap();
        assert_eq!(
            result.0.credentials().oauth_client_id,
            oauth::DESKTOP_CLIENT_ID
        );
        assert_eq!(result.1, "Synthetic server");
        assert_eq!(
            handler("read", "").unwrap(),
            previous,
            "the previous desktop handler must be restored"
        );
        let result = sign_in_with(&origin, |_| async { Err(Error::Transport) }).await;
        assert!(matches!(result, Err(Error::Transport)));
        assert_eq!(
            handler("read", "").unwrap(),
            previous,
            "opening/cancelling a login must not leave its relay registered"
        );
        server.abort();
    }
}
