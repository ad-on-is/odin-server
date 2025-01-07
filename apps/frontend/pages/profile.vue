<template>
	<div class="flex flex-row">
		<ProfileTrakt />
		<div class="card w-96 bg-base-100 shadow-xl">
			<div class="card-body">
				<h2 class="card-title">Server</h2>
				<p>Version: {{ serverVersion }}</p>
			</div>
		</div>
	</div>
</template>

<script setup lang="ts">
	const serverVersion = ref('')

	onMounted(async () => {
		const data = await usePb().send('/-/health', {
			method: 'get',
			cache: 'no-cache',
		})
		serverVersion.value = data.version
	})
</script>
