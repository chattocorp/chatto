import assert from 'node:assert/strict';
import test from 'node:test';

import { isReviewedStorageRelocation } from './check-storage-proto-breaking.mjs';

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
  assert.equal(isReviewedStorageRelocation(file), true);
  assert.equal(isReviewedStorageRelocation(field), true);
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
    assert.equal(isReviewedStorageRelocation(diagnostic), false);
  }
});
