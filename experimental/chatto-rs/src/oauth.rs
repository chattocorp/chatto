//! Native browser OAuth with PKCE. Hosts own browser presentation and callbacks.
//! Transaction secrets and callback URLs must never be logged or saved.
use crate::{Client, Error, HumanSession, SessionCredentials};
use base64::{Engine, engine::general_purpose::URL_SAFE_NO_PAD};
use serde::Deserialize;
use serde_json::json;
use sha2::{Digest, Sha256};
use std::time::{Duration, SystemTime};
use url::Url;

pub const DESKTOP_CLIENT_ID: &str = "chatto://desktop";
pub const DESKTOP_CALLBACK: &str = "chatto://desktop/servers/callback?mode=popup";

/// Public discovery fields used before a bearer credential exists.
#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Discovery {
    pub profile: PublicProfile,
    pub login: Login,
}
#[derive(Deserialize)]
pub struct PublicProfile {
    pub name: String,
    #[serde(default, rename = "logoUrl")]
    pub logo_url: String,
}
#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Login {
    pub authorize_url: String,
}

/// Read public discovery without cookies, bearer credentials, or redirects.
pub async fn discover(origin: &str) -> Result<Discovery, Error> {
    let client = Client::new(origin, "discovery")?;
    let info: Discovery = client
        .post(
            "/api/connect/chatto.discovery.v1.ServerDiscoveryService/GetServer",
            json!({}),
            false,
        )
        .await?;
    if info.profile.name.is_empty() || info.login.authorize_url.is_empty() {
        return Err(Error::InvalidResponse);
    }
    Ok(info)
}

/// One in-memory authorization transaction, bound to an origin and exact callback.
/// Debug is intentionally unavailable because the verifier and state are sensitive.
pub struct Flow {
    client: Client,
    client_id: String,
    callback: Url,
    state: String,
    verifier: String,
    authorization: Url,
}
impl Flow {
    /// Create a fresh S256 transaction. Only same-origin authorization is allowed.
    pub fn new(
        origin: &str,
        authorize_url: &str,
        client_id: &str,
        callback: &str,
    ) -> Result<Self, Error> {
        let client = Client::new(origin, "oauth")?;
        let mut authorization = client
            .origin
            .join(authorize_url)
            .map_err(|_| Error::InvalidInput("Invalid authorization endpoint"))?;
        let callback = Url::parse(callback).map_err(|_| Error::InvalidInput("Invalid callback"))?;
        if authorization.origin() != client.origin.origin()
            || !authorization.username().is_empty()
            || authorization.password().is_some()
            || authorization.fragment().is_some()
            || client_id.is_empty()
            || !callback.username().is_empty()
            || callback.password().is_some()
            || callback.fragment().is_some()
            || (callback.as_str() != DESKTOP_CALLBACK
                && !(callback.scheme() == "http"
                    && matches!(callback.host_str(), Some("127.0.0.1" | "[::1]"))))
        {
            return Err(Error::InvalidInput(
                "Invalid native authorization configuration",
            ));
        }
        let random = || {
            URL_SAFE_NO_PAD.encode(
                [
                    uuid::Uuid::new_v4().as_bytes().as_slice(),
                    uuid::Uuid::new_v4().as_bytes().as_slice(),
                ]
                .concat(),
            )
        };
        let state = random();
        let verifier = random();
        let challenge = URL_SAFE_NO_PAD.encode(Sha256::digest(verifier.as_bytes()));
        let reserved = [
            "response_type",
            "client_id",
            "redirect_uri",
            "code_challenge",
            "code_challenge_method",
            "state",
        ];
        let parameters: Vec<_> = authorization
            .query_pairs()
            .filter(|(key, _)| !reserved.contains(&key.as_ref()))
            .map(|(key, value)| (key.into_owned(), value.into_owned()))
            .collect();
        authorization.set_query(None);
        authorization.query_pairs_mut().extend_pairs(parameters);
        authorization.query_pairs_mut().extend_pairs([
            ("response_type", "code"),
            ("client_id", client_id),
            ("redirect_uri", callback.as_str()),
            ("code_challenge", &challenge),
            ("code_challenge_method", "S256"),
            ("state", &state),
        ]);
        Ok(Self {
            client,
            client_id: client_id.into(),
            callback,
            state,
            verifier,
            authorization,
        })
    }
    /// Open this URL in the system browser. Do not print it in diagnostics.
    pub fn authorization_url(&self) -> &Url {
        &self.authorization
    }

    /// Reject duplicate parameters, a wrong route/state, or a different issuer.
    pub fn accepts(&self, value: &str) -> bool {
        matches_callback(&self.client.origin, &self.callback, &self.state, value)
    }

    /// Give a listener routing/state validation without sharing the PKCE verifier.
    /// The validator may outlive the listener's response while exchange consumes
    /// the original flow. It must not be logged or saved.
    pub fn callback_validator(&self) -> impl Fn(&str) -> bool + Send + Sync + 'static + use<> {
        let origin = self.client.origin.clone();
        let callback = self.callback.clone();
        let state = self.state.clone();
        move |value| matches_callback(&origin, &callback, &state, value)
    }
    /// Consume one validated callback and exchange its code, without retries.
    pub async fn exchange(self, callback: &str) -> Result<HumanSession, Error> {
        if !self.accepts(callback) {
            return Err(Error::InvalidInput("Invalid browser sign-in callback"));
        }
        let url = Url::parse(callback).map_err(|_| Error::InvalidResponse)?;
        let code = url
            .query_pairs()
            .find(|(key, _)| key == "code")
            .map(|(_, value)| value.into_owned())
            .ok_or(Error::InvalidInput("Browser sign-in was declined"))?;
        #[derive(Deserialize)]
        struct Response {
            access_token: String,
            refresh_token: String,
            expires_in: u64,
            refresh_token_expires_in: u64,
            token_type: String,
        }
        let response: Response = self.client.post("/oauth/token", json!({"grant_type":"authorization_code", "client_id":self.client_id, "code":code, "code_verifier":self.verifier, "redirect_uri":self.callback.as_str()}), false).await?;
        if !response.token_type.eq_ignore_ascii_case("bearer")
            || response.access_token.is_empty()
            || response.refresh_token.is_empty()
            || !(1..=315_360_000).contains(&response.expires_in)
            || !(1..=315_360_000).contains(&response.refresh_token_expires_in)
        {
            return Err(Error::InvalidResponse);
        }
        let now = SystemTime::now();
        HumanSession::restore(SessionCredentials {
            origin: self.client.origin.to_string(),
            oauth_client_id: self.client_id,
            access_token: response.access_token,
            refresh_token: response.refresh_token,
            refresh_request_id: String::new(),
            access_expires_at: now
                .checked_add(Duration::from_secs(response.expires_in))
                .ok_or(Error::InvalidResponse)?,
            session_expires_at: now
                .checked_add(Duration::from_secs(response.refresh_token_expires_in))
                .ok_or(Error::InvalidResponse)?,
        })
    }
}

fn matches_callback(origin: &Url, callback: &Url, state: &str, value: &str) -> bool {
    let Ok(url) = Url::parse(value) else {
        return false;
    };
    if url.scheme() != callback.scheme()
        || url.host_str() != callback.host_str()
        || url.port() != callback.port()
        || url.path() != callback.path()
        || !url.username().is_empty()
        || url.password().is_some()
        || url.fragment().is_some()
    {
        return false;
    }
    let pairs: Vec<_> = url.query_pairs().collect();
    let get = |key: &str| {
        pairs
            .iter()
            .filter(|(k, _)| k == key)
            .map(|(_, v)| v.as_ref())
            .collect::<Vec<_>>()
    };
    if get("state") != [state]
        || get("code").len() > 1
        || get("error").len() > 1
        || get("iss").len() > 1
    {
        return false;
    }
    if let Some(issuer) = get("iss").first()
        && *issuer != origin.origin().ascii_serialization()
    {
        return false;
    }
    for (key, value) in callback.query_pairs() {
        if get(&key) != [value.as_ref()] {
            return false;
        }
    }
    let code = get("code");
    let error = get("error");
    code.len() == 1 && !code[0].is_empty() && error.is_empty()
        || error.len() == 1 && !error[0].is_empty() && code.is_empty()
}
