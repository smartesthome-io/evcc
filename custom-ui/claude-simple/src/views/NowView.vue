<template>
	<div class="page">
		<StatusBadge :result="status" />

		<FlowSankey :flow="flow" />

		<BatteryCard v-if="state.battery" :battery="state.battery" />

		<section v-if="chargers.length" class="card" aria-labelledby="lp-title">
			<h2 id="lp-title">{{ t("loadpoints.title") }}</h2>
			<LoadpointRow
				v-for="{ lp, index } in chargers"
				:key="index"
				:loadpoint="lp"
				@mode="(m) => setMode(index, m)"
			/>
		</section>

		<section v-if="heaters.length" class="card" aria-labelledby="heat-title">
			<h2 id="heat-title">{{ t("heating.title") }}</h2>
			<LoadpointRow
				v-for="{ lp, index } in heaters"
				:key="index"
				:loadpoint="lp"
				@mode="(m) => setMode(index, m)"
			/>
		</section>

		<section v-if="others.length" class="card" aria-labelledby="others-title">
			<h2 id="others-title">{{ t("consumers.title") }}</h2>
			<ul class="list">
				<li v-for="(m, i) in others" :key="i">
					<span>{{ m.title || m.name }}</span>
					<strong>{{ fmtPower(m.power) }}</strong>
				</li>
			</ul>
		</section>
	</div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import StatusBadge from "../components/StatusBadge.vue";
import FlowSankey from "../components/FlowSankey.vue";
import BatteryCard from "../components/BatteryCard.vue";
import LoadpointRow from "../components/LoadpointRow.vue";
import { state, post } from "../state";
import { liveFlow } from "../flow";
import { siteStatus, type SiteStatusResult } from "../status";
import { fmtPower } from "../format";
import { labels, t } from "../i18n";
import type { Loadpoint } from "../types";

const flowLabels = labels("flow", [
	"pv",
	"gridImport",
	"batteryDischarge",
	"home",
	"consumer",
	"batteryCharge",
	"loadpoint",
	"gridExport",
]);

const flow = computed(() => liveFlow(state, flowLabels));

const status = ref<SiteStatusResult>(siteStatus(state));
watch(
	() => [state.grid?.power, state.battery?.power, state.loadpoints.map((lp) => lp.chargePower)],
	() => {
		status.value = siteStatus(state, status.value);
	},
	{ deep: true }
);

const indexed = computed(() => state.loadpoints.map((lp, index) => ({ lp, index })));
const chargers = computed(() => indexed.value.filter(({ lp }) => !lp.chargerFeatureHeating));
const heaters = computed(() => indexed.value.filter(({ lp }) => lp.chargerFeatureHeating));
const others = computed(() => [
	...(state.aux ?? []),
	...(state.consumers ?? []),
	...(state.ext ?? []),
]);

async function setMode(index: number, mode: Loadpoint["mode"]) {
	try {
		await post(`loadpoints/${index + 1}/mode/${mode}`);
	} catch (e) {
		console.error(e);
	}
}
</script>

<style scoped>
.list {
	list-style: none;
	margin: 0;
	padding: 0;
}

.list li {
	display: flex;
	justify-content: space-between;
	padding: 10px 0;
}

.list li + li {
	border-top: 1px solid var(--border);
}
</style>
