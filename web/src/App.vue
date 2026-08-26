<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'

const messages = ref([])
const connected = ref(false)
const expanded = ref(false)
let source

const latest = computed(() => messages.value[0])
const visibleHistory = computed(() => expanded.value ? messages.value : messages.value.slice(0, 8))

function addMessage(message) {
  messages.value = [message, ...messages.value.filter((item) => item.id !== message.id)].slice(0, 100)
}

function formatTime(value) {
  return new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(value))
}

function spriteClass(message) {
  return message?.kind === 'weather' ? 'sprite-weather' : message?.kind === 'message' ? 'sprite-message' : 'sprite-radio'
}

function spriteStyle(message) {
  if (!message?.symbol || message.symbol.length < 2) return {}
  const index = message.symbol.charCodeAt(1) - 33
  if (index < 0 || index > 95) return {}
  const sheet = message.symbol[0] === '\\' ? 1 : 0
  return { backgroundImage: `url('/digipi/aprs-symbols-128-${sheet}.png')`, backgroundPosition: `${-(index % 16) * 56}px ${-Math.floor(index / 16) * 56}px` }
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
      <div class="status" :class="{ offline: !connected }"><span class="status-dot"></span>{{ connected ? 'LIVE' : 'WAITING FOR TNC' }}</div>
    </header>

    <section v-if="latest" class="hero-message" aria-live="polite">
      <div class="hero-meta"><span>INCOMING MESSAGE</span><time>{{ formatTime(latest.received) }}</time></div>
      <div class="identity"><span class="aprs-symbol" :class="spriteClass(latest)" :style="spriteStyle(latest)" aria-hidden="true"></span><div><div class="callsign">{{ latest.source }}</div><div class="route">to {{ latest.destination }}<span v-if="latest.path"> via {{ latest.path }}</span></div></div></div>
      <p class="payload">{{ latest.payload }}</p>
      <div v-if="latest.weather" class="weather-readout"><span>WX</span><strong v-if="latest.weather.temperatureC">{{ latest.weather.temperatureC.toFixed(1) }}°C</strong><span v-if="latest.weather.windDirection">{{ latest.weather.windDirection }}°</span><span v-if="latest.weather.windSpeedKnots">wind {{ latest.weather.windSpeedKnots }}kt</span><span v-if="latest.weather.gustKnots">gust {{ latest.weather.gustKnots }}kt</span><span v-if="latest.weather.humidity">humidity {{ latest.weather.humidity }}%</span><span v-if="latest.weather.pressureHpa">{{ latest.weather.pressureHpa.toFixed(1) }}hPa</span></div>
      <div v-if="latest.position" class="position-readout">{{ latest.position.latitude.toFixed(5) }}, {{ latest.position.longitude.toFixed(5) }} <span>{{ latest.symbol }}</span></div>
      <div class="packet-line"><span>PACKET {{ String(latest.id).padStart(4, '0') }}</span><span>FULL TEXT</span></div>
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
          <span class="mini-symbol aprs-symbol" :class="spriteClass(message)" :style="spriteStyle(message)" aria-hidden="true"></span>
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
