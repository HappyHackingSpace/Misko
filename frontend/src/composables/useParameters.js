// Presentation of paradigm parameters. The API speaks raw values (a ratio is
// 0.5, a switch is 0 or 1); people read and type 50 %, on and off, and units
// in their own language. Conversion happens only here.
import { useI18n } from "vue-i18n";

// Rounds away the binary noise of x * 100 (0.07 * 100 = 7.000000000000001).
const clean = (value) => Number(Number(value).toPrecision(12));

export function isPercent(parameter) {
  return parameter.unit === "ratio";
}

export function isSwitch(parameter) {
  return parameter.unit === "boolean";
}

export function toDisplay(parameter, value) {
  if (value === null || value === undefined || value === "") return value;
  return isPercent(parameter) ? clean(Number(value) * 100) : Number(value);
}

export function fromDisplay(parameter, value) {
  if (value === null || value === undefined || value === "") return value;
  return isPercent(parameter) ? clean(Number(value) / 100) : Number(value);
}

// Defaults for every parameter, in raw API values.
export function defaults(parameters) {
  return Object.fromEntries(parameters.map((parameter) => [parameter.key, parameter.default]));
}

export function useParameters() {
  const { t, n, te } = useI18n();

  const unit = (parameter) => {
    if (isPercent(parameter)) return "%";
    if (isSwitch(parameter)) return "";
    return te(`paradigms.units.${parameter.unit}`) ? t(`paradigms.units.${parameter.unit}`) : parameter.unit;
  };

  const number = (value) => n(value, { maximumFractionDigits: 6 });

  // "50 cm", "50 %", "On".
  const format = (parameter, value) => {
    if (value === null || value === undefined) return "-";
    if (isSwitch(parameter)) return Number(value) === 1 ? t("paradigms.on") : t("paradigms.off");
    const u = unit(parameter);
    return u ? `${number(toDisplay(parameter, value))} ${u}` : number(toDisplay(parameter, value));
  };

  // "20 - 150 cm"
  const range = (parameter) => {
    if (isSwitch(parameter)) return "";
    const u = unit(parameter);
    const text = `${number(toDisplay(parameter, parameter.min))} - ${number(toDisplay(parameter, parameter.max))}`;
    return u ? `${text} ${u}` : text;
  };

  return { unit, format, range };
}
