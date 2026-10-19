<template>
	<section class="card" aria-labelledby="battery-title" data-testid="battery">
		<h2 id="battery-title">{{ t("battery.title") }}</h2>
		<div class="row">
			<div class="soc">{{ Math.round(battery.soc) }}<small>%</small></div>
			<div class="info">
				<div
					class="bar"
					role="progressbar"
					:aria-valuenow="Math.round(battery.soc)"
					aria-valuemin="0"
					aria-valuemax="100"
				>
					<span :style="{ width: `${Math.min(Math.max(battery.soc, 0), 100)}%` }"></span>
				</div>
				<div class="detail">
					<strong>{{ stateLabel }}</strong>
					<span v-if="Math.abs(battery.power) >= 50" class="muted">{{
						fmtPower(Math.abs(battery.power))
					}}</span>
					<span v-if="battery.capacity" class="muted"
						>· {{ battery.capacity.toFixed(1) }} kWh</span
					>
				</div>
			</div>
		</div>
	</section>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { Battery } from "../types";
import { fmtPower } from "../format";
import { t } from "../i18n";

const props = defineProps<{ battery: Battery }>();

const stateLabel = computed(() => {
	if (props.battery.power > 50) return t("battery.discharging");
	if (props.battery.power < -50) return t("battery.charging");
	return t("battery.standby");
});
</script>

<style scoped>
.row {
	display: flex;
	align-items: center;
	gap: 20px;
}

.soc {
	font-size: 3rem;
	font-weight: 700;
	line-height: 1;
	min-width: 5.5rem;
}

.soc small {
	font-size: 1.2rem;
	font-weight: 600;
	color: var(--muted);
	margin-left: 2px;
}

.info {
	flex: 1;
	min-width: 0;
}

.bar > span {
	background: var(--battery);
}

.detail {
	margin-top: 8px;
	font-size: 0.95rem;
	display: flex;
	flex-wrap: wrap;
	gap: 0 6px;
}
</style>
