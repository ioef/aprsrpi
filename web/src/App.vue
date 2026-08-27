<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'

const messages = ref([])
const connected = ref(false)
const expanded = ref(false)
const weatherReportOpen = ref(false)
let source

const latest = computed(() => messages.value[0])
const visibleHistory = computed(() => expanded.value ? messages.value : messages.value.slice(0, 8))
const latestWeather = computed(() => messages.value.find((message) => message.weather))

function addMessage(message) {
  messages.value = [message, ...messages.value.filter((item) => item.id !== message.id)].slice(0, 100)
}

function formatTime(value) {
  return new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(value))
}

function formatCoordinate(value, latitude) {
  const direction = latitude ? (value < 0 ? 'S' : 'N') : (value < 0 ? 'W' : 'E')
  const absolute = Math.abs(value)
  const degrees = Math.floor(absolute)
  const minutes = (absolute - degrees) * 60
  return `${degrees}°${minutes.toFixed(2)}' ${direction}`
}

function formatLocation(position) {
  return `${formatCoordinate(position.latitude, true)} ${formatCoordinate(position.longitude, false)}`
}

function mapURL(source) {
  return `https://aprs.fi/#!call=a%2F${encodeURIComponent(source)}`
}

function locatorURL(locator) {
  return `https://aprs.fi/#!addr=${encodeURIComponent(locator)}`
}

function spriteClass(message) {
  return message?.kind === 'weather' ? 'sprite-weather' : message?.kind === 'message' ? 'sprite-message' : 'sprite-radio'
}

function spriteStyle(message) {
  if (!message?.symbol || message.symbol.length < 2) return {}
  const index = message.symbol.charCodeAt(1) - 33
  if (index < 0 || index > 95) return {}
  // APRS table: '/' is primary, '\\' is alternate, and alphanumeric tables are overlays.
  const sheet = message.symbol[0] === '\\' ? 1 : 0
  const column = index % 16
  const row = Math.floor(index / 16)
  return { '--sprite-column': column, '--sprite-row': row, backgroundImage: `url('/digipi/aprs-symbols-128-${sheet}.png')` }
}

function spriteOverlay(message) {
  if (!message?.symbol || message.symbol.length < 2) return undefined
  const table = message.symbol[0]
  return /^[0-9A-Za-z]$/.test(table) ? table.toUpperCase() : undefined
}

function connect() {
  source = new EventSource('/api/events')
  source.onopen = () => { connected.value = true }
  source.onerror = () => { connected.value = false }
  source.addEventListener('aprs', (event) => addMessage(JSON.parse(event.data)))
}

onMounted(async () => {
  try {
    const response = await fetch('/api/messages')
    messages.value = await response.json()
  } catch { connected.value = false }
  connect()
})

onUnmounted(() => source?.close())
</script>

<template>
  <main class="kiosk-shell">
    <header class="topbar">
      <div class="brand"><span class="brand-mark">//</span><span>APRS WATCH</span></div>
      <div class="topbar-actions">
        <button v-if="latestWeather" class="weather-button" type="button" @click="weatherReportOpen = true">Latest WX</button>
        <div class="status" :class="{ offline: !connected }"><span class="status-dot"></span>{{ connected ? 'LIVE' : 'WAITING FOR TNC' }}</div>
      </div>
    </header>

    <div v-if="weatherReportOpen && latestWeather" class="weather-dialog-backdrop" @click.self="weatherReportOpen = false">
      <section class="weather-dialog" role="dialog" aria-modal="true" aria-labelledby="latest-weather-title">
        <div class="weather-dialog-header">
          <div><small>LAST RECEIVED WEATHER REPORT</small><h2 id="latest-weather-title">{{ latestWeather.source }}</h2><time>{{ formatTime(latestWeather.received) }}</time></div>
          <button class="dialog-close" type="button" aria-label="Close latest weather report" @click="weatherReportOpen = false">&#215;</button>
        </div>
        <div class="weather-grid weather-dialog-grid">
          <div v-if="latestWeather.weather.temperatureC != null" class="weather-metric"><img src="/icons/thermometer.svg" alt=""><div><small>TEMP</small><strong>{{ latestWeather.weather.temperatureC.toFixed(1) }}°C</strong></div></div>
          <div v-if="latestWeather.weather.pressureHpa != null" class="weather-metric"><img src="/icons/pressure.svg" alt=""><div><small>PRESSURE</small><strong>{{ latestWeather.weather.pressureHpa.toFixed(1) }} hPa</strong></div></div>
          <div v-if="latestWeather.weather.humidity != null" class="weather-metric"><img src="/icons/humidity.svg" alt=""><div><small>HUMIDITY</small><strong>{{ latestWeather.weather.humidity }}%</strong></div></div>
          <div v-if="latestWeather.weather.windSpeedKnots != null" class="weather-metric"><img src="/icons/wind.svg" alt=""><div><small>WIND</small><strong>{{ latestWeather.weather.windSpeedKnots }} kt<span v-if="latestWeather.weather.windDirection != null"> @ {{ latestWeather.weather.windDirection }}°</span></strong></div></div>
          <div v-if="latestWeather.weather.gustKnots != null" class="weather-metric"><img src="/icons/wind.svg" alt=""><div><small>GUST</small><strong>{{ latestWeather.weather.gustKnots }} kt</strong></div></div>
          <div v-if="latestWeather.weather.rainLastHourMm != null" class="weather-metric"><img src="/icons/rain.svg" alt=""><div><small>RAIN / 1H</small><strong>{{ latestWeather.weather.rainLastHourMm.toFixed(1) }} mm</strong></div></div>
          <div v-if="latestWeather.weather.rain24HoursMm != null" class="weather-metric"><img src="/icons/rain.svg" alt=""><div><small>RAIN / 24H</small><strong>{{ latestWeather.weather.rain24HoursMm.toFixed(1) }} mm</strong></div></div>
        </div>
      </section>
    </div>

    <section v-if="latest" class="hero-message" aria-live="polite">
      <div class="hero-meta"><span>INCOMING MESSAGE</span><time>{{ formatTime(latest.received) }}</time></div>
      <div class="hero-grid">
        <div class="message-reading">
          <div class="identity"><span class="aprs-symbol" :class="spriteClass(latest)" :style="spriteStyle(latest)" :data-overlay="spriteOverlay(latest)" aria-hidden="true"></span><div><div class="callsign">{{ latest.source }}</div><div class="route">to {{ latest.destination }}<span v-if="latest.path"> via {{ latest.path }}</span></div></div></div>
          <div v-if="latest.kind !== 'weather'" class="payload-block"><small class="raw-label">Raw packet text</small><p class="payload">{{ latest.payload }}</p></div>
          <p v-else class="weather-summary">Structured weather report from {{ latest.source }}</p>
          <div v-if="latest.position" class="position-readout"><strong>Location:</strong> {{ formatLocation(latest.position) }} <span>{{ latest.symbol }}</span><div class="location-links">locator <a :href="locatorURL(latest.position.locator)" target="_blank" rel="noreferrer">{{ latest.position.locator }}</a> <span aria-hidden="true">-</span> <a :href="mapURL(latest.source)" target="_blank" rel="noreferrer">show map</a></div><div v-if="latest.position.micEStatus" class="position-comment">Mic-E message: {{ latest.position.micEStatus }}</div><div v-if="latest.position.comment" class="position-comment">Comment: {{ latest.position.comment }}</div><div v-if="latest.position.phg" class="position-comment">PHG: {{ latest.position.phg }}</div><div v-if="latest.position.url" class="position-comment"><a :href="latest.position.url" target="_blank" rel="noreferrer">{{ latest.position.url }}</a></div></div>
          <div class="packet-line"><span>PACKET {{ String(latest.id).padStart(4, '0') }}</span><span>{{ latest.type.toUpperCase() }}</span></div>
        </div>
        <aside class="weather-panel" v-if="latest.weather">
          <div class="weather-title"><span>WX REPORT</span><span>APRS</span></div>
          <div class="weather-grid">
            <div v-if="latest.weather.temperatureC != null" class="weather-metric"><img src="/icons/thermometer.svg" alt=""><div><small>TEMP</small><strong>{{ latest.weather.temperatureC.toFixed(1) }}°C</strong></div></div>
            <div v-if="latest.weather.pressureHpa != null" class="weather-metric"><img src="/icons/pressure.svg" alt=""><div><small>PRESSURE</small><strong>{{ latest.weather.pressureHpa.toFixed(1) }} hPa</strong></div></div>
            <div v-if="latest.weather.humidity != null" class="weather-metric"><img src="/icons/humidity.svg" alt=""><div><small>HUMIDITY</small><strong>{{ latest.weather.humidity }}%</strong></div></div>
            <div v-if="latest.weather.windSpeedKnots != null" class="weather-metric"><img src="/icons/wind.svg" alt=""><div><small>WIND</small><strong>{{ latest.weather.windSpeedKnots }} kt<span v-if="latest.weather.windDirection != null"> @ {{ latest.weather.windDirection }}°</span></strong></div></div>
            <div v-if="latest.weather.gustKnots != null" class="weather-metric"><img src="/icons/wind.svg" alt=""><div><small>GUST</small><strong>{{ latest.weather.gustKnots }} kt</strong></div></div>
            <div v-if="latest.weather.rainLastHourMm != null" class="weather-metric"><img src="/icons/rain.svg" alt=""><div><small>RAIN / 1H</small><strong>{{ latest.weather.rainLastHourMm.toFixed(1) }} mm</strong></div></div>
            <div v-if="latest.weather.rain24HoursMm != null" class="weather-metric"><img src="/icons/rain.svg" alt=""><div><small>RAIN / 24H</small><strong>{{ latest.weather.rain24HoursMm.toFixed(1) }} mm</strong></div></div>
          </div>
        </aside>
        <aside v-else class="packet-panel">
          <small>PACKET TYPE</small><strong>{{ latest.type.toUpperCase() }}</strong>
          <small>DESTINATION</small><strong>{{ latest.destination }}</strong>
          <small>PATH</small><strong>{{ latest.path || 'DIRECT' }}</strong>
        </aside>
      </div>
    </section>

    <section v-else class="empty-state">
      <div class="radar"><span></span></div>
      <p>SCANNING APRS FREQUENCY</p>
      <small>Waiting for the next packet from the KISS interface</small>
    </section>

    <section class="history-section">
      <div class="section-heading"><h2>Recent traffic</h2><button @click="expanded = !expanded">{{ expanded ? 'Collapse' : 'Show all' }}</button></div>
      <div v-if="visibleHistory.length" class="traffic-list">
        <article v-for="message in visibleHistory" :key="message.id" class="traffic-row" :class="{ selected: latest?.id === message.id }">
          <span class="mini-symbol aprs-symbol" :class="spriteClass(message)" :style="spriteStyle(message)" :data-overlay="spriteOverlay(message)" aria-hidden="true"></span>
          <time>{{ formatTime(message.received) }}</time>
          <strong>{{ message.source }}</strong>
          <span class="arrow">→</span>
          <span>{{ message.destination }}</span>
          <p>{{ message.payload }}</p>
        </article>
      </div>
      <div v-else class="no-traffic">No packets received yet</div>
    </section>
  </main>
</template>
