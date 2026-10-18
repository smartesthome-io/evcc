<template>
	<div class="tile">
		<div class="label">{{ label }}</div>
		<div class="value" :class="{ none: value === null }">{{ value ?? "–" }}</div>
		<div v-if="hint" class="hint muted">{{ hint }}</div>
	</div>
</template>

<script setup lang="ts">
defineProps<{ label: string; value: string | null; hint?: string }>();
</script>

<style scoped>
.tile {
	min-width: 0;
}

.label {
	font-size: 0.8rem;
	color: var(--muted);
	font-weight: 600;
}

.value {
	font-size: 1.6rem;
	font-weight: 700;
	line-height: 1.2;
	margin-top: 2px;
	white-space: nowrap;
}

.value.none {
	color: var(--muted);
}

.hint {
	font-size: 0.8rem;
	margin-top: 2px;
}
</style>
