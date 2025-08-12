<template>
	<div
		v-if="item"
		class="bg-cover min-h-screen max-h-screen relative faded"
		:style="{
			backgroundImage: 'url(' + item['images']['fanart'] + ')',
			backgroundPosition: 'center',
			backgroundRepeat: 'no-repeat',
			backgroundSize: 'cover',
		}"
	>
		<div class="md:container md:mx-auto z-20 relative">
			<DetailInfo :item="item" />
			<button v-if="item.type === 'movie'" class="btn btn-success" @click="useStreams().openModal(item)"><FaIcon icon="play-circle" size="xl" />Play</button>
			<Streams />
			<Video />

			<DetailSeasonsAndEpisodes :item="item" v-if="useRoute().params.type === 'show'" />
		</div>
	</div>
	<div v-else>Loading...</div>
</template>

<script lang="tsx" setup>
	definePageMeta({
		layout: 'detail',
	})
	const item = ref()
	onMounted(async () => {
		item.value = await useMedia().getDetail(useRoute().params.id as string, useRoute().params.type as string)
		item.value.tmdb = await useMedia().getTmdb(item.value.type, item.value.ids['tmdb'])
	})
</script>

<style>
	.faded:after {
		content: ' ';
		position: absolute;
		top: 0;
		left: 0;
		background: linear-gradient(-150deg, #1e1e2e00 0%, #1e1e2e00 20%, #1e1e2edd 40%, #1e1e2efa 60%, #1e1e2e 100%);
		width: 100%;
		height: 100%;
	}
</style>
