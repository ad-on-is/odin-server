export const useMedia = defineStore('useMedia', () => {
	const detail = ref<{ [id: string]: any }>({})

	const lists = ref<{ [id: string]: [] }>({})

	async function getDetail(id: string, type: string) {
		const trakt = id.split('-').at(-1) || ''
		if (!detail.value[id] && id) {
			const res = await usePb().send(`/-/trakt/search/trakt/${trakt}?type=${type}`, { method: 'GET' })
			if (res.length > 0) {
				const item = res[0]
				if (item.type === 'show') {
					item.seasons = await usePb().send(`/-/traktseasons/${item.ids.trakt}`, { method: 'GET' })
				}
				detail.value[id] = item
			}
		} else {
			if (detail.value[id].type === 'show') {
				if (!detail.value[id].seasons) {
					detail.value[id].seasons = await usePb().send(`/-/traktseasons/${detail.value[id].ids.trakt}`, { method: 'GET' })
				}
			}
		}
		return detail.value[id]
	}

	async function search(term: string) {
		if (term.length < 2) {
			return []
		}
		const movies = await usePb().send(`/-/trakt/search/movie?query=${term}`, { method: 'GET' })
		const shows = await usePb().send(`/-/trakt/search/show?query=${term}`, { method: 'GET' })

		return { movies, shows }
	}

	const getId = (item: any) => {
		if (item['type'] === 'episode') {
			return `${item['show']['ids']['slug']}-${item['show']['ids']['trakt']}`
		}
		return `${item['ids']['slug']}-${item['ids']['trakt']}`
	}

	const getTmdb = async (typ: string, id: number) => {
		const res = await usePb().send(`/-/tmdbdetail/${typ}/${id}`, { method: 'GET' })
		return res
	}

	const getLink = (item: any) => {
		if (item['type'] === 'episode') {
			return `/show/${getId(item)}`
		}
		return `/${item['type']}/${getId(item)}`
	}

	const setDetail = (item: any) => {
		if (!detail.value[getId(item)]) {
			if (item.type === 'episode') {
				detail.value[getId(item)] = item.show
			} else {
				detail.value[getId(item)] = item
			}
		}
	}

	const getList = async (url: string) => {
		if (!lists.value[url]) {
			try {
				const res = await usePb().send(`/-/trakt${url}`, {
					method: 'GET',
					cache: 'no-cache',
				})
				lists.value[url] = res
			} catch (e) {
				lists.value[url] = []
			}
		}
		return lists.value[url]
	}

	return {
		setDetail,
		getDetail,
		getTmdb,
		getList,
		getLink,
		detail,
		search,
	}
})

if (import.meta.hot) {
	import.meta.hot.accept(acceptHMRUpdate(useMedia, import.meta.hot))
}
