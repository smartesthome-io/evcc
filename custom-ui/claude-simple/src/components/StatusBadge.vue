<template>
	<div class="status" role="status" data-testid="status">
		<span class="badge" :class="result.primary">
			<span class="dot" aria-hidden="true"></span>
			{{ t(`status.${result.primary}` as Key) }}
		</span>
		<span v-for="chip in result.chips" :key="chip" class="chip">
			{{ t(`status.${chip}` as Key) }}
		</span>
	</div>
</template>

<script setup lang="ts">
import { t, type Key } from "../i18n";
import type { SiteStatusResult } from "../status";

defineProps<{ result: SiteStatusResult }>();
</script>

<style scoped>
.status {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: 8px;
}

.badge {
	display: inline-flex;
	align-items: center;
	gap: 10px;
	padding: 10px 18px;
	border-radius: 999px;
	font-weight: 700;
	font-size: 1.1rem;
	background: color-mix(in srgb, var(--tone) 16%, var(--surface));
	color: var(--tone);
	border: 1px solid color-mix(in srgb, var(--tone) 40%, transparent);
	--tone: var(--muted);
}

.dot {
	width: 10px;
	height: 10px;
	border-radius: 50%;
	background: currentColor;
}

.charging-car {
	--tone: var(--car);
}
.discharging-battery,
.charging-battery {
	--tone: var(--battery);
}
.grid-export {
	--tone: var(--export);
}
.grid-import {
	--tone: var(--grid);
}

.chip {
	padding: 4px 12px;
	border-radius: 999px;
	font-size: 0.8rem;
	font-weight: 600;
	color: var(--muted);
	border: 1px solid var(--border);
	background: var(--surface);
}
</style>
