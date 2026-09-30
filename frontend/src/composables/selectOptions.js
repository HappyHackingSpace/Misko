import { h } from "vue";
import { NTag } from "naive-ui";

// Options for Naive UI's NSelect. The label is rendered with a stable,
// locale-independent test hook, because the dropdown is not a native <select> and
// the browser suite cannot pick by value the way it could. `text` is kept beside
// it: a rendered label cannot be searched, so filterable lists match on it.
// `badge` is a short tag shown before the text, where a long label cannot cut it off, such as "Latest".
export function selectOption(testId, value, text, badge = "") {
  return {
    value,
    text,
    label: () =>
      h("span", { "data-test": `${testId}-option`, "data-value": value }, [
        badge ? h(NTag, { size: "small", round: true, bordered: false, type: "info", style: "margin-right: 8px", "data-test": `${testId}-badge` }, () => badge) : null,
        text,
      ]),
  };
}

// The search of a filterable list: a case-insensitive match on the option text.
export const filterByText = (pattern, option) => String(option.text || "").toLowerCase().includes(pattern.trim().toLowerCase());
