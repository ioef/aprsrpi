<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'

const messages = ref([])
const stationPosition = ref(null)
const connected = ref(false)
const expanded = ref(false)
const weatherReportOpen = ref(false)
const mapDialogOpen = ref(false)
const mapDialogURL = ref('')
const morseEnabled = ref(false)
const morsePlaying = ref(false)
const morseError = ref('')
let source
let morseContext
let morseOscillator

const morseCode = {
  A: '.-', B: '-...', C: '-.-.', D: '-..', E: '.', F: '..-.', G: '--.', H: '....',
  I: '..', J: '.---', K: '-.-', L: '.-..', M: '--', N: '-.', O: '---', P: '.--.',
  Q: '--.-', R: '.-.', S: '...', T: '-', U: '..-', V: '...-', W: '.--', X: '-..-',
  Y: '-.--', Z: '--..', 0: '-----', 1: '.----', 2: '..---', 3: '...--', 4: '....-',
  5: '.....', 6: '-....', 7: '--...', 8: '---..', 9: '----.', '-': '-....-', '/': '-..-.',
}

const latest = computed(() => messages.value[0])
const visibleHistory = computed(() => expanded.value ? messages.value : messages.value.slice(0, 8))
const latestWeather = computed(() => messages.value.find((message) => message.weather))
const latestDistance = computed(() => {
  const position = latest.value?.position
  const station = stationPosition.value
  if (!position || !station
    || !Number.isFinite(position.latitude) || !Number.isFinite(position.longitude)
    || Math.abs(position.latitude) > 90 || Math.abs(position.longitude) > 180) return ''

  const radians = (degrees) => degrees * Math.PI / 180
  const latitude1 = radians(station.latitude)
  const latitude2 = radians(position.latitude)
  const latitudeDelta = latitude2 - latitude1
  const longitudeDelta = radians(position.longitude - station.longitude)
  const sinLatitude = Math.sin(latitudeDelta / 2)
  const sinLongitude = Math.sin(longitudeDelta / 2)
  const haversine = Math.min(1, sinLatitude ** 2 + Math.cos(latitude1) * Math.cos(latitude2) * sinLongitude ** 2)
  const distanceKm = 6371.0088 * 2 * Math.atan2(Math.sqrt(haversine), Math.sqrt(1 - haversine))
  const bearingY = Math.sin(longitudeDelta) * Math.cos(latitude2)
  const bearingX = Math.cos(latitude1) * Math.sin(latitude2) - Math.sin(latitude1) * Math.cos(latitude2) * Math.cos(longitudeDelta)
  const bearing = (Math.atan2(bearingY, bearingX) * 180 / Math.PI + 360) % 360
  const directions = ['N', 'NE', 'E', 'SE', 'S', 'SW', 'W', 'NW']
  const direction = directions[Math.round(bearing / 45) % directions.length]
  const distance = distanceKm < 1
    ? `${distanceKm.toLocaleString(undefined, { minimumFractionDigits: 1, maximumFractionDigits: 1 })} km`
    : `${Math.round(distanceKm)} km`

  return `${distance} ${direction}`
})
const packetComments = computed(() => {
  const message = latest.value
  if (!message) return []

  const comments = []
  const add = (label, value, href) => {
    if (value !== undefined && value !== null && value !== '') comments.push({ label, value, href })
  }

  add('Message', message.aprsMessage?.text)
  add('Status', message.statusText)
  add('Mic-E', message.position?.micEStatus)
  add('Comment', message.position?.comment)
  add('PHG', message.position?.phg)
  add('URL', message.position?.url, message.position?.url)

  return comments
})
const structuredDetails = computed(() => {
  const message = latest.value
  if (!message) return []

  const details = []
  const add = (label, value) => {
    if (value !== undefined && value !== null && value !== '') details.push({ label, value })
  }

  if (message.aprsMessage) {
    add('Addressee', message.aprsMessage.addressee)
    add('Message number', message.aprsMessage.number)
    add('Reply acknowledgement', message.aprsMessage.replyAck)
  }
  if (message.object) {
    add('Object', `${message.object.name} · ${message.object.alive ? 'active' : 'killed'}`)
    add('Object timestamp', message.object.timestamp)
  }
  add('Query', message.query)
  add('Grid locator', message.gridLocator)

  const position = message.position
  if (position) {
    add('Position timestamp', position.timestamp)
    add('Position ambiguity', position.ambiguity ? `${position.ambiguity} digit${position.ambiguity === 1 ? '' : 's'}` : '')
    add('Altitude', position.altitudeFt != null ? `${position.altitudeFt} ft` : '')
    add('Course', position.course != null ? `${position.course}°` : '')
    add('Speed', position.speedKnots != null ? `${position.speedKnots} kt` : '')
    add('DF bearing', position.dfBearing != null ? `${position.dfBearing}°` : '')
    add('DF number', position.dfNumber)
    add('DF range', position.dfRange)
    add('DF quality', position.dfQuality)
  }

  const telemetry = message.telemetry
  if (telemetry) {
    add('Telemetry sequence', telemetry.sequenceLabel || telemetry.sequence)
    add('Analog values', telemetry.analogValues?.length ? telemetry.analogValues.join(', ') : telemetry.analog?.join(', '))
    add('Digital bits', telemetry.digital || telemetry.bits)
    add('Parameters', telemetry.parameters?.join(' · '))
    add('Units', telemetry.units?.join(' · '))
    add('Calibration coefficients', telemetry.coefficients?.map((group) => group.join(', ')).join(' · '))
    if (telemetry.base91) add('Encoding', 'Base91 telemetry')
  }

  if (message.weather) {
    add('Weather software', message.weather.software)
    add('Weather station unit', message.weather.unit)
  }
  if (message.thirdParty) {
    add('Encapsulated packet', `${message.thirdParty.source} · ${message.thirdParty.type}`)
  }

  return details
})

function stopMorsePlayback() {
  if (morseOscillator) {
    const oscillator = morseOscillator
    morseOscillator = undefined
    oscillator.onended = null
    oscillator.stop()
  }
  morsePlaying.value = false
}

async function toggleMorse() {
  if (morseEnabled.value) {
    morseEnabled.value = false
    stopMorsePlayback()
    return
  }

  morseError.value = ''
  const AudioContextClass = window.AudioContext || window.webkitAudioContext
  if (!AudioContextClass) {
    morseError.value = 'Morse audio is not supported by this browser.'
    return
  }

  try {
    morseContext ||= new AudioContextClass()
    await morseContext.resume()
    morseEnabled.value = true
  } catch (error) {
    morseError.value = 'Unable to play Morse audio.'
    console.error('Morse playback failed:', error)
  }
}

function playMorse(callsign) {
  const characters = callsign.toUpperCase().split('')
    .map((character) => morseCode[character])
    .filter(Boolean)
  if (!characters.length) {
    morseError.value = 'This callsign has no supported Morse characters.'
    return
  }

  try {
    if (!morseContext || morseContext.state !== 'running') return
    const context = morseContext
    const dotSeconds = 0.08
    const startAt = context.currentTime + 0.05
    let elapsed = 0
    const oscillator = context.createOscillator()
    const gain = context.createGain()
    oscillator.frequency.value = 600
    gain.gain.setValueAtTime(0, startAt)
    oscillator.connect(gain)
    gain.connect(context.destination)

    characters.forEach((character, characterIndex) => {
      for (const element of character) {
        const duration = element === '.' ? dotSeconds : dotSeconds * 3
        const toneStart = startAt + elapsed
        gain.gain.setValueAtTime(0, toneStart)
        gain.gain.linearRampToValueAtTime(0.08, toneStart + 0.005)
        gain.gain.setValueAtTime(0.08, toneStart + duration - 0.005)
        gain.gain.linearRampToValueAtTime(0, toneStart + duration)
        elapsed += duration + dotSeconds
      }
      elapsed += dotSeconds * (characterIndex === characters.length - 1 ? -1 : 2)
    })

    morseOscillator = oscillator
    oscillator.onended = () => {
      if (morseOscillator !== oscillator) return
      morseOscillator = undefined
      morsePlaying.value = false
    }
    oscillator.start(startAt)
    oscillator.stop(startAt + elapsed + 0.02)
    morsePlaying.value = true
  } catch (error) {
    stopMorsePlayback()
    morseError.value = 'Unable to play Morse audio.'
    console.error('Morse playback failed:', error)
  }
}

watch(() => latest.value?.source, (callsign) => {
  stopMorsePlayback()
  morseError.value = ''
  if (morseEnabled.value && callsign) playMorse(callsign)
})

function addMessage(message) {
  if (!message || typeof message !== 'object' || typeof message.id !== 'number') return
  message.source = typeof message.source === 'string' ? message.source : 'UNKNOWN'
  message.destination = typeof message.destination === 'string' ? message.destination : 'UNKNOWN'
  message.path = typeof message.path === 'string' ? message.path : ''
  message.payload = typeof message.payload === 'string' ? message.payload : ''
  message.kind = typeof message.kind === 'string' ? message.kind : 'packet'
  message.type = typeof message.type === 'string' ? message.type : 'packet'
  message.received = typeof message.received === 'string' && !Number.isNaN(Date.parse(message.received)) ? message.received : new Date().toISOString()
  messages.value = [message, ...messages.value.filter((item) => item.id !== message.id)].slice(0, 100)
}

function loadMessages(payload) {
  if (!Array.isArray(payload)) return
  messages.value = []
  payload.forEach(addMessage)
}

function formatTime(value) {
  return new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(value))
}

function mapURL(source) {
  return `https://aprs.fi/#!call=a%2F${encodeURIComponent(source)}`
}

function openMap(source) {
  mapDialogURL.value = mapURL(source)
  mapDialogOpen.value = true
}

function locatorURL(locator) {
  return `https://aprs.fi/#!addr=${encodeURIComponent(locator)}`
}

function openLocator(locator) {
  mapDialogURL.value = locatorURL(locator)
  mapDialogOpen.value = true
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
  source.addEventListener('aprs', (event) => {
    try {
      addMessage(JSON.parse(event.data))
    } catch { connected.value = false }
  })
}

onMounted(async () => {
  try {
    const response = await fetch('/api/station')
    if (!response.ok) throw new Error(`station request failed: ${response.status}`)
    const station = await response.json()
    if (station.configured && Number.isFinite(station.latitude) && Number.isFinite(station.longitude)
      && Math.abs(station.latitude) <= 90 && Math.abs(station.longitude) <= 180) {
      stationPosition.value = { latitude: station.latitude, longitude: station.longitude }
    }
  } catch (error) {
    console.error('Unable to load configured station position:', error)
  }

  try {
    const response = await fetch('/api/messages')
    if (!response.ok) throw new Error(`message request failed: ${response.status}`)
    loadMessages(await response.json())
  } catch { connected.value = false }
  connect()
})

onUnmounted(() => {
  source?.close()
  morseEnabled.value = false
  stopMorsePlayback()
  if (morseContext && morseContext.state !== 'closed') void morseContext.close()
})
</script>

<template>
  <main class="kiosk-shell">
    <header class="topbar">
      <div class="brand"><span class="brand-mark">//</span><span>APRS WATCH</span></div>
      <div class="topbar-actions">
        <button v-if="latestWeather" class="weather-button" type="button" @click="weatherReportOpen = true">Latest WX</button>
        <button class="morse-button" type="button" :aria-pressed="morseEnabled" :aria-label="morseEnabled ? 'Disable automatic Morse playback' : 'Enable automatic Morse playback'" @click="toggleMorse">{{ morseEnabled ? (morsePlaying ? 'Morse ON · PLAYING' : 'Morse ON') : 'Morse OFF' }}</button>
        <p v-if="morseError" class="morse-error" role="status">{{ morseError }}</p>
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
          <div v-if="latestWeather.weather.windSpeedMph != null || latestWeather.weather.windSpeedKnots != null" class="weather-metric"><img src="/icons/wind.svg" alt=""><div><small>WIND</small><strong>{{ latestWeather.weather.windSpeedMph ?? latestWeather.weather.windSpeedKnots }} {{ latestWeather.weather.windSpeedMph != null ? 'mph' : 'kt' }}<span v-if="latestWeather.weather.windDirection != null"> @ {{ latestWeather.weather.windDirection }}°</span></strong></div></div>
          <div v-if="latestWeather.weather.gustMph != null || latestWeather.weather.gustKnots != null" class="weather-metric"><img src="/icons/wind.svg" alt=""><div><small>GUST</small><strong>{{ latestWeather.weather.gustMph ?? latestWeather.weather.gustKnots }} {{ latestWeather.weather.gustMph != null ? 'mph' : 'kt' }}</strong></div></div>
          <div v-if="latestWeather.weather.rainLastHourMm != null" class="weather-metric"><img src="/icons/rain.svg" alt=""><div><small>RAIN / 1H</small><strong>{{ latestWeather.weather.rainLastHourMm.toFixed(1) }} mm</strong></div></div>
          <div v-if="latestWeather.weather.rain24HoursMm != null" class="weather-metric"><img src="/icons/rain.svg" alt=""><div><small>RAIN / 24H</small><strong>{{ latestWeather.weather.rain24HoursMm.toFixed(1) }} mm</strong></div></div>
          <div v-if="latestWeather.weather.rainSinceMidnightMm != null" class="weather-metric"><img src="/icons/rain.svg" alt=""><div><small>RAIN / TODAY</small><strong>{{ latestWeather.weather.rainSinceMidnightMm.toFixed(1) }} mm</strong></div></div>
          <div v-if="latestWeather.weather.luminosity != null" class="weather-metric"><img src="/icons/weather.svg" alt=""><div><small>LUMINOSITY</small><strong>{{ latestWeather.weather.luminosity }} W/m²</strong></div></div>
        </div>
      </section>
    </div>

    <div v-if="mapDialogOpen" class="map-dialog-backdrop" @click.self="mapDialogOpen = false">
      <section class="map-dialog" role="dialog" aria-modal="true" aria-labelledby="map-dialog-title">
        <div class="map-dialog-header"><h2 id="map-dialog-title">APRS.FI MAP</h2><button class="dialog-close" type="button" aria-label="Close map" @click="mapDialogOpen = false">&#215;</button></div>
        <iframe :src="mapDialogURL" title="APRS.fi map" class="map-frame"></iframe>
      </section>
    </div>

    <section v-if="latest" class="hero-message" aria-live="polite">
      <div class="hero-meta"><span>INCOMING MESSAGE</span><time>{{ formatTime(latest.received) }}</time></div>
      <div class="hero-grid">
        <div class="message-reading">
          <div class="identity"><span class="aprs-symbol" :class="spriteClass(latest)" :style="spriteStyle(latest)" :data-overlay="spriteOverlay(latest)" aria-hidden="true"></span><div><div class="callsign">{{ latest.source }}</div><div class="route">to {{ latest.destination }}<span v-if="latest.path"> via {{ latest.path }}</span></div></div></div>
          <p v-if="latest.kind === 'weather'" class="weather-summary">Structured weather report from {{ latest.source }}</p>
          <div v-if="latest.position" class="position-readout"><strong v-if="latestDistance">Distance:</strong> {{ latestDistance }}<div class="location-links">locator <button class="map-button" type="button" @click="openLocator(latest.position.locator)">{{ latest.position.locator }}</button> <span aria-hidden="true">-</span> <button class="map-button" type="button" @click="openMap(latest.source)">show map</button></div></div>
          <div v-if="packetComments.length" class="packet-comments" aria-label="Packet comments">
            <p v-for="comment in packetComments" :key="comment.label" class="packet-comment">
              <span class="packet-comment-label">{{ comment.label }}:</span>
              <a v-if="comment.href" :href="comment.href" target="_blank" rel="noreferrer">{{ comment.value }}</a>
              <span v-else>{{ comment.value }}</span>
            </p>
          </div>
          <dl v-if="structuredDetails.length" class="packet-details" aria-label="Decoded packet details">
            <div v-for="detail in structuredDetails" :key="detail.label" class="packet-detail">
              <dt>{{ detail.label }}</dt>
              <dd>{{ detail.value }}</dd>
            </div>
          </dl>
          <details class="payload-details">
            <summary>Show raw APRS payload</summary>
            <p class="payload">{{ latest.payload }}</p>
          </details>
          <div class="packet-line"><span>PACKET {{ String(latest.id).padStart(4, '0') }}</span><span>{{ latest.type.toUpperCase() }}</span></div>
        </div>
        <aside class="weather-panel" v-if="latest.weather">
          <div class="weather-title"><span>WX REPORT</span><span>APRS</span></div>
          <div class="weather-grid">
            <div v-if="latest.weather.temperatureC != null" class="weather-metric"><img src="/icons/thermometer.svg" alt=""><div><small>TEMP</small><strong>{{ latest.weather.temperatureC.toFixed(1) }}°C</strong></div></div>
            <div v-if="latest.weather.pressureHpa != null" class="weather-metric"><img src="/icons/pressure.svg" alt=""><div><small>PRESSURE</small><strong>{{ latest.weather.pressureHpa.toFixed(1) }} hPa</strong></div></div>
            <div v-if="latest.weather.humidity != null" class="weather-metric"><img src="/icons/humidity.svg" alt=""><div><small>HUMIDITY</small><strong>{{ latest.weather.humidity }}%</strong></div></div>
            <div v-if="latest.weather.windSpeedMph != null || latest.weather.windSpeedKnots != null" class="weather-metric"><img src="/icons/wind.svg" alt=""><div><small>WIND</small><strong>{{ latest.weather.windSpeedMph ?? latest.weather.windSpeedKnots }} {{ latest.weather.windSpeedMph != null ? 'mph' : 'kt' }}<span v-if="latest.weather.windDirection != null"> @ {{ latest.weather.windDirection }}°</span></strong></div></div>
            <div v-if="latest.weather.gustMph != null || latest.weather.gustKnots != null" class="weather-metric"><img src="/icons/wind.svg" alt=""><div><small>GUST</small><strong>{{ latest.weather.gustMph ?? latest.weather.gustKnots }} {{ latest.weather.gustMph != null ? 'mph' : 'kt' }}</strong></div></div>
            <div v-if="latest.weather.rainLastHourMm != null" class="weather-metric"><img src="/icons/rain.svg" alt=""><div><small>RAIN / 1H</small><strong>{{ latest.weather.rainLastHourMm.toFixed(1) }} mm</strong></div></div>
            <div v-if="latest.weather.rain24HoursMm != null" class="weather-metric"><img src="/icons/rain.svg" alt=""><div><small>RAIN / 24H</small><strong>{{ latest.weather.rain24HoursMm.toFixed(1) }} mm</strong></div></div>
            <div v-if="latest.weather.rainSinceMidnightMm != null" class="weather-metric"><img src="/icons/rain.svg" alt=""><div><small>RAIN / TODAY</small><strong>{{ latest.weather.rainSinceMidnightMm.toFixed(1) }} mm</strong></div></div>
            <div v-if="latest.weather.luminosity != null" class="weather-metric"><img src="/icons/weather.svg" alt=""><div><small>LUMINOSITY</small><strong>{{ latest.weather.luminosity }} W/m²</strong></div></div>
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
          <p :title="message.payload">{{ message.payload }}</p>
        </article>
      </div>
      <div v-else class="no-traffic">No packets received yet</div>
    </section>
  </main>
</template>
