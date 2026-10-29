import assert from 'node:assert/strict';
import test from 'node:test';

import { isReviewedStorageChange } from './check-storage-proto-breaking.mjs';

test('only the approved beta invitation use-limit removal is accepted', () => {
  const removal = {
    path: 'chatto/core/evt/v1/invitation_events.proto',
    type: 'FIELD_NO_DELETE',
    message:
      'Previously present field "2" with name "max_uses" on message "InvitationCreatedEvent" was deleted.'
  };
  assert.equal(isReviewedStorageChange(removal), true);
  for (const diagnostic of [
    { ...removal, path: 'chatto/core/evt/v1/event.proto' },
    { ...removal, type: 'FIELD_SAME_TYPE' },
    { ...removal, message: removal.message.replace('max_uses', 'expires_at') },
    { ...removal, message: removal.message.replace('"2"', '"3"') }
  ]) {
    assert.equal(isReviewedStorageChange(diagnostic), false);
  }
});

test('storage exceptions accept only the reviewed file and field relocations', () => {
  const file = {
    type: 'FILE_NO_DELETE',
    message: 'Previously present file "chatto/core/evt/v1/models.proto" was deleted.'
  };
  const field = {
    path: 'chatto/core/runtime_state/v1/legacy_state.proto',
    type: 'FIELD_SAME_TYPE',
    message:
      'Field "1" with name "legacy_sections" on message "RoomLayout" changed type from "chatto.core.evt.v1.RoomGroup" to "chatto.core.runtime_state.v1.RoomGroup".'
  };
  assert.equal(isReviewedStorageChange(file), true);
  assert.equal(isReviewedStorageChange(field), true);
  for (const diagnostic of [
    { ...file, path: 'chatto/core/evt/v1/event.proto' },
    { ...file, message: 'Previously present file "chatto/core/evt/v1/event.proto" was deleted.' },
    { ...file, type: 'MESSAGE_NO_DELETE' },
    { ...field, path: 'chatto/api/v1/rooms.proto' },
    { ...field, type: 'FIELD_NO_DELETE' },
    { ...field, message: field.message.replace('Field "1"', 'Field "2"') },
    {
      ...field,
      message: field.message.replace('runtime_state.v1.RoomGroup', 'runtime_state.v1.SidebarLink')
    },
    { ...field, message: 'Unexpected Buf output' }
  ]) {
    assert.equal(isReviewedStorageChange(diagnostic), false);
  }
});
