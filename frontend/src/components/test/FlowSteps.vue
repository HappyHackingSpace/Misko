<script setup>
// Where a test stands, in the order its work happens: planned, trials recorded,
// a video uploaded, analysis done. Each step is done, the one to do now, or still
// ahead, so a reader sees the next thing to do without reading the page.
import { computed } from "vue";

const props = defineProps({
  // planned | in progress | completed, as the test itself reports it.
  status: { type: String, required: true },
  trialsDone: { type: Number, default: 0 },
  trialsPlanned: { type: Number, default: 0 },
  verifiedVideos: { type: Number, default: 0 },
  // Analysis runs waiting or working, and runs that published a result.
  activeRuns: { type: Number, default: 0 },
  publishedRuns: { type: Number, default: 0 },
});

// A step is "done", "current" (the next thing to do) or "todo".
const steps = computed(() => {
  const started = props.status !== "PLANNED";
  const trialsComplete = props.trialsPlanned > 0 && props.trialsDone >= props.trialsPlanned;
  const list = [
    { key: "planned", state: "done" },
    { key: "trials", state: trialsComplete ? "done" : started ? "current" : "todo", detail: `${props.trialsDone}/${props.trialsPlanned}` },
    { key: "video", state: props.verifiedVideos > 0 ? "done" : "todo" },
    { key: "analysis", state: props.publishedRuns > 0 ? "done" : props.activeRuns > 0 ? "current" : "todo" },
  ];
  // The first step that is not done is the one to do now, unless one is already under way.
  if (!list.some((step) => step.state === "current")) {
    const next = list.find((step) => step.state === "todo");
    if (next) next.state = "current";
  }
  return list;
});
</script>

<template>
  <ol class="flow" data-test="flow">
    <template v-for="(step, index) in steps" :key="step.key">
      <li class="step" :class="step.state" data-test="flow-step" :data-step="step.key" :data-state="step.state">
        <span class="mark">
          <svg v-if="step.state === 'done'" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3.2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12" /></svg>
          <template v-else>{{ index + 1 }}</template>
        </span>
        <span class="label">{{ $t(`tests.flow.${step.key}`) }}<span v-if="step.detail" class="detail"> {{ step.detail }}</span></span>
      </li>
      <li v-if="index < steps.length - 1" class="line" :class="{ done: step.state === 'done' }" aria-hidden="true"></li>
    </template>
  </ol>
</template>

<style scoped>
.flow { list-style: none; margin: 0; padding: 0; display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.step { display: flex; align-items: center; gap: 7px; font-size: 13px; color: var(--muted); }
.mark {
  width: 22px; height: 22px; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center;
  font-size: 11px; font-weight: 700; border: 1.5px solid var(--line); color: var(--muted); flex-shrink: 0;
}
.step.done .mark { background: var(--good); border-color: var(--good); color: #fff; }
.step.done { color: var(--txt); }
.step.current .mark { border-color: var(--accent); color: var(--accent); }
.step.current { color: var(--txt); font-weight: 700; }
.detail { color: var(--muted); font-weight: 400; font-variant-numeric: tabular-nums; margin-left: 5px; }
.line { flex: 1; min-width: 16px; height: 2px; background: var(--line); border-radius: 2px; }
.line.done { background: var(--good); }
</style>
