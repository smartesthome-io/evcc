<template>
	<div class="page">
		<section class="card">
			<h2>{{ t("tab.config") }}</h2>
			<p class="muted">{{ t("config.hint") }}</p>
			<a class="link" href="../classic/#/config">{{ t("config.classic") }}</a>
		</section>
	</div>
</template>

<script setup lang="ts">
import { t } from "../i18n";
</script>

<style scoped>
.link {
	display: inline-block;
	margin-top: 8px;
	padding: 10px 18px;
	border-radius: 999px;
	background: var(--accent);
	color: #fff;
	font-weight: 600;
	text-decoration: none;
}
</style>
