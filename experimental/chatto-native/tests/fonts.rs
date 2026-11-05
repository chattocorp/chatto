//! Check native font initialization in a child process on its main thread.
#![cfg(target_os = "macos")]

use std::{
    process::Command,
    thread,
    time::{Duration, Instant},
};

#[test]
fn app_binary_uses_a_real_font_backend() {
    let mut child = Command::new(env!("CARGO_BIN_EXE_chatto-native"))
        .arg("--check-fonts")
        .spawn()
        .expect("start font smoke check");
    let start = Instant::now();
    loop {
        if let Some(status) = child.try_wait().expect("read font smoke status") {
            assert!(status.success(), "native font initialization failed");
            return;
        }
        if start.elapsed() > Duration::from_secs(10) {
            child.kill().expect("stop stalled font smoke check");
            child.wait().expect("reap font smoke check");
            panic!("native font initialization timed out");
        }
        thread::sleep(Duration::from_millis(50));
    }
}
