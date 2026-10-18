<template>
	<div class="page">
		<section class="card" data-testid="today-placeholder">
			<h2>{{ t("soon.title") }}</h2>
			<p class="muted">{{ t("tab.today") }}</p>
		</section>
	</div>
</template>

<script setup lang="ts">
import { t } from "../i18n";
</script>
