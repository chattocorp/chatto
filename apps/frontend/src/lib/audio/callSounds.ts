/** Local, pre-rendered call cues use media elements without a persistent audio graph. */
import joinURL from './call-join.wav?url';
import leaveURL from './call-leave.wav?url';

export type CallSoundKind = 'join' | 'leave';

/** Play a short cue when allowed; blocked or interrupted playback never affects a call. */
export function playCallSound(kind: CallSoundKind): Promise<void> {
  if (typeof Audio === 'undefined') return Promise.resolve();
  return new Promise((resolve) => {
    const audio = new Audio(kind === 'join' ? joinURL : leaveURL);
    const finish = () => {
      clearTimeout(timeout);
      audio.onended = null;
      audio.onerror = null;
      audio.pause();
      audio.removeAttribute('src');
      audio.load();
      resolve();
    };
    // Bound cleanup even when a browser never settles play() after interruption.
    const timeout = setTimeout(finish, 1500);
    audio.onended = finish;
    audio.onerror = finish;
    void audio.play().catch(finish);
  });
}
