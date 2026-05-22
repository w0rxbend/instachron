export function useClock() {
  const now = ref(new Date())
  let id: ReturnType<typeof setInterval> | null = null

  onMounted(() => {
    now.value = new Date()
    id = setInterval(() => { now.value = new Date() }, 1000)
  })

  onUnmounted(() => { if (id) clearInterval(id) })

  return now
}
