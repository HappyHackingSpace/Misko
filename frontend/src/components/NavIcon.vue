<script setup>
// One small stroke-icon set for the sidebar menu, keyed by name, so a dynamic
// menu can pick an icon per route without a full icon library dependency.
const props = defineProps({ name: { type: String, required: true }, size: { type: [Number, String], default: 19 } });
</script>

<template>
  <svg :width="props.size" :height="props.size" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
    <g v-if="name === 'experiments'">
      <path d="M9 2v6.2L4.5 17a2 2 0 0 0 1.8 3h11.4a2 2 0 0 0 1.8-3L15 8.2V2" />
      <path d="M8.5 2h7" />
      <path d="M8 14h8" />
    </g>
    <g v-else-if="name === 'subjects'">
      <rect x="3" y="5" width="18" height="14" rx="2.4" />
      <circle cx="8.7" cy="10.6" r="1.7" />
      <path d="M5.6 15.2c0-1.7 1.4-2.9 3.1-2.9s3.1 1.2 3.1 2.9" />
      <path d="M14.2 9.2h4M14.2 12.2h4" />
    </g>
    <g v-else-if="name === 'reports'">
      <path d="M4 20V10M11 20V4M18 20v-7" />
    </g>
    <g v-else-if="name === 'paradigms'">
      <path d="M12 3 3 8l9 5 9-5-9-5Z" />
      <path d="M3 16l9 5 9-5" />
      <path d="M3 12l9 5 9-5" />
    </g>
    <g v-else-if="name === 'environments'">
      <path d="M21 8 12 3 3 8v8l9 5 9-5V8Z" />
      <path d="M3 8l9 5 9-5" />
      <path d="M12 13v8" />
    </g>
    <g v-else-if="name === 'users'">
      <circle cx="9" cy="8" r="3.2" />
      <path d="M3 20c0-3.3 2.7-6 6-6s6 2.7 6 6" />
      <path d="M16 4.2a3.2 3.2 0 0 1 0 6.2M21 20c0-2.9-2-5.3-4.7-5.9" />
    </g>
    <g v-else-if="name === 'test'">
      <path d="M9 3h6a1 1 0 0 1 1 1v1H8V4a1 1 0 0 1 1-1Z" />
      <path d="M8 5H6a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-2" />
      <path d="m9 13 2 2 4-4" />
    </g>
  </svg>
</template>
