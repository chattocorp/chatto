import assert from 'node:assert/strict';
import test from 'node:test';
import { hasAppOrigin, isAuthorizationLaunchUrl, isDesktopPermissionAllowed } from './security.mjs';

test('recognises only the fixed desktop origin', () => {
  assert.equal(hasAppOrigin('chatto://desktop/chat/room'), true);
  assert.equal(hasAppOrigin('chatto://desktop.evil/chat/room'), false);
  assert.equal(hasAppOrigin('https://desktop/chat/room'), false);
});

test('allows only required permissions for the desktop origin', () => {
  assert.equal(isDesktopPermissionAllowed('media', 'chatto://desktop'), true);
  assert.equal(isDesktopPermissionAllowed('notifications', 'chatto://desktop/login'), true);
  assert.equal(isDesktopPermissionAllowed('geolocation', 'chatto://desktop'), false);
  assert.equal(isDesktopPermissionAllowed('notifications', 'https://chat.example'), false);
});

test('allows screen capture only for the desktop origin', () => {
  assert.equal(isDesktopPermissionAllowed('display-capture', 'chatto://desktop'), true);
  assert.equal(isDesktopPermissionAllowed('display-capture', 'chatto://desktop/chat/room'), true);
  for (const origin of [
    'https://chat.example',
    'chatto://desktop.evil',
    'https://desktop',
    'null'
  ]) {
    assert.equal(isDesktopPermissionAllowed('display-capture', origin), false);
  }
});

test('opens windows only on the desktop authorization launch page', () => {
  assert.equal(isAuthorizationLaunchUrl('chatto://desktop/servers/authorize#launch-id'), true);
  assert.equal(isAuthorizationLaunchUrl('chatto://desktop/servers/authorize?next=x'), false);
  assert.equal(isAuthorizationLaunchUrl('chatto://desktop/servers/callback'), false);
  assert.equal(isAuthorizationLaunchUrl('https://desktop/servers/authorize'), false);
  assert.equal(isAuthorizationLaunchUrl('about:blank'), false);
});

test('allows clipboard writes only for the desktop origin', () => {
  assert.equal(isDesktopPermissionAllowed('clipboard-sanitized-write', 'chatto://desktop'), true);
  assert.equal(
    isDesktopPermissionAllowed('clipboard-sanitized-write', 'chatto://desktop/chat/room'),
    true
  );
  for (const origin of [
    'https://chat.example',
    'chatto://desktop.evil',
    'https://desktop',
    'null'
  ]) {
    assert.equal(isDesktopPermissionAllowed('clipboard-sanitized-write', origin), false);
  }
});

test('denies clipboard reads even for the desktop origin', () => {
  assert.equal(isDesktopPermissionAllowed('clipboard-read', 'chatto://desktop'), false);
  assert.equal(
    isDesktopPermissionAllowed('deprecated-sync-clipboard-read', 'chatto://desktop'),
    false
  );
});

test('allows fullscreen only for the desktop origin', () => {
  assert.equal(isDesktopPermissionAllowed('fullscreen', 'chatto://desktop'), true);
  assert.equal(isDesktopPermissionAllowed('fullscreen', 'chatto://desktop/chat/room'), true);
  for (const origin of [
    'https://chat.example',
    'chatto://desktop.evil',
    'https://desktop',
    'null'
  ]) {
    assert.equal(isDesktopPermissionAllowed('fullscreen', origin), false);
  }
});

test('denies automatic fullscreen even for the desktop origin', () => {
  assert.equal(isDesktopPermissionAllowed('automatic-fullscreen', 'chatto://desktop'), false);
});
