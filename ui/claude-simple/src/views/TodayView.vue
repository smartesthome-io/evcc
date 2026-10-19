<template>
	<div class="page">
		<p v-if="summary.updated" class="muted updated">
			{{ t("today.updated") }} {{ timeFmt.format(summary.updated) }}
		</p>

		<section v-if="error" class="card" role="alert">
			<h2>{{ t("today.error") }}</h2>
			<p class="muted">{{ error }}</p>
		</section>

		<p v-else-if="loaded && !summary.updated" class="muted empty" data-testid="today-empty">
			{{ t("today.empty") }}
		</p>

		<template v-if="!error && summary.updated">
			<section class="card" aria-labelledby="solar-title">
				<h2 id="solar-title">{{ t("today.solar") }}</h2>
				<div class="tiles">
					<StatTile
						:label="t('today.forecast')"
						:value="kwh(summary.forecastDay)"
						:hint="summary.forecastDay === null ? t('today.noForecast') : undefined"
					/>
					<StatTile :label="t('today.produced')" :value="kwh(summary.produced)" />
					<StatTile :label="t('today.remaining')" :value="kwh(summary.remaining)" />
				</div>

				<div v-if="summary.accuracy !== null" class="accuracy">
					<StatTile
						:label="t('today.accuracy')"
						:value="`${Math.round(summary.accuracy)} %`"
						:hint="accuracyHint"
					/>
					<CumulativeChart :curve="summary.curve" />
				</div>
			</section>

			<section class="card" aria-labelledby="car-title">
				<h2 id="car-title">{{ t("today.car") }}</h2>
				<div class="tiles two">
					<StatTile :label="t('today.sessions')" :value="String(summary.carSessions)" />
					<StatTile :label="t('today.carEnergy')" :value="kwh(summary.carEnergy)" />
				</div>
			</section>

			<section class="card" aria-labelledby="grid-title">
				<h2 id="grid-title">{{ t("today.grid") }}</h2>
				<div class="tiles two">
					<StatTile :label="t('today.import')" :value="kwh(summary.gridImport)" />
					<StatTile :label="t('today.export')" :value="kwh(summary.gridExport)" />
				</div>
			</section>

			<section v-if="state.battery" class="card" aria-labelledby="battery-title">
				<h2 id="battery-title">{{ t("today.battery") }}</h2>
				<div class="tiles two">
					<StatTile
						:label="t('today.batteryCharged')"
						:value="kwh(summary.batteryCharge)"
					/>
					<StatTile
						:label="t('today.batteryDischarged')"
						:value="kwh(summary.batteryDischarge)"
					/>
				</div>
			</section>
		</template>
	</div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import StatTile from "../components/StatTile.vue";
import CumulativeChart from "../components/CumulativeChart.vue";
import { get, state } from "../state";
import { fmtEnergy } from "../format";
import { t } from "../i18n";
import { startOfDay, summarizeToday, type HistorySeries, type SessionRecord } from "../today";

const timeFmt = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" });

const history = ref<HistorySeries[] | null>(null);
const sessions = ref<SessionRecord[]>([]);
const error = ref("");
const loaded = ref(false);
const now = ref(new Date());

const heating = computed(() =>
	state.loadpoints.filter((lp) => lp.chargerFeatureHeating).map((lp) => lp.title)
);

const summary = computed(() =>
	summarizeToday(history.value, sessions.value, {
		now: now.value,
		heating: heating.value,
		remaining: state.forecast?.solar?.today?.energy ?? null,
	})
);

const kwh = (v: number | null) => (v === null ? null : fmtEnergy(v));

const accuracyHint = computed(() => {
	const a = summary.value.accuracy;
	if (a === null) return undefined;
	const diff = Math.round(Math.abs(a - 100));
	if (diff === 0) return t("today.accuracyOf");
	return `${diff} % ${a > 100 ? t("today.above") : t("today.below")}`;
});

let seq = 0;

async function load() {
	const id = ++seq;
	const today = startOfDay(new Date());
	const query = new URLSearchParams({
		from: today.toISOString(),
		aggregate: "15m",
		grouped: "false",
	});

	const [h, s] = await Promise.allSettled([
		get<HistorySeries[] | null>(`history/energy?${query}`),
		get<SessionRecord[] | null>(
			`sessions?year=${today.getFullYear()}&month=${today.getMonth() + 1}`
		),
	]);

	// a newer request has been started meanwhile
	if (id !== seq) return;

	now.value = new Date();
	if (h.status === "fulfilled") {
		history.value = h.value;
		error.value = "";
	} else {
		error.value = String(h.reason?.message ?? h.reason);
	}
	sessions.value = s.status === "fulfilled" ? (s.value ?? []) : [];
	loaded.value = true;
}

let timer: number | undefined;
let day = startOfDay(new Date()).getTime();

onMounted(() => {
	load();
	// pick up the day change
	timer = window.setInterval(() => {
		const current = startOfDay(new Date()).getTime();
		if (current !== day) {
			day = current;
			load();
		}
	}, 60_000);
});

onBeforeUnmount(() => window.clearInterval(timer));

watch(() => state.historyUpdated, load);
</script>

<style scoped>
.updated {
	margin: 0;
	font-size: 0.85rem;
}

.empty {
	text-align: center;
	margin: 32px 0;
}

.tiles {
	display: grid;
	grid-template-columns: repeat(3, minmax(0, 1fr));
	gap: 16px;
}

.tiles.two {
	grid-template-columns: repeat(2, minmax(0, 1fr));
}

.accuracy {
	margin-top: 20px;
	padding-top: 16px;
	border-top: 1px solid var(--border);
	display: flex;
	flex-direction: column;
	gap: 8px;
}

@media (max-width: 420px) {
	.tiles {
		grid-template-columns: 1fr 1fr;
	}
}
</style>
