<template>
	<div ref="el" class="chart" role="img" :aria-label="t('today.chart')"></div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import * as echarts from "echarts/core";
import { LineChart } from "echarts/charts";
import { GridComponent, TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import type { CurvePoint } from "../today";
import { fmtEnergy } from "../format";
import { t } from "../i18n";

echarts.use([LineChart, GridComponent, TooltipComponent, CanvasRenderer]);

const props = defineProps<{ curve: CurvePoint[] }>();

const el = ref<HTMLDivElement>();
let chart: echarts.ECharts | undefined;
let observer: ResizeObserver | undefined;

const cssVar = (name: string) =>
	getComputedStyle(document.documentElement).getPropertyValue(name).trim();

function render() {
	if (!chart) return;
	const muted = cssVar("--muted");
	const border = cssVar("--border");
	const time = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" });

	chart.setOption({
		animationDuration: 400,
		grid: { left: 8, right: 8, top: 16, bottom: 8, containLabel: true },
		tooltip: {
			trigger: "axis",
			backgroundColor: cssVar("--surface"),
			borderColor: border,
			textStyle: { color: cssVar("--text") },
			formatter: (params: any[]) =>
				[
					time.format(params[0].value[0]),
					...params.map((p) => `${p.marker} ${p.seriesName}: ${fmtEnergy(p.value[1])}`),
				].join("<br/>"),
		},
		xAxis: {
			type: "time",
			splitNumber: 4,
			axisLabel: {
				color: muted,
				hideOverlap: true,
				formatter: (v: number) => time.format(v),
			},
			axisLine: { lineStyle: { color: border } },
			splitLine: { show: false },
		},
		yAxis: {
			type: "value",
			axisLabel: { color: muted, formatter: (v: number) => `${v} kWh` },
			splitLine: { lineStyle: { color: border } },
		},
		series: [
			{
				name: t("today.expected"),
				type: "line",
				showSymbol: false,
				smooth: true,
				lineStyle: { type: "dashed", color: muted, width: 2 },
				itemStyle: { color: muted },
				data: props.curve.map((p) => [p.ts, p.forecast]),
			},
			{
				name: t("today.actual"),
				type: "line",
				showSymbol: false,
				smooth: true,
				lineStyle: { color: cssVar("--pv"), width: 3 },
				itemStyle: { color: cssVar("--pv") },
				areaStyle: { color: cssVar("--pv"), opacity: 0.15 },
				data: props.curve.map((p) => [p.ts, p.actual]),
			},
		],
	});
}

const dark = window.matchMedia("(prefers-color-scheme: dark)");

onMounted(() => {
	chart = echarts.init(el.value!);
	render();
	observer = new ResizeObserver(() => chart?.resize());
	observer.observe(el.value!);
	dark.addEventListener("change", render);
});

onBeforeUnmount(() => {
	observer?.disconnect();
	dark.removeEventListener("change", render);
	chart?.dispose();
});

watch(() => props.curve, render);
</script>

<style scoped>
.chart {
	width: 100%;
	height: 220px;
}
</style>
