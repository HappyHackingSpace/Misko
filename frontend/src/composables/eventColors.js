// One color per event type, in the order the types first appear, so the
// timeline strip, the filter chips and the event list agree on what is what.
const PALETTE = ["#4cc2ff", "#46d369", "#ffb454", "#c792ea", "#ff6b81", "#5bd1c5", "#e0c46c"];

export function eventColors(events) {
  const map = {};
  let i = 0;
  for (const event of events) {
    if (!(event.type in map)) map[event.type] = PALETTE[i++ % PALETTE.length];
  }
  return map;
}
