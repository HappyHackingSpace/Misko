import { darkTheme } from "naive-ui";

// Minimal, low-chroma overrides (Linear/Vercel-leaning): one accent color,
// restrained radii, no heavy shadows. Kept close to the existing CSS-variable
// palette in style.css so pages not yet moved to Naive UI still feel related.
const FONT_FAMILY = "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif";

const shape = {
  Button: {
    borderRadiusMedium: "8px",
    borderRadiusSmall: "7px",
    borderRadiusTiny: "6px",
    fontWeight: "600",
    fontWeightStrong: "650",
    heightMedium: "36px",
    heightSmall: "30px",
  },
  Card: { borderRadius: "12px" },
  Input: { borderRadius: "8px", heightMedium: "36px" },
  Select: { peers: { InternalSelection: { borderRadius: "8px", heightMedium: "36px" } } },
  Tag: { borderRadius: "6px" },
  DataTable: { borderRadius: "10px", thPaddingMedium: "10px 12px", tdPaddingMedium: "10px 12px" },
  Menu: {
    borderRadius: "8px",
    itemHeight: "40px",
    fontSize: "14.5px",
    // A soft tint instead of a filled pill, plus a left accent bar drawn in
    // App.vue's :deep() styles; var(--accent) already tracks the active theme.
    itemColorActive: "color-mix(in srgb, var(--accent) 12%, transparent)",
    itemColorActiveHover: "color-mix(in srgb, var(--accent) 18%, transparent)",
    itemColorActiveCollapsed: "color-mix(in srgb, var(--accent) 12%, transparent)",
    itemTextColorActive: "var(--accent)",
    itemTextColorActiveHover: "var(--accent)",
    itemIconColorActive: "var(--accent)",
    itemIconColorActiveHover: "var(--accent)",
    itemIconColorActiveCollapsed: "var(--accent)",
  },
  Dropdown: { borderRadius: "10px" },
};

export const lightThemeOverrides = {
  common: {
    // Monochrome in light mode: black on white, no blue. Dark mode keeps its
    // blue accent below, since that wasn't the complaint.
    primaryColor: "#171717",
    primaryColorHover: "#3a3a3d",
    primaryColorPressed: "#000000",
    primaryColorSuppl: "#171717",
    fontFamily: FONT_FAMILY,
    fontSize: "14px",
    bodyColor: "#f6f8fa",
    cardColor: "#ffffff",
    modalColor: "#ffffff",
    popoverColor: "#ffffff",
    textColorBase: "#1f2328",
    textColor1: "#1f2328",
    textColor2: "#3a4149",
    textColor3: "#57606a",
    borderColor: "#d0d7de",
    dividerColor: "#d0d7de",
  },
  ...shape,
};

export const darkThemeOverrides = {
  common: {
    primaryColor: "#4cc2ff",
    primaryColorHover: "#6ccdff",
    primaryColorPressed: "#2fa8e6",
    primaryColorSuppl: "#4cc2ff",
    fontFamily: FONT_FAMILY,
    fontSize: "14px",
    bodyColor: "#0b0f14",
    cardColor: "#121a24",
    modalColor: "#121a24",
    popoverColor: "#121a24",
    textColorBase: "#e6edf3",
    textColor1: "#e6edf3",
    textColor2: "#c3ceda",
    textColor3: "#8a9bb0",
    borderColor: "#233244",
    dividerColor: "#233244",
  },
  ...shape,
};

export { darkTheme };
