import assert from 'node:assert/strict';
import test from 'node:test';

import { isReadModelDeletion } from './check-storage-proto-breaking.mjs';

test('read-model exception accepts only the reviewed deletions', () => {
  const deletion = {
    path: 'chatto/core/evt/v1/models.proto',
    type: 'MESSAGE_NO_DELETE',
    message: 'Previously present message "Room" was deleted from file.'
  };
  for (const name of ['Role', 'Room', 'RoomMembership', 'RoomNotice', 'User']) {
    assert.equal(
      isReadModelDeletion({
        ...deletion,
        message: `Previously present message "${name}" was deleted from file.`
      }),
      true
    );
  }
  for (const diagnostic of [
    { ...deletion, path: 'chatto/api/v1/rooms.proto' },
    { ...deletion, type: 'FIELD_NO_DELETE' },
    { ...deletion, message: 'Previously present message "RoomGroup" was deleted from file.' },
    { ...deletion, message: 'Previously present message "SidebarLink" was deleted from file.' },
    { ...deletion, message: 'Previously present message "Event" was deleted from file.' },
    { ...deletion, message: 'Unexpected Buf output' }
  ]) {
    assert.equal(isReadModelDeletion(diagnostic), false);
  }
});
