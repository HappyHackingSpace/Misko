<script setup>
// Small bar-chart wrapper over Chart.js (via vue-chartjs). Reactive: re-renders
// when labels/values change, so live-derived metrics update the chart.
import { computed } from "vue";
import { Bar } from "vue-chartjs";
import {
  Chart as ChartJS,
  Title,
  Tooltip,
  Legend,
  BarElement,
  CategoryScale,
  LinearScale,
} from "chart.js";

ChartJS.register(Title, Tooltip, Legend, BarElement, CategoryScale, LinearScale);

const props = defineProps({
  labels: { type: Array, default: () => [] },
  values: { type: Array, default: () => [] },
  label: { type: String, default: "" },
});

const data = computed(() => ({
  labels: props.labels,
  datasets: [
    {
      label: props.label,
      data: props.values,
      backgroundColor: "rgba(76, 194, 255, 0.55)",
      borderColor: "rgba(76, 194, 255, 1)",
      borderWidth: 1,
      borderRadius: 4,
    },
  ],
}));

const options = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: { legend: { display: false } },
  scales: {
    y: { beginAtZero: true, grid: { color: "rgba(128,128,128,0.15)" } },
    x: { grid: { display: false } },
  },
};
</script>

<template>
  <div class="chart-wrap"><Bar :data="data" :options="options" /></div>
</template>

<style scoped>
.chart-wrap { height: 240px; position: relative; }
</style>
