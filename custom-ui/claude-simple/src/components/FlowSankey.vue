<template>
	<section class="card" aria-labelledby="flow-title">
		<h2 id="flow-title">{{ t("flow.title") }}</h2>
		<div class="stage" :style="{ height: `${height}px` }">
			<div ref="el" class="chart"></div>
			<p v-if="!flow.links.length" class="empty muted" data-testid="flow-empty">
				{{ t("flow.empty") }}
			</p>
		</div>
	</section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import * as echarts from "echarts/core";
import { SankeyChart } from "echarts/charts";
import { TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import type { Flow, NodeKind } from "../flow";
import { fmtPower } from "../format";
import { t } from "../i18n";

echarts.use([SankeyChart, TooltipComponent, CanvasRenderer]);

const props = defineProps<{ flow: Flow }>();

const el = ref<HTMLDivElement>();
let chart: echarts.ECharts | undefined;
let timer: number | undefined;
let last = 0;

const MIN_INTERVAL = 1000;

const height = computed(() => {
	const sources = props.flow.nodes.filter((n) => n.side === "source").length;
	const sinks = props.flow.nodes.filter((n) => n.side === "sink").length;
	return Math.max(220, 90 + 52 * Math.max(sources, sinks));
});

const cssVar = (name: string) =>
	getComputedStyle(document.documentElement).getPropertyValue(name).trim();

const COLORS: Record<NodeKind, string> = {
	pv: "--pv",
	"grid-import": "--grid",
	"battery-discharge": "--battery",
	home: "--home",
	consumer: "--consumer",
	car: "--car",
	heating: "--heating",
	"battery-charge": "--battery",
	"grid-export": "--export",
};

function render() {
	if (!chart) return;
	last = Date.now();
	const text = cssVar("--text");
	const bg = cssVar("--surface");
	const nodes = props.flow.nodes;
	const color = new Map(nodes.map((n) => [n.id, cssVar(COLORS[n.kind])]));

	chart.setOption({
		animationDurationUpdate: 600,
		tooltip: {
			trigger: "item",
			valueFormatter: (v: number) => fmtPower(v),
			backgroundColor: bg,
			borderColor: cssVar("--border"),
			textStyle: { color: text },
		},
		series: [
			{
				type: "sankey",
				left: 8,
				right: 8,
				top: 8,
				bottom: 8,
				nodeWidth: 14,
				nodeGap: 16,
				draggable: false,
				layoutIterations: 0,
				emphasis: { focus: "adjacency" },
				lineStyle: { color: "gradient", curveness: 0.5, opacity: 0.45 },
				data: nodes.map((n) => ({
					name: n.id,
					depth: n.side === "source" ? 0 : 1,
					itemStyle: { color: color.get(n.id) },
					label: {
						position: n.side === "source" ? "right" : "left",
						formatter: `${n.label}\n${fmtPower(n.power)}`,
						color: text,
						fontSize: 12,
						lineHeight: 16,
						textBorderColor: bg,
						textBorderWidth: 3,
					},
				})),
				links: props.flow.links,
			},
		],
	});
}

// re-render at most once per interval, the flow changes with every websocket tick
function schedule() {
	window.clearTimeout(timer);
	const wait = Math.max(0, MIN_INTERVAL - (Date.now() - last));
	timer = window.setTimeout(render, wait);
}

const dark = window.matchMedia("(prefers-color-scheme: dark)");
const onTheme = () => render();
let observer: ResizeObserver | undefined;

onMounted(() => {
	chart = echarts.init(el.value!);
	render();
	observer = new ResizeObserver(() => chart?.resize());
	observer.observe(el.value!);
	dark.addEventListener("change", onTheme);
});

onBeforeUnmount(() => {
	window.clearTimeout(timer);
	observer?.disconnect();
	dark.removeEventListener("change", onTheme);
	chart?.dispose();
});

watch(() => props.flow, schedule);
watch(height, () => requestAnimationFrame(() => chart?.resize()));
</script>

<style scoped>
.stage {
	position: relative;
}

.chart {
	width: 100%;
	height: 100%;
}

.empty {
	position: absolute;
	inset: 0;
	display: flex;
	align-items: center;
	justify-content: center;
	margin: 0;
}
</style>
