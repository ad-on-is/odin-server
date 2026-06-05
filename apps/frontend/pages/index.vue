<template>
	<div>
		<div v-for="e in list">
			<h2>{{ parseTitle(e.title) }}</h2>
			<MediaList :url="e.url" />
		</div>
	</div>
</template>
<script lang="ts" setup>
	if (usePb().authStore.isAdmin) {
		navigateTo('/admin')
	}
	const list = useProfile().me['simkl_sections']['home']
</script>
