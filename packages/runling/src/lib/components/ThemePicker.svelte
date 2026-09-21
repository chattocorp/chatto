<script lang="ts">
  import { onMount } from "svelte";
  let preference = $state("system");
  let ready = $state(false);
  let update = () => {};

  function choose(theme: "light" | "dark" | "system") {
    preference = theme;
    update();
  }

  onMount(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const read = () => {
      try {
        return (
          localStorage.getItem("runling-theme") ??
          localStorage.getItem("factory-theme")
        );
      } catch {
        return null;
      }
    };
    const sync = () => {
      const saved = read();
      preference = saved === "light" || saved === "dark" ? saved : "system";
      apply();
    };
    const apply = () => {
      document.documentElement.dataset.theme =
        preference === "system"
          ? media.matches
            ? "dark"
            : "light"
          : preference;
    };
    update = () => {
      try {
        localStorage.setItem("runling-theme", preference);
      } catch {
        /* Session-only preference. */
      }
      apply();
    };
    const storage = (event: StorageEvent) => {
      if (
        event.key === "runling-theme" ||
        event.key === "factory-theme" ||
        event.key === null
      )
        sync();
    };
    sync();
    ready = true;
    media.addEventListener("change", apply);
    window.addEventListener("storage", storage);
    return () => {
      media.removeEventListener("change", apply);
      window.removeEventListener("storage", storage);
    };
  });
</script>

<div
  class="flex items-center gap-0.5 rounded-full bg-base-300/30 p-1"
  role="group"
  aria-label="Color theme"
>
  <button
    class="btn btn-ghost btn-circle btn-xs size-7 text-base-content/60 aria-pressed:text-base-content"
    class:btn-active={preference === "light"}
    type="button"
    aria-label="Light theme"
    aria-pressed={preference === "light"}
    title="Light theme"
    disabled={!ready}
    onclick={() => choose("light")}
  >
    <span class="icon-[lucide--sun] size-4" aria-hidden="true"></span>
  </button>
  <button
    class="btn btn-ghost btn-circle btn-xs size-7 text-base-content/60 aria-pressed:text-base-content"
    class:btn-active={preference === "dark"}
    type="button"
    aria-label="Dark theme"
    aria-pressed={preference === "dark"}
    title="Dark theme"
    disabled={!ready}
    onclick={() => choose("dark")}
  >
    <span class="icon-[lucide--moon] size-4" aria-hidden="true"></span>
  </button>
  <button
    class="btn btn-ghost btn-circle btn-xs size-7 text-base-content/60 aria-pressed:text-base-content"
    class:btn-active={preference === "system"}
    type="button"
    aria-label="Use system theme"
    aria-pressed={preference === "system"}
    title="Use system theme"
    disabled={!ready}
    onclick={() => choose("system")}
  >
    <span class="icon-[lucide--monitor] size-4" aria-hidden="true"></span>
  </button>
</div>
