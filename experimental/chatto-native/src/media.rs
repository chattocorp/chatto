//! Bounded raster previews for the active authorized room. Image requests never
//! send cookies, bearer credentials, referrers, or follow redirects. Foreign
//! origins require a click that names the contacted host before bytes are read.
use crate::{storage::Storage, ui};
use chatto::{Error, Message};
use gpui::component::button::{Button, ButtonVariants};
use gpui::{
    App, Context, Entity, ImageSource, IntoElement, ObjectFit, RenderImage, SharedString, Task,
    div, img, prelude::*, rgb,
};
use serde_json::Value;
use std::{
    collections::{BTreeMap, BTreeSet},
    io::Cursor,
    sync::Arc,
    time::Duration,
};
use tokio::{runtime::Runtime, sync::Semaphore, task::AbortHandle};
use url::Url;

const MAX_BYTES: usize = 12 * 1024 * 1024;
const MAX_PIXELS: u64 = 8 * 1024 * 1024;
const MAX_CACHE_BYTES: usize = 48 * 1024 * 1024;
static READS: Semaphore = Semaphore::const_new(4);

#[derive(Clone)]
pub struct MediaItem {
    pub key: String,
    pub url: String,
    pub label: String,
    pub asset_id: Option<String>,
    pub expires: Option<String>,
    pub open_url: String,
    pub image: bool,
    width: f32,
    height: f32,
    original: bool,
}
impl MediaItem {
    pub fn inline(url: &str) -> Self {
        Self {
            key: format!("inline:{url}"),
            url: url.into(),
            label: "Image".into(),
            asset_id: None,
            expires: None,
            open_url: url.into(),
            image: true,
            width: 0.,
            height: 0.,
            original: false,
        }
    }
}
struct ReadTask {
    abort: AbortHandle,
    _delivery: Task<()>,
}
impl Drop for ReadTask {
    fn drop(&mut self) {
        self.abort.abort();
    }
}
struct Entry {
    image: Option<Arc<RenderImage>>,
    failed: bool,
    allowed_origin: Option<String>,
    bytes: usize,
    used: u64,
    _read: Option<ReadTask>,
}

/// A cache belongs to one selected server/room authorization generation. Drop
/// it at that boundary to cancel reads and release decoded private media.
pub struct MediaCache {
    runtime: Arc<Runtime>,
    origin: String,
    store: Option<Arc<Storage>>,
    room: String,
    entries: BTreeMap<String, Entry>,
    clock: u64,
    open_errors: BTreeSet<String>,
}
impl MediaCache {
    pub fn new(
        runtime: Arc<Runtime>,
        origin: String,
        store: Option<Arc<Storage>>,
        room: String,
    ) -> Self {
        Self {
            runtime,
            origin,
            store,
            room,
            entries: BTreeMap::new(),
            clock: 0,
            open_errors: BTreeSet::new(),
        }
    }
    pub fn origin(&self) -> &str {
        &self.origin
    }
    pub fn requires_consent(&self, item: &MediaItem) -> bool {
        media_url(&item.url, &self.origin).is_some_and(|url| {
            let origin = url.origin().ascii_serialization();
            origin != self.origin.trim_end_matches('/')
                && !self
                    .entries
                    .get(&item.key)
                    .is_some_and(|entry| entry.allowed_origin.as_deref() == Some(origin.as_str()))
        })
    }
    pub fn allow(&mut self, item: &MediaItem, cx: &mut Context<Self>) {
        self.entries
            .entry(item.key.clone())
            .or_insert(Entry {
                image: None,
                failed: false,
                allowed_origin: Some(self.origin.clone()),
                bytes: 0,
                used: 0,
                _read: None,
            })
            .allowed_origin =
            media_url(&item.url, &self.origin).map(|url| url.origin().ascii_serialization());
        self.get(item, cx);
        cx.notify();
    }
    pub fn failed(&self, key: &str) -> bool {
        self.entries.get(key).is_some_and(|entry| entry.failed)
    }
    pub fn retry(&mut self, key: &str, cx: &mut Context<Self>) {
        if let Some(entry) = self.entries.get_mut(key) {
            entry.failed = false;
            entry._read = None;
        }
        cx.notify();
    }
    fn get(&mut self, item: &MediaItem, cx: &mut Context<Self>) -> Option<Arc<RenderImage>> {
        if self.requires_consent(item) {
            return None;
        }
        media_url(&item.url, &self.origin)?;
        self.clock += 1;
        let entry = self.entries.entry(item.key.clone()).or_insert(Entry {
            image: None,
            failed: false,
            allowed_origin: None,
            bytes: 0,
            used: self.clock,
            _read: None,
        });
        entry.used = self.clock;
        if let Some(image) = &entry.image {
            return Some(image.clone());
        }
        if entry.failed || entry._read.is_some() {
            return None;
        }
        let item = item.clone();
        let key = item.key.clone();
        let origin = self.origin.clone();
        let store = self.store.clone();
        let room = self.room.clone();
        let worker = self.runtime.spawn(async move {
            let _permit = READS.acquire().await.map_err(|_| Error::Transport)?;
            let resolved = resolve(&item, &origin, store.as_ref(), &room, false).await?;
            let result = fetch_image(&resolved).await;
            let bytes = match result {
                Err(Error::Http(401 | 403 | 410)) if item.asset_id.is_some() => {
                    let fresh = resolve(&item, &origin, store.as_ref(), &room, true).await?;
                    fetch_image(&fresh).await?
                }
                result => result?,
            };
            tokio::task::spawn_blocking(move || {
                decode(
                    &bytes,
                    item.key == "server-logo" || item.key.starts_with("avatar:"),
                )
            })
            .await
            .map_err(|_| Error::InvalidResponse)?
        });
        let abort = worker.abort_handle();
        let delivery = cx.spawn(async move |this, cx| {
            let result = worker.await.unwrap_or(Err(Error::Transport));
            let _ = this.update(cx, |this, cx| {
                if let Some(entry) = this.entries.get_mut(&key) {
                    match result {
                        Ok((image, bytes)) => {
                            entry.image = Some(image);
                            entry.bytes = bytes;
                        }
                        Err(_) => entry.failed = true,
                    }
                }
                this.trim(&key);
                cx.notify();
            });
        });
        entry._read = Some(ReadTask {
            abort,
            _delivery: delivery,
        });
        None
    }
    fn trim(&mut self, keep: &str) {
        while self.entries.len() > 48
            || self
                .entries
                .values()
                .map(|entry| entry.bytes)
                .sum::<usize>()
                > MAX_CACHE_BYTES
        {
            let victim = self
                .entries
                .iter()
                .filter(|(key, _)| key.as_str() != keep)
                .min_by_key(|(_, entry)| entry.used)
                .map(|(key, _)| key.clone());
            if let Some(victim) = victim {
                self.entries.remove(&victim);
            } else {
                break;
            }
        }
    }
    pub fn source(cache: &Entity<Self>, item: MediaItem) -> ImageSource {
        let cache = cache.downgrade();
        ImageSource::Custom(Arc::new(move |_, cx| {
            cache
                .update(cx, |cache, cx| cache.get(&item, cx))
                .ok()
                .flatten()
                .map(Ok)
        }))
    }
    /// Resolve signatures again for an explicit open action. No signed URL is saved.
    pub fn open(&mut self, item: MediaItem, cx: &mut Context<Self>) {
        let key = item.key.clone();
        self.open_errors.remove(&key);
        cx.notify();
        let origin = self.origin.clone();
        let store = self.store.clone();
        let room = self.room.clone();
        let worker = self.runtime.spawn(async move {
            let mut full = item;
            full.url = full.open_url.clone();
            full.original = true;
            resolve(
                &full,
                &origin,
                store.as_ref(),
                &room,
                full.asset_id.is_some(),
            )
            .await
        });
        cx.spawn(async move |this, cx| match worker.await {
            Ok(Ok(url)) => cx.update(|cx| cx.open_url(url.as_str())),
            _ => {
                let _ = this.update(cx, |this, cx| {
                    this.open_errors.insert(key);
                    cx.notify();
                });
            }
        })
        .detach();
    }

    /// Synthetic pixels let the offline gallery exercise the real image renderer.
    pub fn seed_preview(&mut self) {
        let image = bundled_logo();
        let size = image.size(0);
        self.entries.insert(
            "attachment:preview-image".into(),
            Entry {
                image: Some(image),
                failed: false,
                allowed_origin: Some(self.origin.clone()),
                bytes: size.width.0 as usize * size.height.0 as usize * 4,
                used: 0,
                _read: None,
            },
        );
    }
}

pub fn media_url(raw: &str, origin: &str) -> Option<Url> {
    let url = Url::parse(origin).ok()?.join(raw).ok()?;
    if !url.username().is_empty() || url.password().is_some() || url.fragment().is_some() {
        return None;
    }
    let loopback = matches!(url.host_str(), Some("localhost" | "127.0.0.1" | "[::1]"));
    (url.scheme() == "https" || url.scheme() == "http" && loopback).then_some(url)
}
fn expired(item: &MediaItem) -> bool {
    item.expires
        .as_deref()
        .and_then(|value| chrono::DateTime::parse_from_rfc3339(value).ok())
        .is_some_and(|expiry| expiry.timestamp() < chrono::Utc::now().timestamp() + 30)
}
async fn resolve(
    item: &MediaItem,
    origin: &str,
    store: Option<&Arc<Storage>>,
    room: &str,
    force: bool,
) -> Result<Url, Error> {
    let original =
        media_url(&item.url, origin).ok_or(Error::InvalidInput("Unsupported image address"))?;
    if (force || expired(item))
        && let (Some(id), Some(store)) = (&item.asset_id, store)
    {
        let response = store
            .client(false)
            .await?
            .attachment_urls(room, std::slice::from_ref(id))
            .await?;
        let asset = response
            .get("assets")
            .and_then(Value::as_array)
            .and_then(|assets| {
                assets
                    .iter()
                    .find(|asset| asset.get("id").and_then(Value::as_str) == Some(id))
            })
            .ok_or(Error::InvalidResponse)?;
        let raw = if item.original {
            asset.pointer("/assetUrl/url")
        } else {
            asset
                .pointer("/thumbnailAssetUrl/url")
                .or_else(|| asset.pointer("/assetUrl/url"))
        }
        .and_then(Value::as_str)
        .ok_or(Error::InvalidResponse)?;
        let fresh = media_url(raw, origin).ok_or(Error::InvalidResponse)?;
        if fresh.origin() != original.origin()
            && fresh.origin().ascii_serialization() != origin.trim_end_matches('/')
        {
            return Err(Error::InvalidResponse);
        }
        return Ok(fresh);
    }
    Ok(original)
}
async fn fetch_image(url: &Url) -> Result<Vec<u8>, Error> {
    let client = reqwest::Client::builder()
        .redirect(reqwest::redirect::Policy::none())
        .timeout(Duration::from_secs(20))
        .build()
        .map_err(|_| Error::Transport)?;
    let mut response = client
        .get(url.clone())
        .send()
        .await
        .map_err(|_| Error::Transport)?;
    if !response.status().is_success() {
        return Err(Error::Http(response.status().as_u16()));
    }
    let mime = response
        .headers()
        .get("content-type")
        .and_then(|v| v.to_str().ok())
        .and_then(|v| v.split(';').next())
        .unwrap_or("")
        .trim();
    if !matches!(
        mime,
        "image/png" | "image/jpeg" | "image/gif" | "image/webp"
    ) {
        return Err(Error::InvalidResponse);
    }
    if response
        .content_length()
        .is_some_and(|length| length > MAX_BYTES as u64)
    {
        return Err(Error::ResponseTooLarge);
    }
    let mut bytes = Vec::new();
    while let Some(chunk) = response.chunk().await.map_err(|_| Error::Transport)? {
        if chunk.len() > MAX_BYTES - bytes.len() {
            return Err(Error::ResponseTooLarge);
        }
        bytes.extend_from_slice(&chunk);
    }
    Ok(bytes)
}
fn decode(bytes: &[u8], small: bool) -> Result<(Arc<RenderImage>, usize), Error> {
    let reader = image::ImageReader::new(Cursor::new(bytes))
        .with_guessed_format()
        .map_err(|_| Error::InvalidResponse)?;
    let (width, height) = reader
        .into_dimensions()
        .map_err(|_| Error::InvalidResponse)?;
    if width == 0 || height == 0 || u64::from(width) * u64::from(height) > MAX_PIXELS {
        return Err(Error::ResponseTooLarge);
    }
    let mut reader = image::ImageReader::new(Cursor::new(bytes))
        .with_guessed_format()
        .map_err(|_| Error::InvalidResponse)?;
    let mut limits = image::Limits::default();
    limits.max_alloc = Some(MAX_PIXELS * 4);
    reader.limits(limits);
    let mut pixels = reader
        .decode()
        .map_err(|_| Error::InvalidResponse)?
        .thumbnail(if small { 128 } else { 800 }, if small { 128 } else { 600 })
        .into_rgba8();
    for pixel in pixels.pixels_mut() {
        pixel.0.swap(0, 2);
    }
    let bytes = pixels.as_raw().len();
    Ok((
        Arc::new(RenderImage::new(vec![image::Frame::new(pixels)])),
        bytes,
    ))
}

/// Decode only the known display fields; unknown timeline data stays in the SDK.
pub fn attachments(message: &Message) -> Vec<MediaItem> {
    message
        .metadata
        .get("attachments")
        .and_then(Value::as_array)
        .into_iter()
        .flatten()
        .take(8)
        .filter_map(|attachment| {
            let id = attachment.get("id")?.as_str()?;
            let image = attachment
                .get("contentType")
                .and_then(Value::as_str)
                .is_some_and(|mime| mime.starts_with("image/"));
            let asset = attachment.get("assetUrl");
            let thumbnail = attachment
                .get("thumbnailAssetUrl")
                .or_else(|| attachment.pointer("/videoProcessing/thumbnailAssetUrl"));
            let source = if image { thumbnail.or(asset) } else { asset }?;
            Some(MediaItem {
                key: format!("attachment:{id}"),
                url: source.get("url")?.as_str()?.into(),
                open_url: asset
                    .and_then(|asset| asset.get("url"))
                    .and_then(Value::as_str)
                    .unwrap_or("")
                    .into(),
                label: attachment
                    .get("filename")
                    .and_then(Value::as_str)
                    .unwrap_or("Attachment")
                    .into(),
                expires: source
                    .get("expiresAt")
                    .and_then(Value::as_str)
                    .map(str::to_owned),
                asset_id: Some(id.into()),
                image,
                width: attachment
                    .get("width")
                    .and_then(Value::as_f64)
                    .unwrap_or(0.) as f32,
                height: attachment
                    .get("height")
                    .and_then(Value::as_f64)
                    .unwrap_or(0.) as f32,
                original: false,
            })
        })
        .collect()
}

pub fn image_view(cache: &Entity<MediaCache>, item: MediaItem, cx: &mut App) -> gpui::AnyElement {
    let consent = cache.read(cx).requires_consent(&item);
    let failed = cache.read(cx).failed(&item.key);
    let host = media_url(&item.url, cache.read(cx).origin())
        .and_then(|url| url.host_str().map(str::to_owned))
        .unwrap_or_default();
    let mut view = div().w_full().min_w_0().flex().flex_col().gap_1();
    if item.image && !consent && !failed {
        let ratio = if item.width > 0. && item.height > 0. {
            (item.width / item.height).clamp(0.2, 5.)
        } else {
            1.8
        };
        let width = if item.width > 0. {
            item.width.min(440.).min(280. * ratio)
        } else {
            400.
        };
        let source = MediaCache::source(cache, item.clone());
        let full = item.clone();
        let media = cache.clone();
        view = view.child(
            Button::new(SharedString::from(format!("open-image:{}", item.key)))
                .ghost()
                .p_0()
                .h(ui::unit(width / ratio + 10.))
                .min_h_0()
                .w(ui::unit(width + 10.))
                .max_w_full()
                .self_start()
                .rounded_lg()
                .tooltip(item.label.clone())
                .child(
                    ui::inset_frame().size_full().child(
                        img(source)
                            .size_full()
                            .object_fit(ObjectFit::Contain)
                            .rounded(ui::unit(5.)),
                    ),
                )
                .on_click(move |_, _, cx| {
                    media.update(cx, |media, cx| media.open(full.clone(), cx))
                }),
        );
    } else {
        let mut card = ui::card().max_w(ui::unit(440.));
        if consent {
            let key = item.clone();
            let media = cache.clone();
            card = card.child(
                ui::text_button(
                    SharedString::from(format!("load:{}", item.key)),
                    format!("Load image from {host}"),
                )
                .on_click(move |_, _, cx| media.update(cx, |media, cx| media.allow(&key, cx))),
            );
        } else if failed {
            let key = item.key.clone();
            let media = cache.clone();
            card = card.child("Image unavailable").child(
                ui::text_button(SharedString::from(format!("retry:{}", item.key)), "Retry")
                    .on_click(move |_, _, cx| media.update(cx, |media, cx| media.retry(&key, cx))),
            );
        }
        let full = item.clone();
        let media = cache.clone();
        card = card.child(item.label.clone()).child(
            ui::text_button(
                SharedString::from(format!("open:{}", item.key)),
                "Open in browser",
            )
            .on_click(move |_, _, cx| media.update(cx, |media, cx| media.open(full.clone(), cx))),
        );
        view = view.child(card);
    }
    if cache.read(cx).open_errors.contains(&item.key) {
        view = view.child(
            div()
                .text_size(ui::unit(ui::CAPTION))
                .text_color(rgb(0xefabab))
                .child("Could not open this attachment. Try again."),
        );
    }
    view.into_any_element()
}

/// Native link and social cards never create a browser embed or autoplay media.
/// Rendering depth and image counts stay bounded even with unknown provider data.
pub fn link_preview(cache: &Entity<MediaCache>, preview: &Value, cx: &mut App) -> gpui::AnyElement {
    let mut card = ui::card();
    if let Some(post) = preview.get("socialPost") {
        card = card.child(social_preview(cache, post, 0, cx));
    } else {
        if let Some(raw) = preview.get("imageUrl").and_then(Value::as_str) {
            let mut item = MediaItem::inline(raw);
            item.key = format!(
                "preview:{}",
                preview
                    .get("imageAssetId")
                    .and_then(Value::as_str)
                    .unwrap_or(raw)
            );
            item.label = preview
                .get("title")
                .and_then(Value::as_str)
                .unwrap_or("Link preview")
                .into();
            card = card.child(image_view(cache, item, cx));
        }
        card = card.child(
            div()
                .text_size(ui::unit(ui::CAPTION))
                .text_color(rgb(ui::MUTED))
                .child(
                    preview
                        .get("siteName")
                        .and_then(Value::as_str)
                        .unwrap_or("Link preview")
                        .to_owned(),
                ),
        );
        if let Some(title) = preview.get("title").and_then(Value::as_str) {
            card = card.child(
                div()
                    .font_weight(gpui::FontWeight::SEMIBOLD)
                    .child(title.to_owned()),
            );
        }
        if let Some(description) = preview.get("description").and_then(Value::as_str) {
            card = card.child(description.to_owned());
        }
    }
    if let Some(url) = preview
        .get("url")
        .and_then(Value::as_str)
        .and_then(|raw| media_url(raw, cache.read(cx).origin()))
    {
        card = card.child(
            ui::text_button("open-preview-link", "Open link in browser")
                .self_start()
                .on_click(move |_, _, cx| cx.open_url(url.as_str())),
        );
    }
    card.into_any_element()
}
fn social_preview(
    cache: &Entity<MediaCache>,
    post: &Value,
    depth: u8,
    cx: &mut App,
) -> gpui::AnyElement {
    let mut card = ui::card();
    let author = post
        .pointer("/author/displayName")
        .and_then(Value::as_str)
        .unwrap_or("Social post");
    card = card.child(
        div()
            .font_weight(gpui::FontWeight::SEMIBOLD)
            .child(author.to_owned()),
    );
    if let Some(handle) = post.pointer("/author/handle").and_then(Value::as_str) {
        card = card.child(
            div()
                .text_size(ui::unit(ui::CAPTION))
                .text_color(rgb(ui::MUTED))
                .child(format!("@{handle}")),
        );
    }
    if let Some(warning) = post
        .get("contentWarning")
        .and_then(Value::as_str)
        .filter(|warning| !warning.is_empty())
    {
        // Keep warning-bearing content collapsed rather than revealing it as a
        // side effect of opening a channel. The canonical post is one click away.
        return card
            .child(format!("Content warning: {warning}"))
            .child("Open the post in your browser to view it.")
            .into_any_element();
    }
    if let Some(text) = post.get("text").and_then(Value::as_str) {
        card = card.child(div().whitespace_normal().child(text.to_owned()));
    }
    for image in post
        .get("images")
        .and_then(Value::as_array)
        .into_iter()
        .flatten()
        .take(4)
    {
        if let Some(raw) = image.get("url").and_then(Value::as_str) {
            let mut item = MediaItem::inline(raw);
            item.key = format!(
                "social:{}",
                image.get("assetId").and_then(Value::as_str).unwrap_or(raw)
            );
            item.label = image
                .get("alt")
                .and_then(Value::as_str)
                .unwrap_or("Post image")
                .into();
            card = card.child(image_view(cache, item, cx));
        }
    }
    if depth == 0
        && let Some(quote) = post.get("quotedPost")
    {
        card = card.child(social_preview(cache, quote, 1, cx));
    }
    card.into_any_element()
}

/// Same-origin profile images use the same bounded decoder. Foreign profile
/// images keep their initials fallback instead of making a third-party request.
pub fn avatar(
    cache: &Entity<MediaCache>,
    name: &str,
    raw: Option<&str>,
    key: String,
    diameter: f32,
    square: bool,
    cx: &App,
) -> gpui::AnyElement {
    let fallback = || {
        if square {
            ui::avatar(name, diameter).into_any_element()
        } else {
            ui::avatar(name, diameter).rounded_full().into_any_element()
        }
    };
    let Some(raw) = raw.filter(|raw| !raw.is_empty()) else {
        return fallback();
    };
    let mut item = MediaItem::inline(raw);
    item.key = key;
    if cache.read(cx).requires_consent(&item) || media_url(raw, cache.read(cx).origin()).is_none() {
        return fallback();
    }
    let loading_name = name.to_owned();
    let failed_name = name.to_owned();
    let image = img(MediaCache::source(cache, item))
        .size(ui::unit(diameter))
        .object_fit(if square {
            ObjectFit::Contain
        } else {
            ObjectFit::Cover
        })
        .with_loading(move || {
            let view = ui::avatar(&loading_name, diameter);
            if square {
                view.into_any_element()
            } else {
                view.rounded_full().into_any_element()
            }
        })
        .with_fallback(move || {
            let view = ui::avatar(&failed_name, diameter);
            if square {
                view.into_any_element()
            } else {
                view.rounded_full().into_any_element()
            }
        });
    if square {
        image.rounded(ui::unit(8.)).into_any_element()
    } else {
        image.rounded_full().into_any_element()
    }
}

/// Bundled product artwork is public and does not belong to an account cache.
pub fn bundled_logo() -> Arc<RenderImage> {
    static LOGO: std::sync::OnceLock<Arc<RenderImage>> = std::sync::OnceLock::new();
    LOGO.get_or_init(|| {
        decode(include_bytes!("../assets/chatto-icon.png"), false)
            .expect("bundled Chatto artwork")
            .0
    })
    .clone()
}
/// Server artwork obeys the same origin and decode limits as profile images.
pub fn server_banner(cache: &Entity<MediaCache>, raw: &str, cx: &App) -> gpui::AnyElement {
    let mut item = MediaItem::inline(raw);
    item.key = "server-banner".into();
    if cache.read(cx).requires_consent(&item) || media_url(raw, cache.read(cx).origin()).is_none() {
        return div().into_any_element();
    }
    ui::inset_frame()
        .w_full()
        .child(
            img(MediaCache::source(cache, item))
                .w_full()
                .aspect_ratio(1.8)
                .object_fit(ObjectFit::Contain)
                .rounded(ui::unit(5.)),
        )
        .into_any_element()
}

#[cfg(test)]
mod tests {
    use super::*;
    use axum::{
        Json, Router,
        extract::State,
        http::{HeaderMap, StatusCode},
        routing::{get, post},
    };
    use serde_json::json;
    use std::{
        sync::{
            Mutex,
            atomic::{AtomicUsize, Ordering},
        },
        time::SystemTime,
    };
    #[test]
    fn only_web_addresses_are_accepted_and_foreign_images_need_consent() {
        let runtime = Arc::new(tokio::runtime::Runtime::new().unwrap());
        let cache = MediaCache::new(
            runtime,
            "https://server.example".into(),
            None,
            "room".into(),
        );
        assert!(!cache.requires_consent(&MediaItem::inline("/image.png")));
        assert!(cache.requires_consent(&MediaItem::inline("https://cdn.example/image.png")));
        for raw in [
            "file:///private/image.png",
            "data:image/png;base64,abc",
            "javascript:alert(1)",
            "http://remote.example/image.png",
            "https://user:password@server.example/image.png",
        ] {
            assert!(media_url(raw, "https://server.example").is_none());
        }
    }
    #[test]
    fn consent_for_one_image_host_does_not_authorize_another_host_after_metadata_changes() {
        let runtime = Arc::new(Runtime::new().unwrap());
        let mut cache = MediaCache::new(
            runtime,
            "https://server.example".into(),
            None,
            "room".into(),
        );
        cache.seed_preview();
        cache
            .entries
            .get_mut("attachment:preview-image")
            .unwrap()
            .allowed_origin = Some("https://first.example".into());
        let mut item = MediaItem::inline("https://first.example/image");
        item.key = "attachment:preview-image".into();
        assert!(!cache.requires_consent(&item));
        item.url = "https://second.example/image".into();
        assert!(cache.requires_consent(&item));
    }
    #[tokio::test]
    async fn expired_attachment_urls_are_renewed_but_image_reads_have_no_credentials_or_redirects()
    {
        #[derive(Clone)]
        struct Fixture {
            origin: String,
            bytes: Arc<Vec<u8>>,
            reads: Arc<AtomicUsize>,
            renewals: Arc<AtomicUsize>,
            headers: Arc<Mutex<Vec<HeaderMap>>>,
        }
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let origin = format!("http://{}", listener.local_addr().unwrap());
        let image = image::DynamicImage::ImageRgba8(image::RgbaImage::from_pixel(
            4,
            3,
            image::Rgba([17, 34, 51, 255]),
        ));
        let mut bytes = Cursor::new(Vec::new());
        image.write_to(&mut bytes, image::ImageFormat::Png).unwrap();
        let state = Fixture {
            origin: origin.clone(),
            bytes: Arc::new(bytes.into_inner()),
            reads: Arc::new(AtomicUsize::new(0)),
            renewals: Arc::new(AtomicUsize::new(0)),
            headers: Arc::new(Mutex::new(vec![])),
        };
        let router = Router::new()
            .route("/image", get(|State(state):State<Fixture>, headers:HeaderMap| async move {
                state.reads.fetch_add(1, Ordering::SeqCst); state.headers.lock().unwrap().push(headers);
                ([("Content-Type","image/png")], state.bytes.as_ref().clone())
            }))
            .route("/redirect", get(|| async { (StatusCode::FOUND, [("Location","/image")]) }))
            .route("/svg", get(|| async { ([("Content-Type","image/svg+xml")], "<svg/>") }))
            .route("/api/connect/chatto.api.v1.AssetService/BatchGetAssets", post(|State(state):State<Fixture>, headers:HeaderMap, Json(body):Json<Value>| async move {
                assert_eq!(headers.get("authorization").unwrap(), "Bearer synthetic-media-access");
                assert_eq!(body["roomId"], "private-room"); assert_eq!(body["assetIds"], json!(["asset"]));
                state.renewals.fetch_add(1, Ordering::SeqCst);
                Json(json!({"assets":[{"id":"asset", "thumbnailAssetUrl":{"url":format!("{}/image",state.origin)}, "assetUrl":{"url":format!("{}/full",state.origin)}}]}))
            })).with_state(state.clone());
        let peer = tokio::spawn(async move {
            axum::serve(listener, router).await.unwrap();
        });
        let directory = std::env::current_dir()
            .unwrap()
            .join("../../.context")
            .join(format!("native-media-{}", uuid::Uuid::new_v4()));
        let store = Storage::fixture(directory.clone());
        store
            .install(
                chatto::HumanSession::restore(chatto::SessionCredentials {
                    origin: origin.clone(),
                    access_token: "synthetic-media-access".into(),
                    refresh_token: "synthetic-media-refresh".into(),
                    refresh_request_id: String::new(),
                    oauth_client_id: String::new(),
                    access_expires_at: SystemTime::now() + Duration::from_secs(900),
                    session_expires_at: SystemTime::now() + Duration::from_secs(3600),
                })
                .unwrap(),
            )
            .await
            .unwrap();
        let mut item = MediaItem::inline("/expired");
        item.asset_id = Some("asset".into());
        item.expires = Some("2000-01-01T00:00:00Z".into());
        let url = resolve(&item, &origin, Some(&store), "private-room", false)
            .await
            .unwrap();
        let bytes = fetch_image(&url).await.unwrap();
        let (decoded, _) = decode(&bytes, false).unwrap();
        assert_eq!(
            &decoded.as_bytes(0).unwrap()[..4],
            &[51, 34, 17, 255],
            "GPUI raster data must be BGRA"
        );
        assert_eq!(state.renewals.load(Ordering::SeqCst), 1);
        assert!(matches!(
            fetch_image(&Url::parse(&format!("{origin}/redirect")).unwrap()).await,
            Err(Error::Http(302))
        ));
        assert!(matches!(
            fetch_image(&Url::parse(&format!("{origin}/svg")).unwrap()).await,
            Err(Error::InvalidResponse)
        ));
        assert_eq!(
            state.reads.load(Ordering::SeqCst),
            1,
            "redirects must not trigger another image request"
        );
        for headers in state.headers.lock().unwrap().iter() {
            for name in ["authorization", "cookie", "referer"] {
                assert!(headers.get(name).is_none());
            }
        }
        drop(store);
        peer.abort();
        std::fs::remove_dir_all(directory).unwrap();
    }
    #[gpui::test]
    fn a_signature_refresh_does_not_discard_an_already_decoded_attachment(
        cx: &mut gpui::TestAppContext,
    ) {
        let runtime = Arc::new(Runtime::new().unwrap());
        let cache = cx.new(|_| {
            let mut cache = MediaCache::new(
                runtime,
                "https://server.example".into(),
                None,
                "room".into(),
            );
            cache.seed_preview();
            cache
        });
        cache.update(cx, |cache, cx| {
            let mut item = MediaItem::inline("https://server.example/image?signature=first");
            item.key = "attachment:preview-image".into();
            let first = cache.get(&item, cx).unwrap();
            item.url = "https://server.example/image?signature=second".into();
            let second = cache.get(&item, cx).unwrap();
            assert!(Arc::ptr_eq(&first, &second));
        });
    }
}
