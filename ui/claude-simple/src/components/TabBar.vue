<template>
	<nav class="tabs" aria-label="Main">
		<a
			v-for="tab in tabs"
			:key="tab.id"
			:href="`#/${tab.id}`"
			class="tab"
			:class="{ active: modelValue === tab.id }"
			:aria-current="modelValue === tab.id ? 'page' : undefined"
		>
			<svg viewBox="0 0 24 24" aria-hidden="true" class="icon">
				<path :d="tab.icon" />
			</svg>
			<span>{{ tab.label }}</span>
		</a>
	</nav>
</template>

<script setup lang="ts">
import { t } from "../i18n";

defineProps<{ modelValue: string }>();

const tabs = [
	{ id: "now", label: t("tab.now"), icon: "M13 2 4 14h6l-1 8 9-12h-6l1-8Z" },
	{
		id: "today",
		label: t("tab.today"),
		icon: "M7 2v2H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6a2 2 0 0 0-2-2h-2V2h-2v2H9V2H7Zm-2 8h14v10H5V10Z",
	},
	{
		id: "config",
		label: t("tab.config"),
		icon: "M19.4 13a7.6 7.6 0 0 0 0-2l2-1.6-2-3.4-2.4 1a7 7 0 0 0-1.7-1L15 3h-4l-.4 3a7 7 0 0 0-1.7 1l-2.4-1-2 3.4 2 1.6a7.6 7.6 0 0 0 0 2l-2 1.6 2 3.4 2.4-1a7 7 0 0 0 1.7 1l.4 3h4l.4-3a7 7 0 0 0 1.7-1l2.4 1 2-3.4-2-1.6ZM13 15.5a3.5 3.5 0 1 1 0-7 3.5 3.5 0 0 1 0 7Z",
	},
];
</script>

<style scoped>
.tabs {
	position: fixed;
	left: 0;
	right: 0;
	bottom: 0;
	z-index: 10;
	display: flex;
	background: var(--surface);
	border-top: 1px solid var(--border);
	padding-bottom: env(safe-area-inset-bottom);
}

.tab {
	flex: 1;
	display: flex;
	flex-direction: column;
	align-items: center;
	gap: 2px;
	padding: 10px 0 8px;
	color: var(--muted);
	text-decoration: none;
	font-size: 0.75rem;
	font-weight: 600;
}

.tab.active {
	color: var(--accent);
}

.icon {
	width: 24px;
	height: 24px;
	fill: currentColor;
}

@media (min-width: 768px) {
	.tabs {
		position: sticky;
		top: 0;
		bottom: auto;
		justify-content: center;
		gap: 8px;
		border-top: 0;
		border-bottom: 1px solid var(--border);
		padding: 0;
	}

	.tab {
		flex: 0 0 auto;
		flex-direction: row;
		gap: 8px;
		padding: 14px 20px;
		font-size: 0.9rem;
		border-bottom: 2px solid transparent;
	}

	.tab.active {
		border-bottom-color: var(--accent);
	}

	.icon {
		width: 20px;
		height: 20px;
	}
}
</style>
