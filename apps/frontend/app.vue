<template>
  <div id="app" data-theme="odin">
    <div v-if="loading"><span class="loading loading-infinity loading-lg text-success"></span></div>
    <NuxtLayout v-else>
      <NuxtPage />
    </NuxtLayout>
  </div>

</template>

<script lang="ts" setup>
const loading = ref(false)
onMounted(async () => {
  loading.value = true
  // await useSettings().initConfig()
  loading.value = false
})

</script>

<style>
.page-enter-active,
.page-leave-active {
  transition: all 0.2s;
}

.page-enter-from,
.page-leave-to {
  opacity: 0;
  filter: blur(1rem);
}

.layout-enter-active,
.layout-leave-active {
  transition: all 0.2s;
}



.layout-enter-from,
.layout-leave-to {
  opacity: 0;
  filter: blur(1rem);
}

#app {
  min-height: 100vh;
}
</style>
