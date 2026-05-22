type CameraEndpoint = 'snapshot' | 'stream'

export function useCameraApi() {
  const config = useRuntimeConfig()
  const base = computed(() =>
    String(config.public.cameraApiBase || '').replace(/\/$/, '')
  )

  const camerasUrl = computed(() => `${base.value}/cameras`)

  function cameraUrl(id: string, endpoint: CameraEndpoint) {
    return `${base.value}/cameras/${encodeURIComponent(id)}/${endpoint}`
  }

  return {
    camerasUrl,
    snapshotUrl: (id: string) => cameraUrl(id, 'snapshot'),
    streamUrl: (id: string) => cameraUrl(id, 'stream'),
  }
}
