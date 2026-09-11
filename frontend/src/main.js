import './style.css'

const ENV_BASE_URL = import.meta.env.VITE_API_URL
let baseUrl = ENV_BASE_URL || localStorage.getItem('cacheDemoBaseUrl') || 'https://devops-redis-cache.onrender.com'

const EM_DASH = '—'

let lastNoCacheMs = null
let lastCachedMs = null
let ncBusy = false
let cBusy = false
let warm = false // has the cached side ever been fetched (miss or hit)?

const ICONS = {
  brand: `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="var(--brand-mark-icon)" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M13 3 5 14h6l-1 7 8-11h-6z"/></svg>`,
  db: `<svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="var(--icon-nocache)" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><ellipse cx="12" cy="5.5" rx="7.5" ry="3"/><path d="M4.5 5.5v6c0 1.66 3.36 3 7.5 3s7.5-1.34 7.5-3v-6"/><path d="M4.5 11.5v6c0 1.66 3.36 3 7.5 3s7.5-1.34 7.5-3v-6"/></svg>`,
  bolt: `<svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="var(--icon-cached)" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M13 3 5 14h6l-1 7 8-11h-6z"/></svg>`,
  pin: `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="var(--pin-stroke)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 21s7-6.3 7-11a7 7 0 1 0-14 0c0 4.7 7 11 7 11z"/><circle cx="12" cy="10" r="2.5"/></svg>`,
  sun: `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4.5"/><path d="M12 2.5v2.5M12 19v2.5M4.2 4.2l1.8 1.8M18 18l1.8 1.8M2.5 12H5M19 12h2.5M4.2 19.8 6 18M18 6l1.8-1.8"/></svg>`,
  moon: `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 14.5A8.5 8.5 0 1 1 9.5 4a6.8 6.8 0 0 0 10.5 10.5z"/></svg>`,
}

const app = document.getElementById('app')
app.innerHTML = `
  <div class="page">
    <div class="nav">
      <div class="brand">
        <div class="brand-mark">${ICONS.brand}</div>
        <span class="brand-name">cache-demo</span>
      </div>
      <div class="nav-config">
        <label for="base-url">backend</label>
        <input id="base-url" class="mono" type="text" value="${baseUrl}" spellcheck="false" />
        <button class="theme-toggle" id="theme-toggle" aria-label="Toggle light/dark theme"></button>
      </div>
    </div>

    <div class="hero">
      <h1>No-Cache vs Cached.</h1>
      <p>The same profile record served two ways &mdash; straight from the database on every request, or from Redis while the key is warm.</p>
    </div>

    <div class="split">
      <div class="card" id="panel-no-cache">
        <div class="card-head">
          <div class="card-icon">${ICONS.db}</div>
          <div>
            <div class="card-tag" style="color:var(--label-nocache)">NO CACHE</div>
            <div class="card-route mono">GET /profile/no-cache</div>
          </div>
        </div>
        <p class="card-desc">Every call queries the database. No shortcut &mdash; the full cost is paid each time.</p>
        <div class="card-number">
          <span class="val mono" id="nc-val" style="color:var(--num-nocache)">${EM_DASH}</span>
          <span class="unit mono" id="nc-unit"></span>
        </div>
        <div class="card-caption idle" id="nc-caption">flat, every single request</div>
        <div class="card-divider"></div>
        <div class="card-chips">
          <span class="chip mono" id="nc-source">source: ${EM_DASH}</span>
          <span class="chip mono" id="nc-duration">duration_ms: ${EM_DASH}</span>
        </div>
        <div class="card-error mono" id="nc-error" hidden></div>
        <button class="card-fetch" id="btn-no-cache">Fetch</button>
      </div>

      <div class="card" id="panel-cached">
        <div class="card-head">
          <div class="card-icon">${ICONS.bolt}</div>
          <div>
            <div class="card-tag" style="color:var(--label-cached)">CACHED</div>
            <div class="card-route mono">GET /profile/cached</div>
          </div>
        </div>
        <p class="card-desc">Redis answers first. The database is touched once per TTL window, on the miss that fills the key.</p>
        <div class="card-number">
          <span class="val mono" id="c-val" style="color:var(--num-cached)">${EM_DASH}</span>
          <span class="unit mono" id="c-unit"></span>
        </div>
        <div class="card-caption idle" id="c-caption">cold key &mdash; the first fetch fills it</div>
        <div class="card-divider"></div>
        <div class="card-chips">
          <span class="chip mono" id="c-source">source: ${EM_DASH}</span>
          <span class="chip mono" id="c-duration">duration_ms: ${EM_DASH}</span>
        </div>
        <div class="card-error mono" id="c-error" hidden></div>
        <button class="card-fetch" id="btn-cached">Fetch</button>
      </div>
    </div>

    <div class="midrow">
      <div id="speedup-wrap"><span class="idle-note">fetch both sides to compare.</span></div>
      <button class="btn-reset" id="btn-reset">Reset cache</button>
    </div>

    <div class="profile-section">
      <div class="profile-label">The record being fetched</div>
      <div class="profile">
        <div class="avatar mono" id="profile-avatar">···</div>
        <div class="profile-body">
          <div class="profile-name-row">
            <h2 id="profile-name">Loading&hellip;</h2>
            <span class="brand-pill" id="profile-brand"></span>
          </div>
          <div class="profile-meta">
            <span class="role" id="profile-role"></span>
            <span class="loc">${ICONS.pin}<span id="profile-location"></span></span>
          </div>
          <div class="profile-stack" id="profile-stack"></div>
          <p class="profile-bio" id="profile-bio">Fetching the record from the database&hellip;</p>
        </div>
      </div>
    </div>
  </div>
`

// ---------- Theme toggle ----------

const themeToggle = document.getElementById('theme-toggle')
function currentTheme() {
  return document.documentElement.getAttribute('data-theme') === 'dark' ? 'dark' : 'light'
}
function renderThemeIcon() {
  // Shows the icon for the theme you'd switch TO.
  themeToggle.innerHTML = currentTheme() === 'dark' ? ICONS.sun : ICONS.moon
}
themeToggle.addEventListener('click', () => {
  const next = currentTheme() === 'dark' ? 'light' : 'dark'
  document.documentElement.setAttribute('data-theme', next)
  localStorage.setItem('cacheDemoTheme', next)
  renderThemeIcon()
})
renderThemeIcon()

// ---------- Backend URL ----------

const baseUrlInput = document.getElementById('base-url')
baseUrlInput.addEventListener('change', () => {
  baseUrl = baseUrlInput.value.trim().replace(/\/$/, '')
  localStorage.setItem('cacheDemoBaseUrl', baseUrl)
})

// ---------- Fetch wiring ----------

document.getElementById('btn-no-cache').addEventListener('click', () => runFetch('no-cache'))
document.getElementById('btn-cached').addEventListener('click', () => runFetch('cached'))
document.getElementById('btn-reset').addEventListener('click', resetCache)

// Populate the profile (and the no-cache card) from the real backend as
// soon as the page loads, instead of ever showing hardcoded placeholder data.
runFetch('no-cache')

async function runFetch(mode) {
  const p = mode === 'no-cache' ? 'nc' : 'c'
  if (p === 'nc' && ncBusy) return
  if (p === 'c' && cBusy) return

  const button = document.getElementById(mode === 'no-cache' ? 'btn-no-cache' : 'btn-cached')
  const errorEl = document.getElementById(`${p}-error`)
  if (p === 'nc') ncBusy = true
  else cBusy = true
  button.disabled = true
  button.textContent = 'Fetching…'
  document.getElementById(`${p}-val`).textContent = '···'
  errorEl.hidden = true

  try {
    const res = await fetch(`${baseUrl}/profile/${mode}`)
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const data = await res.json()

    document.getElementById(`${p}-val`).textContent = data.duration_ms.toFixed(0)
    document.getElementById(`${p}-unit`).textContent = 'ms'
    document.getElementById(`${p}-source`).textContent = `source: ${data.source}`
    document.getElementById(`${p}-duration`).textContent = `duration_ms: ${data.duration_ms.toFixed(2)}`

    if (mode === 'no-cache') {
      lastNoCacheMs = data.duration_ms
    } else {
      warm = true
      lastCachedMs = data.source === 'cache' ? data.duration_ms : null
      const captionEl = document.getElementById('c-caption')
      captionEl.classList.remove('idle')
      captionEl.classList.add('active')
      captionEl.textContent = data.source === 'cache' ? 'served from cache' : 'miss — key written, 30s ttl'
    }
    updateSpeedup()
    renderProfile(data.profile)
  } catch (err) {
    document.getElementById(`${p}-val`).textContent = EM_DASH
    errorEl.textContent = `connection refused — is the Go server running? (${err.message})`
    errorEl.hidden = false
  } finally {
    if (p === 'nc') ncBusy = false
    else cBusy = false
    button.disabled = false
    button.textContent = 'Fetch'
  }
}

async function resetCache() {
  try {
    await fetch(`${baseUrl}/reset`)
  } catch (err) {
    // Best-effort — still reset the UI's own idea of cache state.
  }
  warm = false
  lastCachedMs = null
  document.getElementById('c-val').textContent = EM_DASH
  document.getElementById('c-unit').textContent = ''
  document.getElementById('c-source').textContent = `source: ${EM_DASH}`
  document.getElementById('c-duration').textContent = `duration_ms: ${EM_DASH}`
  const captionEl = document.getElementById('c-caption')
  captionEl.classList.remove('active')
  captionEl.classList.add('idle')
  captionEl.textContent = 'cache cleared — next fetch is a miss'
  updateSpeedup()
}

function updateSpeedup() {
  const wrap = document.getElementById('speedup-wrap')
  if (lastNoCacheMs != null && lastCachedMs != null) {
    const factor = Math.round(lastNoCacheMs / lastCachedMs)
    wrap.innerHTML = `<span class="speedup"><span class="val mono">${factor}&times;</span><span class="label">faster with a warm cache</span></span>`
  } else {
    wrap.innerHTML = `<span class="idle-note">fetch both sides to compare.</span>`
  }
}

function renderProfile(p) {
  document.getElementById('profile-avatar').textContent = p.name
    .split(' ')
    .map((w) => w[0])
    .slice(0, 2)
    .join('')
    .toUpperCase()
  document.getElementById('profile-name').textContent = p.name
  document.getElementById('profile-brand').textContent = p.brand
  document.getElementById('profile-role').textContent = p.role
  document.getElementById('profile-location').textContent = p.location
  document.getElementById('profile-stack').innerHTML = p.stack.map((s) => `<span class="chip">${s}</span>`).join('')
  document.getElementById('profile-bio').textContent = p.bio
}
