type EpisodeStream = {
	type: string

	show_imdb?: string
	show_tvdb?: string
	show_title: string
	show_year: string
	season_number: string
	season_aired?: string
	episode_trakt?: string
	episode_imdb?: string
	episode_tvdb?: string
	episode_title: string
	episode_number: string
	episode_year: string
	no_seasons?: string
	country?: string
}

type MovieStream = {
	trakt: string
	imdb?: string
	title: string
	year: string
	type: string
}
import mqtt from 'mqtt'

type Stream = MovieStream | EpisodeStream

export const useStreams = defineStore('useStreams', () => {
	const list = ref<{ [id: string]: [] }>({})

	const data = ref<Stream>()
	const streams = ref<any[]>([])
	const topic = ref('')

	const videoUrl = ref('')
	const proto = location.protocol.includes('https') ? 'wss' : 'ws'
	const url = `${proto}://${location.host}/ws/mqtt`
	const mqttClient = mqtt.connect(url)

	mqttClient.on('connect', () => {
		console.log('MQTT', mqttClient?.connected, url)
	})
	mqttClient.on('error', (e) => {
		console.error('MQTT', e.message)
	})
	mqttClient.on('disconnect', () => {
		console.log('MQTT', mqttClient?.connected, url)
	})

	async function getStreams() {
		streams.value = []
		if (!data.value) return []
		let id = ''
		if (data.value.type === 'movie') {
			id = `${(data.value as MovieStream).title}-${(data.value as MovieStream).year}`
			topic.value = `odin-movieshow/movie/${(data.value as MovieStream).trakt}`
		} else {
			id = `${(data.value as EpisodeStream).show_title}-${(data.value as EpisodeStream).season_number}-${(data.value as EpisodeStream).episode_number}`
			topic.value = `odin-movieshow/episode/${(data.value as EpisodeStream).episode_trakt}`
		}

		console.log('MQTT', topic.value)
		mqttClient.subscribe(topic.value)
		mqttClient.on('message', (topic: string, message: Buffer) => {
			try {
				const m = JSON.parse(message.toString())
				streams.value = [...streams.value, m]
			} catch (_) {}
		})

		if (!list.value[id]) {
			list.value[id] = await usePb().send('/-/scrape', {
				method: 'POST',
				body: data.value,
				cache: 'no-cache',
			})
		}
		return list.value[id]
	}

	const triggerModal = ref(false)
	const triggerVideoModal = ref(false)
	const openModal = (item: any, show?: any, season?: string) => {
		data.value = {
			type: 'movie',
			trakt: `${item.title}`,
			imdb: `${item.ids?.imdb || ''}`,
			title: `${item.title}`,
			year: `${item.year}`,
		} as MovieStream
		if (show) {
			data.value = {
				type: 'episode',
				show_imdb: `${show.ids.imdb}`,
				show_tvdb: `${show.ids.tvdb}`,
				show_title: `${show.title}`,
				show_year: `${show.year}`,
				season_number: `${item.season}`,
				episode_imdb: `${item.ids?.imdb || ''}`,
				episode_trakt: `${item.title}`,
				episode_tvdb: `${item.ids?.tvdb || ''}`,
				episode_title: `${item.title}`,
				episode_number: `${item.number}`,
				episode_year: `${item.year}`,
				season_aired: `${show.year}`,
				no_seasons: '10',
				country: '',
			} as EpisodeStream
		}

		triggerModal.value = !triggerModal.value
	}
	const openVideoModal = (url: string) => {
		videoUrl.value = url
		triggerVideoModal.value = !triggerVideoModal.value
	}
	return {
		getStreams,
		triggerModal,
		openModal,
		triggerVideoModal,
		openVideoModal,
		videoUrl,
		mqttClient,
		streams,
		topic,
	}
})
