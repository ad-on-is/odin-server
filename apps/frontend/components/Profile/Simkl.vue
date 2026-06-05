<template>
	<div v-if="profile !== null" class="card w-96 bg-base-100 shadow-xl">
		<div class="card-body">
			<h2 class="card-title">SIMKL</h2>
			<p>{{ profile.user.username }}<br />{{ profile.user.name }}</p>
			<div class="card-actions justify-end">
				<button class="btn btn-sm" @click="refreshHistory()">Refresh History <span class="loading loading-spinner loading-sm" v-if="loading"></span></button>
			</div>
		</div>
	</div>
	<div v-else class="card w-96 bg-base-100 shadow-xl">
		<dialog ref="login_dialog" class="modal">
			<div class="modal-box">
				<h3 class="font-bold text-lg">Login to SIMKL</h3>
				<p class="py-4">
					Go to: <a :href="url">{{ url }}</a>
				</p>
				<p class="py-4">Enter code:</p>
				<p>{{ user_code }}</p>
			</div>
		</dialog>
		<div class="card-body">
			<h2 class="card-title">SIMKL</h2>
			<p>Click below to login into SIMKL</p>
			<div class="card-actions justify-end">
				<button class="btn btn-sm" @click="login()">Login</button>
			</div>
		</div>
	</div>
</template>

<script lang="ts" setup>
	const login_dialog = ref<HTMLDialogElement>()
	const user_code = ref<string>()
	const url = ref<string>()
	const device_code = ref<string>()
	const profile = ref(null)
	const loading = ref(false)

	async function getProfile() {
		try {
			profile.value = await usePb().send('/-/simkl/users/settings', {
				method: 'GET',
			})
		} catch (e) {
			console.log(e)
		}
	}

	async function refreshHistory() {
		loading.value = true
		// await usePb().send('/-/refreshHistory', { method: 'get' })
		loading.value = false
	}

	onMounted(async () => {
		getProfile()
	})
	async function login() {
		const secrets = await usePb().send('/-/simklsecrets', { method: 'get' })
		const cid = secrets['SIMKL_CLIENTID']

		login_dialog.value?.showModal()
		const res = await usePb().send(`/-/simkl/oauth/pin?client_id=${cid}&fresh=true`, {
			method: 'GET',
		})

		url.value = res.verification_url
		user_code.value = res.user_code

		const poll = setInterval(async () => {
			const res = await usePb().send(`/-/simkl/oauth/pin/${user_code.value}?client_id=${cid}&fresh=true`, {
				method: 'GET',
			})
			if (res !== null && res['result'] === 'OK') {
				console.log(usePb().authStore.model?.id, res)
				await usePb().collection('users').update(usePb().authStore.model?.id, { simkl_token: res })
				getProfile()
				clearInterval(poll)
				login_dialog.value?.close()
			}
		}, 5000)
	}
</script>
