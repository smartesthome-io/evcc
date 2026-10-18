<template>
	<article class="row" data-testid="loadpoint">
		<div class="head">
			<div class="titles">
				<h3>{{ loadpoint.title }}</h3>
				<p class="muted sub">{{ subline }}</p>
			</div>
			<div class="power" :class="{ active: powerActive }">
				{{ powerActive ? fmtPower(loadpoint.chargePower) : stateLabel }}
			</div>
		</div>

		<div v-if="hasSoc" class="soc">
			<div
				class="bar"
				role="progressbar"
				:aria-valuenow="Math.round(loadpoint.vehicleSoc)"
				aria-valuemin="0"
				aria-valuemax="100"
			>
				<span
					:style="{ width: `${Math.min(Math.max(loadpoint.vehicleSoc, 0), 100)}%` }"
				></span>
			</div>
			<span class="value">{{ socText }}</span>
		</div>

		<div class="modes" role="group" :aria-label="loadpoint.title">
			<button
				v-for="m in MODES"
				:key="m"
				type="button"
				:class="{ on: loadpoint.mode === m }"
				:aria-pressed="loadpoint.mode === m"
				@click="$emit('mode', m)"
			>
				{{ t(`mode.${m}` as Key) }}
			</button>
		</div>
	</article>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { Loadpoint } from "../types";
import { fmtDuration, fmtPower } from "../format";
import { t, type Key } from "../i18n";

const props = defineProps<{ loadpoint: Loadpoint }>();
defineEmits<{ mode: [mode: Loadpoint["mode"]] }>();

const MODES: Loadpoint["mode"][] = ["off", "smart", "now"];

const heating = computed(() => props.loadpoint.chargerFeatureHeating);
const powerActive = computed(() => props.loadpoint.chargePower >= 50);

const stateLabel = computed(() => {
	if (heating.value) return t("heating.standby");
	return props.loadpoint.connected ? t("loadpoints.connected") : t("loadpoints.disconnected");
});

const hasSoc = computed(
	() => (heating.value || props.loadpoint.connected) && props.loadpoint.vehicleSoc > 0
);

const socText = computed(() => {
	const lp = props.loadpoint;
	const value = heating.value
		? `${Math.round(lp.vehicleSoc)} °C`
		: `${Math.round(lp.vehicleSoc)} %`;
	return !heating.value && lp.vehicleRange > 0
		? `${value} · ${Math.round(lp.vehicleRange)} km`
		: value;
});

const subline = computed(() => {
	const lp = props.loadpoint;
	if (heating.value) return powerActive.value ? t("heating.running") : t("heating.standby");
	if (!lp.connected) return t("loadpoints.disconnected");
	const name = lp.vehicleTitle || lp.vehicleName || t("loadpoints.noVehicle");
	const remaining = fmtDuration(lp.chargeRemainingDuration);
	return remaining && lp.charging ? `${name} · ${remaining} ${t("loadpoints.remaining")}` : name;
});
</script>

<style scoped>
.row {
	padding: 14px 0;
	display: flex;
	flex-direction: column;
	gap: 12px;
}

.row + .row {
	border-top: 1px solid var(--border);
}

.head {
	display: flex;
	justify-content: space-between;
	align-items: flex-start;
	gap: 12px;
}

h3 {
	font-size: 1.05rem;
}

.sub {
	margin: 2px 0 0;
	font-size: 0.85rem;
}

.power {
	font-weight: 700;
	white-space: nowrap;
	color: var(--muted);
}

.power.active {
	color: var(--accent);
	font-size: 1.1rem;
}

.soc {
	display: flex;
	align-items: center;
	gap: 12px;
}

.soc .bar {
	flex: 1;
}

.value {
	font-size: 0.85rem;
	font-weight: 600;
	white-space: nowrap;
}

.modes {
	display: inline-flex;
	align-self: flex-start;
	border: 1px solid var(--border);
	border-radius: 999px;
	overflow: hidden;
}

.modes button {
	border: 0;
	background: transparent;
	padding: 6px 16px;
	font-size: 0.85rem;
	font-weight: 600;
	color: var(--muted);
	cursor: pointer;
}

.modes button.on {
	background: var(--accent);
	color: #fff;
}

.modes button:focus-visible {
	outline: 2px solid var(--accent);
	outline-offset: -2px;
}
</style>
