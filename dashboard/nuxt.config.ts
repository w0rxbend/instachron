const cameraApiBase = (process.env.API_BASE ?? 'http://localhost:8080').replace(/\/$/, '')

export default defineNuxtConfig({
  devtools: { enabled: false },
  css: ['~/assets/css/main.css'],
  runtimeConfig: {
    public: {
      cameraApiBase: process.env.NUXT_PUBLIC_CAMERA_API_BASE ?? '',
    },
  },
  routeRules: {
    '/cameras': { proxy: `${cameraApiBase}/cameras` },
    '/cameras/**': { proxy: `${cameraApiBase}/cameras/**` },
  },
  app: {
    head: {
      title: 'SENTINEL // ESP32 Camera Mesh',
      link: [
        { rel: 'preconnect', href: 'https://fonts.googleapis.com' },
        { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' },
        {
          rel: 'stylesheet',
          href: 'https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@300;400;500;600;700&family=IBM+Plex+Sans:wght@300;400;500;600;700&display=swap',
        },
      ],
    },
  },
})
