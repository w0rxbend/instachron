import type { Camera } from '~/types/camera'

export function useCameras() {
  const { camerasUrl } = useCameraApi()
  const cameras = ref<Camera[]>([])
  let pollId: ReturnType<typeof setInterval> | null = null

  async function fetchCameras() {
    try {
      const ids = await $fetch<string[]>(camerasUrl.value)
      cameras.value = ids.map((id, fallbackIndex) => {
        const parsedIndex = Number.parseInt(id, 10)
        const index = Number.isNaN(parsedIndex) ? fallbackIndex : parsedIndex

        return {
          id,
          index,
          online: true,
          seed: id.length * 31 + index,
        }
      })
    } catch (err) {
      console.error('failed to fetch cameras', err)
      cameras.value = []
    }
  }

  onMounted(() => {
    fetchCameras()
    pollId = setInterval(fetchCameras, 5000)
  })

  onUnmounted(() => { if (pollId) clearInterval(pollId) })

  return { cameras }
}
