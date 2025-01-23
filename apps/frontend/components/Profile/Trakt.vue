<template>
  <div v-if="traktProfile !== null" class="card w-96 bg-base-100 shadow-xl">
    <div class="card-body">
      <h2 class="card-title">Trakt</h2>
      <p>{{ traktProfile.user.username }}<br />{{ traktProfile.user.name }}</p>
      <div class="card-actions justify-end">
        <button class="btn btn-sm" @click="refreshHistory()">Refresh History <span
            class="loading loading-spinner loading-sm" v-if="loading"></span></button>
      </div>

    </div>
  </div>
  <div v-else class="card w-96 bg-base-100 shadow-xl">
    <dialog ref="login_dialog" class="modal">
      <div class="modal-box">
        <h3 class="font-bold text-lg">Login to Trakt</h3>
        <p class="py-4">
          Go to: <a :href="url">{{ url }}</a>
        </p>
        <p class="py-4">Enter code:</p>
        <p>{{ user_code }}</p>
      </div>
    </dialog>
    <div class="card-body">
      <h2 class="card-title">Trakt</h2>
      <p>Click below to login into Trakt</p>
      <div class="card-actions justify-end">
        <button class="btn btn-sm" @click="traktLogin()">Login</button>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
const login_dialog = ref<HTMLDialogElement>()
const user_code = ref<string>()
const url = ref<string>()
const device_code = ref<string>()
const traktProfile = ref(null)
const loading = ref(false)

async function getProfile() {
  try {
    traktProfile.value = await usePb().send('/-/trakt/users/settings', {
      method: 'GET',
    })
  } catch (e) {
    console.log(e)
  }
}

async function refreshHistory() {
  loading.value = true
  await usePb().send('/-/refreshHistory', { method: 'get' })
  loading.value = false
}

onMounted(async () => {
  getProfile()
})
async function traktLogin() {
  const secrets = await usePb().send('/-/secrets', { method: 'get' })

  login_dialog.value?.showModal()
  const res = await usePb().send('/-/trakt/oauth/device/code?fresh=true', {
    method: 'POST',
    body: {
      client_id: secrets['TRAKT_CLIENTID'],
    },
  })

  url.value = res.verification_url
  user_code.value = res.user_code
  device_code.value = res.device_code

  const poll = setInterval(async () => {
    const res = await usePb().send('/-/trakt/oauth/device/token?fresh=true', {
      method: 'POST',
      body: {
        client_id: secrets['TRAKT_CLIENTID'],
        client_secret: secrets['TRAKT_SECRET'],
        code: device_code.value,
      },
    })
    if (res !== null) {
      console.log(usePb().authStore.model?.id, res)
      await usePb()
        .collection('users')
        .update(usePb().authStore.model?.id, { trakt_token: { ...res, device_code: device_code.value } })
      getProfile()
      clearInterval(poll)
      login_dialog.value?.close()
    }
  }, 5000)
}
</script>
