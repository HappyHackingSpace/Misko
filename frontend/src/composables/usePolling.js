// Calls `tick` again every `interval` milliseconds while `active()` is true,
// one call at a time, and stops for good when the component unmounts. Call
// `schedule()` after every load so polling follows what the screen shows.
import { onBeforeUnmount } from "vue";

export function usePolling(tick, active, interval = 5000) {
  let timer = null;
  let stopped = false;

  function schedule() {
    clearTimeout(timer);
    if (stopped || !active()) return;
    timer = setTimeout(async () => {
      await tick();
      schedule();
    }, interval);
  }

  function stop() {
    stopped = true;
    clearTimeout(timer);
  }

  onBeforeUnmount(stop);
  return { schedule, stop };
}
