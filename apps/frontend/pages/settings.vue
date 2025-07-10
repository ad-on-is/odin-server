<template>
	<div>
		<button class="btn btn-sm btn-primary mt-10" @click="saveSections()"><FaIcon icon="floppy-disk" /> Save sections</button>
		<div class="grid grid-cols-2 gap-10 mt-10">
			<div>
				<section v-for="(_, k) in sections" class="border border-dashed border-slate-700 mb-5 p-4">
					<h3 class="m-0 mb-3">{{ k.toUpperCase() }}</h3>
					<draggable v-model="sections[k]" :group="k" item-key="id" handle=".handle" ghost-class="ghost">
						<template #item="{ element, index }">
							<div class="flex py-2 bg-black bg-opacity-20 px-2 mb-2">
								<FaIcon icon="bars" class="handle mt-2 mr-2 opacity-30" size="sm" />
								<input class="input input-sm input-bordered mr-2 flex-1" v-model="element.title" placeholder="Title" />
								<input class="input input-sm input-bordered flex-grow" placeholder="URL" v-model="element.url" />
								<label class="cursor-pointer label">
									<span class="label-text mr-2">Big</span>
									<input type="checkbox" v-model="element.big" class="checkbox checkbox-secondary checkbox-sm" />
								</label>
								<label class="cursor-pointer label">
									<span class="label-text mr-2">Cache</span>
									<input type="checkbox" v-model="element.cache" class="checkbox checkbox-secondary checkbox-sm" />
								</label>

								<label class="cursor-pointer label">
									<span class="label-text mr-2">Paginate</span>
									<input type="checkbox" v-model="element.paginate" class="checkbox checkbox-secondary checkbox-sm" />
								</label>
								<button class="btn btn-sm ml-2" @click="deleteSection(k, index)">
									<FaIcon icon="trash" />
								</button>
							</div>
						</template>
					</draggable>

					<button class="btn btn-sm btn-accent" @click="addSection(k)">Add</button>
				</section>
			</div>
			<div>
				<h2>Help</h2>
				<p>Please see <a href="https://trakt.docs.apiary.io/" target="_blank">Trakt API</a> for reference.</p>
				<h3>Placeholders</h3>
				<pre>
::(year|month|day):: current year|month|day
::(year|month|day):-1: current year|month|day +1 (or -1)
::monthdays:: days of the current month</pre
				>
				Example: <strong>/movies/popular?years=::year::,::year:-1:</strong> -> <i>/movies/popular?years=2024,2023</i>
			</div>
		</div>
	</div>
</template>

<style scoped>
	.ghost {
		@apply bg-primary bg-opacity-10;
	}
</style>

<script lang="ts" setup>
	const me = useProfile().me
	let sections = me['trakt_sections']
	import draggable from 'vuedraggable'

	const templates = [
		{ title: 'Movies', url: '/movies', big: false, paginate: false, cache: true },
		{ title: 'Shows', url: '/movies', big: false, paginate: false, cache: true },
	]

	if (sections === null) {
		sections = {
			home: [],
			movies: [],
			shows: [],
		}
		usePb().collection('users').update(me.id, { trakt_sections: sections })
	}
	function addSection(place: any) {
		sections[place].push({
			title: '',
			url: '',
			big: false,
			cache: true,
			paginate: false,
		})
	}

	function deleteSection(place: any, index: number) {
		console.log(place, index)
		sections[place].splice(index, 1)
	}

	function saveSections() {
		usePb().collection('users').update(me.id, { trakt_sections: sections })
	}
</script>
