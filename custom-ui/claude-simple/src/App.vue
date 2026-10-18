<template>
	<TabBar :model-value="route" />
	<div v-if="state.offline" class="offline" role="alert">{{ t("offline") }}</div>
	<header class="title">
		<h1>{{ state.siteTitle || "evcc" }}</h1>
		<a class="classic" href="../classic/">{{ t("classic") }}</a>
	</header>
	<main>
		<NowView v-if="route === 'now'" />
		<TodayView v-else-if="route === 'today'" />
		<ConfigView v-else />
	</main>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";
import TabBar from "./components/TabBar.vue";
import NowView from "./views/NowView.vue";
import TodayView from "./views/TodayView.vue";
import ConfigView from "./views/ConfigView.vue";
import { state } from "./state";
import { t } from "./i18n";

const ROUTES = ["now", "today", "config"];

function parse(): string {
	const id = window.location.hash.replace(/^#\/?/, "").split(/[/?]/)[0] ?? "";
	return ROUTES.includes(id) ? id : "now";
}

const route = ref(parse());
const onHash = () => (route.value = parse());

onMounted(() => window.addEventListener("hashchange", onHash));
onBeforeUnmount(() => window.removeEventListener("hashchange", onHash));
</script>

<style scoped>
.title {
	max-width: 880px;
	margin: 0 auto;
	padding: 20px 16px 0;
	display: flex;
	justify-content: space-between;
	align-items: baseline;
	gap: 12px;
}

h1 {
	font-size: 1.4rem;
}

.classic {
	font-size: 0.8rem;
	color: var(--muted);
}

.offline {
	background: var(--danger);
	color: #fff;
	text-align: center;
	padding: 8px 16px;
	font-weight: 600;
	font-size: 0.9rem;
}
</style>
