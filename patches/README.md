# Dependency patches

## app-builder-lib 26.18.0

`app-builder-lib@26.18.0.patch` makes the German Windows installer and
uninstaller messages use the formal address "Sie". The upstream messages for
a running application, an application that cannot close, and extraction or
removal failures use informal language. The uninstall confirmation uses "Sie".

Remove this patch when an upstream release uses consistent formal German in
these messages. Before release, check the German uninstall confirmation and
running-application prompt on Windows.

## virtua 0.54.0

`virtua@0.54.0.patch` keeps retained Svelte rows mounted when older rows are
inserted at the start of a list. With Svelte's async mode, the layout can still
have its previous length when `keepMounted` contains indexes for the new data.
The upstream check against that previous length removes retained rows. This
can stop media playback and clear a text selection.

The patch checks indexes against the current data length and rejects negative
indexes. It does not use the previous layout length to remove retained rows.
Rows outside the current layout stay mounted and hidden until the layout
includes them. Their offsets and cached sizes are not read before this update.
The existing `EventList.mediaRetention.svelte.spec.ts` tests cover audio, video,
and processed video with a text selection during this update.

Remove this patch when an upstream release passes these tests without it.
