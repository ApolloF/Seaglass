<script lang="ts" module>
  // Stroke icons on a 24×24 grid.
  const paths: Record<string, string> = {
    grid: '<rect x="4" y="4" width="6.5" height="6.5" rx="1.5"/><rect x="13.5" y="4" width="6.5" height="6.5" rx="1.5"/><rect x="4" y="13.5" width="6.5" height="6.5" rx="1.5"/><rect x="13.5" y="13.5" width="6.5" height="6.5" rx="1.5"/>',
    check: '<path d="M5 12.5l4.5 4.5L19 7.5"/>',
    cloudCheck: '<path d="M7 18.5h10.2a4.3 4.3 0 0 0 .5-8.6 6 6 0 0 0-11.6 1.3A3.7 3.7 0 0 0 7 18.5z"/><path d="M9.6 13.8l2 2 3.6-3.6"/>',
    cloudDown: '<path d="M7 18.5h10.2a4.3 4.3 0 0 0 .5-8.6 6 6 0 0 0-11.6 1.3A3.7 3.7 0 0 0 7 18.5z"/><path d="M12 10.5v6M9.5 14l2.5 2.5 2.5-2.5"/>',
    star: '<path d="M12 4l2.4 5 5.4.7-4 3.7 1 5.4L12 16.2 7.2 18.8l1-5.4-4-3.7 5.4-.7z"/>',
    clock: '<circle cx="12" cy="12" r="8"/><path d="M12 7.5V12l3 2"/>',
    scan: '<path d="M4 8V5.5A1.5 1.5 0 0 1 5.5 4H8M16 4h2.5A1.5 1.5 0 0 1 20 5.5V8M20 16v2.5a1.5 1.5 0 0 1-1.5 1.5H16M8 20H5.5A1.5 1.5 0 0 1 4 18.5V16"/><circle cx="12" cy="12" r="3.2"/>',
    folder: '<path d="M4 7.5A1.5 1.5 0 0 1 5.5 6h4l2 2h7A1.5 1.5 0 0 1 20 9.5v8a1.5 1.5 0 0 1-1.5 1.5h-13A1.5 1.5 0 0 1 4 17.5z"/>',
    gear: '<path d="M19.09 9.88L21.43 10.19L21.43 13.81L19.09 14.12L18.51 15.52L19.95 17.38L17.38 19.95L15.52 18.51L14.12 19.09L13.81 21.43L10.19 21.43L9.88 19.09L8.48 18.51L6.62 19.95L4.05 17.38L5.49 15.52L4.91 14.12L2.57 13.81L2.57 10.19L4.91 9.88L5.49 8.48L4.05 6.62L6.62 4.05L8.48 5.49L9.88 4.91L10.19 2.57L13.81 2.57L14.12 4.91L15.52 5.49L17.38 4.05L19.95 6.62L18.51 8.48Z"/><circle cx="12" cy="12" r="3.2"/>',
    search: '<circle cx="11" cy="11" r="6.5"/><path d="M16 16l4 4"/>',
    play: '<path d="M8 5.5v13l10.5-6.5z" fill="currentColor" stroke="none"/>',
    stop: '<rect x="6.5" y="6.5" width="11" height="11" rx="2" fill="currentColor" stroke="none"/>',
    download: '<path d="M12 4v11M7.5 10.5L12 15l4.5-4.5M5 19.5h14"/>',
    dots: '<path d="M6 12h.01M12 12h.01M18 12h.01" stroke-width="3"/>',
    pad: '<path d="M8 7.5h8a4.8 4.8 0 0 1 4.7 5.8l-.7 3.4a2.5 2.5 0 0 1-4.3 1.2L14.2 16H9.8l-1.5 1.9A2.5 2.5 0 0 1 4 16.7l-.7-3.4A4.8 4.8 0 0 1 8 7.5z"/><path d="M8.2 10.6v3.2M6.6 12.2h3.2"/>',
    bolt: '<path d="M13.2 2.8L5.5 13.4h5.7l-.9 7.8 7.7-10.6h-5.7z"/>',
    info: '<circle cx="12" cy="12" r="8.5"/><path d="M12 11v5.2M12 7.8h.01"/>',
    refresh: '<path d="M19 8a7.5 7.5 0 1 0 .9 6"/><path d="M19.5 4v4.5H15"/>',
    eyeOff: '<path d="M4 4l16 16"/><path d="M9.9 5.3A9.7 9.7 0 0 1 12 5c5 0 8.5 4.5 9.5 7a13 13 0 0 1-2.6 3.8M6.5 7.1A13.3 13.3 0 0 0 2.5 12c1 2.5 4.5 7 9.5 7a9.6 9.6 0 0 0 4.4-1.1"/><path d="M9.9 10a3 3 0 0 0 4.1 4.1"/>',
    eye: '<path d="M2.5 12c1-2.5 4.5-7 9.5-7s8.5 4.5 9.5 7c-1 2.5-4.5 7-9.5 7s-8.5-4.5-9.5-7z"/><circle cx="12" cy="12" r="3"/>',
    close: '<path d="M6 6l12 12M18 6L6 18"/>',
    pencil: '<path d="M15.5 5.5l3 3L9 18H6v-3z"/><path d="M13.5 7.5l3 3"/>',
    file: '<path d="M7 3.5h7l4 4V20a.5.5 0 0 1-.5.5h-10A.5.5 0 0 1 7 20z"/><path d="M14 3.5V8h4"/>',
    chevronDown: '<path d="M6 9l6 6 6-6"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    trash: '<path d="M5 7h14M10 7V5h4v2M7 7l1 12.5h8L17 7"/>',
    warn: '<path d="M12 4l9 16H3z"/><path d="M12 10v4.5M12 17.2h.01"/>',
    sparkle: '<path d="M12 3.5l1.9 5.2 5.2 1.9-5.2 1.9L12 17.7l-1.9-5.2-5.2-1.9 5.2-1.9z"/>',
    link: '<path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1"/><path d="M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1"/>',
    tv: '<rect x="3" y="5" width="18" height="12" rx="2"/><path d="M8 20h8"/>',
    image: '<rect x="3.5" y="5" width="17" height="14" rx="2"/><circle cx="9" cy="10" r="1.6"/><path d="M20.5 16l-5-5L7 19"/>',
    layers: '<path d="M12 4l8.5 4.5L12 13 3.5 8.5z"/><path d="M3.5 12.5L12 17l8.5-4.5"/><path d="M3.5 16.5L12 21l8.5-4.5"/>',
  };
  export type IconName = keyof typeof paths;
</script>

<script lang="ts">
  let { name, size = 20, stroke = 1.9, label }: { name: IconName; size?: number; stroke?: number; label?: string } = $props();
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width={stroke}
  stroke-linecap="round"
  stroke-linejoin="round"
  role={label ? "img" : undefined}
  aria-label={label}
  aria-hidden={label ? undefined : "true"}
>
  {@html paths[name]}
</svg>
