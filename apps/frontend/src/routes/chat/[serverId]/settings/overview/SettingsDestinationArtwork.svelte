<!-- @component Decorative scenes for account and administration destinations, hidden from assistive technology. -->
<script lang="ts">
  let { kind }: { kind: string | undefined } = $props();
</script>

<svg
  aria-hidden="true"
  viewBox="0 0 280 140"
  class="h-36 w-72 overflow-visible"
  fill="none"
  stroke-linecap="round"
  stroke-linejoin="round"
>
  {#if kind === 'account'}
    <g transform="rotate(-6 125 70)">
      <rect x="38" y="20" width="174" height="104" rx="14" class="fill-background stroke-border" />
      <circle cx="65" cy="45" r="9" fill="currentColor" opacity=".25" />
      <path d="M85 45h63" stroke="currentColor" stroke-width="6" opacity=".3" />
      <rect x="55" y="67" width="140" height="31" rx="7" class="fill-surface-emphasized" />
      {#each [69, 85, 101, 117, 133] as x (x)}
        <circle
          class="pulse"
          style:--delay={`${(x - 69) / 80}s`}
          cx={x}
          cy="83"
          r="3"
          fill="currentColor"
        />
      {/each}
    </g>
    <g transform="rotate(22 215 91)" stroke="currentColor" stroke-width="9">
      <circle cx="220" cy="70" r="19" class="fill-background" />
      <path d="M220 90v39m0-12h13m-13-12h10" />
    </g>
  {:else if kind === 'profile'}
    <g transform="rotate(5 140 70)">
      <rect x="42" y="15" width="196" height="112" rx="15" class="fill-background stroke-border" />
      <path d="M57 16h166q15 0 15 15v23H42V31q0-15 15-15" fill="currentColor" opacity=".18" />
      <circle
        cx="88"
        cy="65"
        r="29"
        class="fill-surface-emphasized stroke-background"
        stroke-width="5"
      />
      <circle cx="88" cy="57" r="9" fill="currentColor" />
      <path d="M71 81c0-19 34-19 34 0" fill="currentColor" />
      <path
        d="M130 70h75M130 86h48M64 109h143"
        stroke="currentColor"
        stroke-width="6"
        opacity=".25"
      />
    </g>
    <g class="write" transform="rotate(35 228 97)">
      <rect x="220" y="68" width="15" height="50" rx="4" fill="currentColor" />
      <path d="m220 122 7 12 8-12" fill="currentColor" />
    </g>
  {:else if kind === 'time'}
    <g stroke="currentColor" stroke-width="2" opacity=".3">
      <circle cx="106" cy="67" r="52" /><ellipse cx="106" cy="67" rx="25" ry="52" />
      <path d="M54 67h104M62 40h88M62 94h88M106 15v104" />
    </g>
    <circle cx="184" cy="82" r="48" class="fill-background stroke-border" stroke-width="3" />
    <g stroke="currentColor" stroke-width="3"
      ><path d="M184 43v5m39 34h-5m-34 39v-5m-39-34h5" />
      <path class="clock" d="M184 56v26l19 12" stroke-width="5" /></g
    >
    <circle cx="184" cy="82" r="5" fill="currentColor" />
  {:else if kind === 'notifications'}
    <g transform="rotate(-6 130 70)">
      <rect x="42" y="22" width="165" height="48" rx="12" class="fill-background stroke-border" />
      <rect
        x="57"
        y="80"
        width="165"
        height="42"
        rx="12"
        class="fill-surface-emphasized stroke-border"
      />
      <circle cx="65" cy="46" r="10" fill="currentColor" opacity=".3" />
      <path d="M88 40h92M88 53h60M76 101h93" stroke="currentColor" stroke-width="5" opacity=".25" />
    </g>
    <circle cx="220" cy="63" r="32" class="fill-background" />
    <path
      d="M202 74h36l-5-9V54a13 13 0 0 0-26 0v11zm13 9q5 6 10 0"
      class="ring"
      stroke="currentColor"
      stroke-width="4"
    />
    <circle cx="240" cy="36" r="8" class="fill-warning stroke-background" stroke-width="3" />
  {:else if kind === 'general'}
    <g transform="rotate(-5 140 70)">
      <rect x="35" y="18" width="210" height="109" rx="13" class="fill-background stroke-border" />
      <rect x="47" y="30" width="186" height="36" rx="7" fill="currentColor" opacity=".2" />
      <rect x="59" y="42" width="37" height="37" rx="10" fill="currentColor" />
      <path
        d="M110 79h99M59 100h87M59 113h62"
        stroke="currentColor"
        stroke-width="5"
        opacity=".3"
      />
      <rect x="180" y="96" width="42" height="16" rx="8" fill="currentColor" opacity=".35" />
      <circle cx="212" cy="104" r="5" class="toggle fill-background" />
    </g>
  {:else if kind === 'neighbors'}
    <path
      d="m80 42 122 10M80 42l50 70m72-60-72 60"
      class="signal"
      stroke="currentColor"
      stroke-width="3"
      stroke-dasharray="5 7"
      opacity=".4"
    />
    {#each [{ x: 75, y: 39 }, { x: 207, y: 49 }, { x: 132, y: 109 }] as node (node.x)}
      <rect
        x={node.x - 28}
        y={node.y - 23}
        width="56"
        height="46"
        rx="12"
        class="fill-background stroke-border"
      />
      <path
        d={`M${node.x - 15} ${node.y - 7}h30m-30 14h30`}
        stroke="currentColor"
        stroke-width="6"
      />
      <circle
        cx={node.x + 19}
        cy={node.y - 20}
        r="5"
        class="fill-success stroke-background"
        stroke-width="2"
      />
    {/each}
  {:else if kind === 'members'}
    {#each [{ x: 65, y: 72 }, { x: 215, y: 72 }, { x: 140, y: 57 }] as person (person.x)}
      <g transform={`translate(${person.x} ${person.y})`}>
        <rect
          x="-38"
          y="-43"
          width="76"
          height="98"
          rx="14"
          class="fill-background stroke-border"
        />
        <circle
          class="greet"
          style:--delay={`${person.x / -200}s`}
          cy="-12"
          r="14"
          fill="currentColor"
          opacity=".7"
        />
        <path d="M-24 27c0-31 48-31 48 0" fill="currentColor" opacity=".3" />
        <path d="M-15 41h30" stroke="currentColor" stroke-width="4" opacity=".35" />
      </g>
    {/each}
  {:else if kind === 'bots'}
    <g transform="rotate(-5 122 73)">
      <path d="M120 29V14" stroke="currentColor" stroke-width="4" /><circle
        cx="120"
        cy="11"
        r="5"
        fill="currentColor"
      />
      <rect
        x="64"
        y="31"
        width="114"
        height="89"
        rx="25"
        class="fill-background stroke-border"
        stroke-width="2"
      />
      <rect x="77" y="48" width="88" height="39" rx="14" fill="currentColor" opacity=".2" />
      <circle class="blink" cx="98" cy="67" r="7" fill="currentColor" /><circle
        class="blink"
        cx="145"
        cy="67"
        r="7"
        fill="currentColor"
      />
      <path d="M107 102h29M55 62v24m132-24v24" stroke="currentColor" stroke-width="5" />
    </g>
    <g transform="rotate(8 212 97)">
      <rect
        x="171"
        y="73"
        width="80"
        height="51"
        rx="10"
        class="fill-surface-emphasized stroke-border"
      />
      <path
        d="m194 89-10 10 10 10m35-20 10 10-10 10m-14-23-8 26"
        stroke="currentColor"
        stroke-width="3"
      />
    </g>
  {:else if kind === 'registration'}
    <g transform="rotate(-7 135 75)">
      <g class="letter">
        <rect x="61" y="14" width="142" height="98" rx="10" class="fill-background stroke-border" />
        <path d="M84 36h98M84 51h65" stroke="currentColor" stroke-width="5" opacity=".3" />
      </g>
      <path d="M42 55h190v72H42z" class="fill-surface-emphasized stroke-border" />
      <path
        d="m42 55 95 55 95-55M42 127l62-35m128 35-62-35"
        stroke="currentColor"
        stroke-width="2"
        opacity=".4"
      />
    </g>
    <circle cx="228" cy="41" r="23" fill="currentColor" />
    <path d="M216 41h24m-12-12v24" class="stroke-background" stroke-width="4" />
  {:else if kind === 'rooms'}
    {#each [{ x: 48, y: 23 }, { x: 150, y: 12 }, { x: 63, y: 81 }, { x: 165, y: 70 }] as room (room.x)}
      <g transform={`translate(${room.x} ${room.y}) rotate(-5)`}>
        <rect width="83" height="48" rx="10" class="fill-background stroke-border" />
        <path d="M19 15v18m9-18v18M13 21h21m-21 7h21" stroke="currentColor" stroke-width="2" />
        <path
          class="pulse"
          style:--delay={`${room.x / -150}s`}
          d="M46 19h23M46 29h14"
          stroke="currentColor"
          stroke-width="4"
          opacity=".3"
        />
      </g>
    {/each}
  {:else if kind === 'moderation'}
    <g transform="rotate(-5 119 70)">
      <path
        d="M42 20h150q12 0 12 12v60q0 12-12 12h-95l-30 22v-22H42q-12 0-12-12V32q0-12 12-12"
        class="fill-background stroke-border"
      />
      <path d="M54 44h120M54 62h88M54 80h57" stroke="currentColor" stroke-width="6" opacity=".25" />
    </g>
    <path
      d="m213 50 36 14v28q0 22-36 38-36-16-36-38V64z"
      class="fill-surface-emphasized"
      stroke="currentColor"
      stroke-width="2"
    />
    <path
      class="draw"
      pathLength="1"
      d="m198 89 10 10 21-23"
      stroke="currentColor"
      stroke-width="5"
    />
  {:else if kind === 'permissions'}
    <path d="M140 40v27m-70 20V67h140v20" stroke="currentColor" stroke-width="3" opacity=".4" />
    <rect x="110" y="9" width="60" height="40" rx="11" fill="currentColor" />
    <path d="m127 29 9 8 16-17" class="draw stroke-background" pathLength="1" stroke-width="4" />
    {#each [70, 140, 210] as x (x)}
      <rect
        x={x - 26}
        y="83"
        width="52"
        height="44"
        rx="10"
        class="fill-background stroke-border"
      />
      <circle cx={x} cy="99" r="6" fill="currentColor" opacity=".65" />
      <path d={`M${x - 11} 116q0-18 22 0`} fill="currentColor" opacity=".3" />
    {/each}
  {:else if kind === 'security'}
    <g stroke="currentColor" stroke-width="2" opacity=".3">
      <path d="M54 47V26h30m142 21V26h-30M54 94v21h30m142-21v21h-30" />
      <path class="scan" d="M39 71h202" stroke-dasharray="4 7" />
    </g>
    <path
      d="m140 9 51 20v44q0 36-51 59-51-23-51-59V29z"
      class="fill-background stroke-border"
      stroke-width="2"
    />
    <rect x="116" y="59" width="48" height="38" rx="8" fill="currentColor" />
    <path d="M125 59V46a15 15 0 0 1 30 0v13" stroke="currentColor" stroke-width="5" />
    <path d="M140 73v10" class="stroke-background" stroke-width="5" />
  {:else if kind === 'event-log'}
    <g transform="rotate(-5 140 70)">
      <rect x="45" y="10" width="190" height="120" rx="13" class="fill-background stroke-border" />
      <path d="M74 32v76" stroke="currentColor" stroke-width="2" opacity=".3" />
      {#each [35, 70, 105] as y (y)}
        <circle
          class="pulse"
          style:--delay={`${y / -100}s`}
          cx="74"
          cy={y}
          r="6"
          fill="currentColor"
        />
        <path
          d={`M93 ${y - 4}h103m-103 11h65`}
          stroke="currentColor"
          stroke-width="5"
          opacity=".25"
        />
      {/each}
    </g>
    <circle
      cx="225"
      cy="100"
      r="22"
      class="fill-surface-emphasized"
      stroke="currentColor"
      stroke-width="3"
    />
    <path d="m241 117 13 13" stroke="currentColor" stroke-width="7" />
  {:else if kind === 'system'}
    {#each [22, 57, 92] as y (y)}
      <rect x="38" {y} width="114" height="28" rx="7" class="fill-background stroke-border" />
      <circle cx="53" cy={y + 14} r="4" class="fill-success" />
      <path d={`M68 ${y + 14}h62`} stroke="currentColor" stroke-width="4" opacity=".3" />
    {/each}
    <g transform="rotate(6 205 74)">
      <rect
        x="143"
        y="32"
        width="109"
        height="80"
        rx="11"
        class="fill-surface-emphasized stroke-border"
      />
      <path d="M157 95h80M157 78h80M157 61h80" stroke="currentColor" opacity=".12" />
      <path
        class="draw"
        pathLength="1"
        d="m156 83 16-5 11 8 15-36 13 23 13-7 14 6"
        stroke="currentColor"
        stroke-width="3"
      />
    </g>
  {/if}
</svg>

<style>
  /* SVG-specific pivots and stroke keyframes need scoped CSS, not shared UI utilities.
     Only pointer hover starts motion; leaving the card restores the still scene.
     Independent translate/rotate/scale properties preserve SVG placement transforms. */
  .pulse,
  .write,
  .clock,
  .ring,
  .toggle,
  .signal,
  .greet,
  .blink,
  .letter,
  .draw,
  .scan {
    animation: pulse 4s ease-in-out infinite;
    animation-delay: var(--delay, 0s);
  }
  :global(a:not(:hover))
    :is(.pulse, .write, .clock, .ring, .toggle, .signal, .greet, .blink, .letter, .draw, .scan) {
    animation-name: none;
  }
  .write {
    animation-name: write;
  }
  .clock {
    animation-name: clock;
    transform-origin: 184px 82px;
  }
  .ring {
    animation-name: ring;
    transform-origin: 220px 45px;
  }
  .toggle {
    animation-name: toggle;
  }
  .signal {
    animation-name: signal;
  }
  .blink {
    animation-name: blink;
    transform-box: fill-box;
    transform-origin: center;
  }
  .draw {
    animation-name: draw;
    stroke-dasharray: 1;
  }
  .scan {
    animation-name: scan;
  }
  @keyframes pulse {
    0%,
    65%,
    100% {
      opacity: 1;
    }
    25%,
    45% {
      opacity: 0.2;
    }
  }
  @keyframes write {
    0%,
    65%,
    100% {
      translate: 0 0;
    }
    20%,
    40% {
      translate: -7px 3px;
    }
    30%,
    50% {
      translate: 2px -2px;
    }
  }
  @keyframes clock {
    0% {
      rotate: 0deg;
    }
    65%,
    100% {
      rotate: 360deg;
    }
  }
  @keyframes ring {
    0%,
    45%,
    100% {
      rotate: 0deg;
    }
    15%,
    30% {
      rotate: 12deg;
    }
    22%,
    37% {
      rotate: -12deg;
    }
  }
  @keyframes toggle {
    0%,
    70%,
    100% {
      translate: 0 0;
    }
    25%,
    50% {
      translate: -22px 0;
    }
  }
  @keyframes signal {
    from {
      stroke-dashoffset: 24;
    }
    to {
      stroke-dashoffset: 0;
    }
  }
  @keyframes blink {
    0%,
    25%,
    35%,
    100% {
      scale: 1 1;
    }
    30% {
      scale: 1 0.1;
    }
  }
  @keyframes draw {
    0% {
      stroke-dashoffset: 1;
    }
    45%,
    100% {
      stroke-dashoffset: 0;
    }
  }
  @keyframes scan {
    0%,
    70%,
    100% {
      translate: 0 0;
    }
    20% {
      translate: 0 -25px;
    }
    50% {
      translate: 0 25px;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .pulse,
    .write,
    .clock,
    .ring,
    .toggle,
    .signal,
    .greet,
    .blink,
    .letter,
    .draw,
    .scan {
      animation: none;
    }
  }
</style>
