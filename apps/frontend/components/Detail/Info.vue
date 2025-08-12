<template>
	<div>
		<div class="grid grid-cols-2">
			<div class="">
				<p v-if="item.tmdb" class="text-sm pt-10">{{ item.tmdb.production_companies.at(0).name }}</p>
				<h1 class="m-0">{{ item.title }}</h1>
				<h2 class="m-0 mt-2 mb-10">
					{{ item.year }} | {{ item.runtime }} |
					{{ item.genres.map((g: string) => g.at(0)?.toUpperCase() + g.slice(1, g.length)).join(', ') }}
					|
					{{ item.language.toUpperCase() }}
				</h2>
				<h3 v-if="item.tagline">{{ item.tagline }}</h3>
				<p>{{ item.overview }}</p>
			</div>
		</div>
		<div v-if="item.tmdb">
			<h3>Cast & Characters</h3>
			<div class="flex gap-8 overflow-y-scroll">
				<div v-for="actor in item.tmdb.credits.cast" class="flex flex-col items-center">
					<div class="rounded-full w-24 h-24 bg-center bg-cover" :style="{ backgroundImage: `url(${'https://image.tmdb.org/t/p/w185' + actor.profile_path}` }"></div>

					<p class="text-sm text-center">
						{{ actor.name }}<br /><span class="text-xs opacity-50">{{ actor.character }}</span>
					</p>
				</div>
			</div>
		</div>
	</div>
</template>

<script lang="ts" setup>
	const props = defineProps({
		item: {
			type: Object,
			required: true,
		},
	})
	const item = props.item
</script>
