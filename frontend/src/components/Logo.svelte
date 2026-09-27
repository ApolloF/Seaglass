<script lang="ts">
  // Seaglass's mark: a drop with a play triangle.
  let { size = 24 }: { size?: number } = $props();
</script>

<svg width={size} height={size} viewBox="0 0 30 30" fill="none" aria-hidden="true">
  <path d="M15 3.5c4.8 5.6 8.5 10 8.5 14.2a8.5 8.5 0 0 1-17 0C6.5 13.5 10.2 9.1 15 3.5z" stroke="currentColor" stroke-width="2.2" stroke-linejoin="round" />
  <path d="M13 14.2v7l5.6-3.5z" fill="var(--accent)" />
</svg>
