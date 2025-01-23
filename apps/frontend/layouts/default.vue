<template>
  <div>
    <div class="bg-base-300 relative">
      <HeaderNavBar />
    </div>
    <div class="md:container md:mx-auto prose">
      <slot />
    </div>
  </div>
</template>

<script lang="ts" setup>
await useProfile().init()
</script>
