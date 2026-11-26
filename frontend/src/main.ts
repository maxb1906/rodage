import './style.css';
import { t, setLang, lang, Lang } from './i18n';
import { LiveChart } from './chart';
import * as App from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

// ---------- État ----------

type View = 'test' | 'reports' | 'compare' | 'server' | 'machine';
type Stage = 'setup' | 'running' | 'result';

interface Finding { sev: string; comp: string; text: string }
interface ReportView {
  id: string; created: string; mode: string; mode_text: string; verdict: string; verdict_text: string;
  verdict_long: string; findings: Finding[]; limits: string[]; elapsed: string; host: string; path: string;
}
interface DiskView {
  name: string; path: string; model: string; serial: string; size_text: string; bus: string;
  partitions: number; system: boolean; mounted: boolean; raw_eligible: boolean;
}
interface Machine {
  lang: string; version: string; config: { k: string; v: string }[]; sensor_level: number; sensor_text: string;
  sensor_details: string[]; temp_c: number; freq_mhz: number; kernels: string[]; disks: DiskView[];
  has_battery: boolean; admin: boolean; prev_crash: string; temp_dir: string; reports_dir: string; addresses: string[];
}
interface EngineEvent {
  kind: string; phase?: string; message?: string; fraction: number; detail?: string; value?: number; unit?: string;
  errors?: number; sample?: { t: string; temp: number | null; freq: number | null }; planned?: number;
}

const S = {
  view: 'test' as View,
  stage: 'setup' as Stage,
  machine: null as Machine | null,
  form: {
    mode: 'stress', profile: 'quick', hours: 8, minutes: 30,
    cpu: true, ram: true, disk: true, net: false, server: '',
    diskDir: '', writeCap: 0, ramSize: 0, margin: 5, kernel: '', battery: false, raw: '', serial: '',
  },
  run: {
    phases: [] as string[], phase: '', done: new Set<string>(), fraction: 0, start: 0, planned: 0,
    detail: '', value: 0, unit: '', errors: 0, temp: NaN, freq: NaN, stopping: false, log: [] as string[],
  },
  result: null as ReportView | null,
  reports: [] as ReportView[],
  selected: new Set<string>(),
  diff: null as any,
  server: false,
  serverLog: [] as string[],
  port: 7420,
  error: '',
};

let charts: { temp?: LiveChart; freq?: LiveChart; thr?: LiveChart } = {};
let thrPhase = '';

const $ = (sel: string) => document.querySelector(sel) as HTMLElement;
const esc = (s: unknown) => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]!));

function fmtDur(sec: number): string {
  sec = Math.max(0, Math.round(sec));
  const h = Math.floor(sec / 3600), m = Math.floor(sec / 60) % 60, s = sec % 60;
  if (h) return `${h} h ${String(m).padStart(2, '0')}`;
  if (m) return `${m} min ${String(s).padStart(2, '0')}`;
  return `${s} s`;
}

// ---------- Plan ----------

function buildPlan(): any {
  const f = S.form;
  const p: any = {
    mode: f.mode, profile: f.mode === 'bench' ? 'bench' : f.profile, duration: 0,
    cpu: f.cpu, ram: f.ram, disk: f.disk || !!f.raw, net: f.net,
    kernel: f.kernel, ram_bytes: Math.round(f.ramSize * 2 ** 30),
    disk_dir: f.diskDir, disk_raw: f.raw, confirm_serial: f.serial,
    disk_write_cap: Math.round(f.writeCap * 2 ** 30), disk_file_size: 0,
    net_server: f.net ? f.server.trim() : '', net_streams: 4,
    thermal_margin: f.margin, allow_battery: f.battery, lang: lang(),
  };
  if (f.mode === 'stress' && f.profile === 'full') p.duration = f.hours * 3600e9;
  if (f.mode === 'stress' && f.profile === 'custom') p.duration = f.minutes * 60e9;
  return p;
}

// ---------- Rendu ----------

function render() {
  document.documentElement.lang = lang();
  $('#app').innerHTML = `
  <aside>
    <div class="brand">${logo()}<div><b>Rodage</b><small>${esc(t('tagline'))}</small></div></div>
    <nav>${(['test', 'reports', 'compare', 'server', 'machine'] as View[]).map(v =>
      `<button class="nav ${S.view === v ? 'on' : ''}" data-view="${v}">${esc(t('nav.' + v))}${v === 'server' && S.server ? ' <span class="dot"></span>' : ''}</button>`).join('')}</nav>
    <div class="lang">
      <button data-lang="fr" class="${lang() === 'fr' ? 'on' : ''}">FR</button>
      <button data-lang="en" class="${lang() === 'en' ? 'on' : ''}">EN</button>
    </div>
    <div class="ver">v${esc(S.machine?.version ?? '')} · GPL-3.0</div>
  </aside>
  <main>${S.error ? `<div class="banner fail">${esc(t('error'))} : ${esc(S.error)} <button class="link" data-act="dismiss">×</button></div>` : ''}${body()}</main>`;
  if (S.view === 'test' && S.stage === 'running') mountCharts();
}

function body(): string {
  switch (S.view) {
    case 'test': return S.stage === 'setup' ? setupView() : S.stage === 'running' ? runningView() : resultView();
    case 'reports': return reportsView();
    case 'compare': return compareView();
    case 'server': return serverView();
    case 'machine': return machineView();
  }
}

function logo(): string {
  return `<svg viewBox="0 0 32 32" width="34" height="34" aria-hidden="true"><circle cx="16" cy="16" r="14.5" fill="#34405a"/>
  <path d="M7 19 a9 9 0 0 1 18 0" fill="none" stroke="#5c7cfa" stroke-width="3" stroke-linecap="round"/>
  <path d="M16 19 L22 12" stroke="#fff" stroke-width="2.4" stroke-linecap="round"/><circle cx="16" cy="19" r="2.2" fill="#fff"/></svg>`;
}

function setupView(): string {
  const f = S.form, m = S.machine;
  const radio = (name: string, val: string, cur: string, title: string, desc: string) =>
    `<label class="card ${cur === val ? 'on' : ''}"><input type="radio" name="${name}" value="${val}" ${cur === val ? 'checked' : ''}><b>${esc(title)}</b><span>${esc(desc)}</span></label>`;
  const check = (k: 'cpu' | 'ram' | 'disk' | 'net') =>
    `<label class="card ${f[k] ? 'on' : ''}"><input type="checkbox" data-k="${k}" ${f[k] ? 'checked' : ''}><b>${esc(t(k))}</b><span>${esc(t(k + '.d'))}</span></label>`;
  const eligible = (m?.disks ?? []).filter(d => d.raw_eligible);
  return `
  <h1>${esc(t('nav.test'))}</h1>
  ${m?.prev_crash ? `<div class="banner warn">${esc(m.prev_crash)}</div>` : ''}
  ${m && !m.admin ? `<div class="banner info">${esc(t('noadmin'))}</div>` : ''}
  ${m?.has_battery ? `<div class="banner info">${esc(t('battery'))}</div>` : ''}
  <section><h2>${esc(t('mode'))}</h2><div class="cards two">
    ${radio('mode', 'bench', f.mode, t('mode.bench'), t('mode.bench.d'))}
    ${radio('mode', 'stress', f.mode, t('mode.stress'), t('mode.stress.d'))}
  </div></section>
  ${f.mode === 'stress' ? `<section><h2>${esc(t('profile'))}</h2><div class="cards four">
    ${(['quick', 'standard', 'full', 'custom']).map(p => radio('profile', p, f.profile, t('profile.' + p), t('profile.' + p + '.d'))).join('')}
  </div>
  ${f.profile === 'full' ? `<div class="row"><input type="range" min="4" max="12" step="1" value="${f.hours}" data-f="hours"><b>${f.hours} ${esc(t('hours'))}</b></div>` : ''}
  ${f.profile === 'custom' ? `<div class="row"><input type="number" min="1" max="720" value="${f.minutes}" data-f="minutes" class="num"> ${esc(t('minutes'))}</div>` : ''}
  </section>` : ''}
  <section><h2>${esc(t('tests'))}</h2><div class="cards four">${check('cpu')}${check('ram')}${check('disk')}${check('net')}</div>
  ${f.net ? `<div class="row"><label>${esc(t('net.server'))} <input type="text" placeholder="192.168.1.20" value="${esc(f.server)}" data-f="server"></label></div>` : ''}
  </section>
  <details ${f.raw || f.diskDir ? 'open' : ''}><summary>${esc(t('advanced'))}</summary><div class="grid">
    <label>${esc(t('adv.diskdir'))}<span class="row"><input type="text" value="${esc(f.diskDir)}" placeholder="${esc(m?.temp_dir ?? '')} (${esc(t('adv.default'))})" data-f="diskDir"><button data-act="choose">${esc(t('adv.choose'))}</button></span></label>
    <label>${esc(t('adv.writecap'))}<input type="number" min="0" step="1" value="${f.writeCap}" data-f="writeCap" class="num"></label>
    <label>${esc(t('adv.ramsize'))}<input type="number" min="0" step="0.5" value="${f.ramSize}" data-f="ramSize" class="num"></label>
    <label>${esc(t('adv.margin'))}<input type="number" min="0" max="40" step="1" value="${f.margin}" data-f="margin" class="num"></label>
    <label>${esc(t('adv.kernel'))}<select data-f="kernel"><option value="">${esc(t('adv.auto'))}</option>${(m?.kernels ?? []).map(k => `<option ${f.kernel === k ? 'selected' : ''}>${esc(k)}</option>`).join('')}</select></label>
    <label class="inline"><input type="checkbox" data-f="battery" ${f.battery ? 'checked' : ''}> ${esc(t('adv.battery'))}</label>
    <label class="danger">${esc(t('adv.raw'))}<select data-f="raw"><option value="">${esc(t('adv.raw.none'))}</option>${eligible.map(d => `<option value="${esc(d.name)}" ${f.raw === d.name ? 'selected' : ''}>${esc(d.name)} — ${esc(d.model)} ${esc(d.size_text)}</option>`).join('')}</select>
      ${f.raw ? `<small>${esc(t('adv.raw.warn'))}</small><input type="text" placeholder="${esc(t('adv.raw.serial'))}" value="${esc(f.serial)}" data-f="serial">` : ''}</label>
  </div></details>
  <div class="actions"><span class="note" id="budget"></span><button class="primary" data-act="start">${esc(t('start'))}</button></div>`;
}

function runningView(): string {
  const r = S.run;
  const el = (Date.now() - r.start) / 1000;
  const remaining = r.planned > 0 ? Math.max(0, r.planned - el) : NaN;
  return `
  <h1>${esc(t('running'))}</h1>
  <div class="progress"><div style="width:${(r.fraction * 100).toFixed(1)}%"></div></div>
  <div class="meta"><span>${(r.fraction * 100).toFixed(0)} %</span><span>${esc(t('elapsed'))} ${fmtDur(el)}</span>${Number.isFinite(remaining) ? `<span>${esc(t('remaining'))} ${fmtDur(remaining)}</span>` : ''}</div>
  <ol class="steps">${r.phases.map(p => `<li class="${r.done.has(p) ? 'done' : p === r.phase ? 'on' : ''}">${esc(t('phase.' + p))}</li>`).join('')}</ol>
  <div class="tiles">
    <div class="tile"><small>${esc(t('temp'))}</small><b id="v-temp">${Number.isFinite(r.temp) ? r.temp.toFixed(0) + ' °C' : esc(t('na'))}</b></div>
    <div class="tile"><small>${esc(t('freq'))}</small><b id="v-freq">${Number.isFinite(r.freq) ? r.freq.toFixed(0) + ' MHz' : esc(t('na'))}</b></div>
    <div class="tile"><small>${esc(t('throughput'))}</small><b id="v-thr">${r.value > 0 ? r.value.toFixed(1) + ' ' + esc(r.unit) : '—'}</b><span id="v-detail">${esc(r.detail)}</span></div>
    <div class="tile ${r.errors ? 'bad' : ''}"><small>${esc(t('errors'))}</small><b id="v-err">${r.errors}</b></div>
  </div>
  <div class="charts">
    <figure><figcaption>${esc(t('temp'))} (°C)</figcaption><canvas id="c-temp"></canvas></figure>
    <figure><figcaption>${esc(t('freq'))} (MHz)</figcaption><canvas id="c-freq"></canvas></figure>
    <figure class="wide"><figcaption>${esc(t('throughput'))} <span id="c-thr-unit">${esc(r.unit)}</span></figcaption><canvas id="c-thr"></canvas></figure>
  </div>
  <ul class="log">${r.log.slice(-4).map(l => `<li>${esc(l)}</li>`).join('')}</ul>
  <div class="actions"><button class="danger" data-act="stop" ${r.stopping ? 'disabled' : ''}>${esc(r.stopping ? t('stopping') : t('stop'))}</button></div>`;
}

function verdictCard(r: ReportView): string {
  return `<div class="verdict ${esc(r.verdict)}"><b>${esc(r.verdict_text)}</b><p>${esc(r.verdict_long)}</p><small>${esc(r.mode_text)} · ${esc(r.created)} · ${esc(r.elapsed)}</small></div>`;
}

function findingsList(r: ReportView): string {
  return `<ul class="findings">${r.findings.map(f => `<li class="${esc(f.sev)}"><span class="sev">${esc(t('sev.' + f.sev))}</span><span class="comp">${esc(f.comp)}</span>${esc(f.text)}</li>`).join('')}</ul>`;
}

function resultView(): string {
  const r = S.result!;
  return `
  <h1>${esc(t('result'))}</h1>
  ${verdictCard(r)}
  <section><h2>${esc(t('findings'))}</h2>${findingsList(r)}</section>
  <section><h2>${esc(t('limits'))}</h2><ul class="limits">${r.limits.map(l => `<li>${esc(l)}</li>`).join('')}</ul></section>
  <div class="actions">
    <button data-act="html" data-path="${esc(r.path)}">${esc(t('open.html'))}</button>
    <button data-act="pdf" data-path="${esc(r.path)}">${esc(t('export.pdf'))}</button>
    <button data-act="json" data-path="${esc(r.path)}">${esc(t('export.json'))}</button>
    <button class="primary" data-act="new">${esc(t('new.test'))}</button>
  </div>`;
}

function reportsView(): string {
  const rows = S.reports.map(r => `<tr>
    <td><input type="checkbox" data-sel="${esc(r.path)}" ${S.selected.has(r.path) ? 'checked' : ''}></td>
    <td>${esc(r.created)}</td><td>${esc(r.host)}</td><td>${esc(r.mode_text)}</td><td>${esc(r.elapsed)}</td>
    <td><span class="pill ${esc(r.verdict)}">${esc(r.verdict_text)}</span></td>
    <td class="btns"><button data-act="show" data-path="${esc(r.path)}">${esc(t('open'))}</button><button data-act="html" data-path="${esc(r.path)}">HTML</button><button data-act="pdf" data-path="${esc(r.path)}">PDF</button><button class="link" data-act="delete" data-path="${esc(r.path)}">${esc(t('delete'))}</button></td></tr>`).join('');
  return `
  <h1>${esc(t('nav.reports'))}</h1>
  <div class="actions left"><button data-act="folder">${esc(t('reports.folder'))}</button><button data-act="import">${esc(t('reports.import'))}</button>
  <button class="primary" data-act="compare" ${S.selected.size === 2 ? '' : 'disabled'}>${esc(t('reports.compare'))}</button><span class="note">${esc(t('reports.select2'))}</span></div>
  ${S.reports.length ? `<table class="list"><tr><th></th><th>${esc(t('date'))}</th><th>${esc(t('nav.machine'))}</th><th>${esc(t('mode'))}</th><th>${esc(t('duration'))}</th><th>${esc(t('verdict'))}</th><th></th></tr>${rows}</table>` : `<p class="note">${esc(t('reports.empty'))}</p>`}`;
}

function compareView(): string {
  const d = S.diff;
  if (!d) return `<h1>${esc(t('compare.title'))}</h1><p class="note">${esc(t('compare.none'))}</p>`;
  const val = (v: number, ok: boolean) => !ok ? '—' : Math.abs(v) >= 100 || Number.isInteger(v) ? v.toFixed(0) : v.toFixed(2);
  return `<h1>${esc(t('compare.title'))}</h1>
  ${d.mismatch ? `<div class="banner warn">${esc(d.mismatch)}</div>` : ''}
  <table class="list"><tr><th></th><th>${esc(t('compare.metric'))}</th><th class="n">A</th><th class="n">B</th><th class="n">${esc(t('compare.delta'))}</th></tr>
  ${d.rows.map((r: any) => `<tr><td>${esc(r.section)}</td><td>${esc(r.metric)}${r.unit ? ` <small>(${esc(r.unit)})</small>` : ''}</td><td class="n">${val(r.a, r.has_a)}</td><td class="n">${val(r.b, r.has_b)}</td>
  <td class="n ${r.better > 0 ? 'up' : r.better < 0 ? 'down' : ''}">${r.has_a && r.has_b && r.a ? (r.delta_pct >= 0 ? '+' : '') + r.delta_pct.toFixed(1) + ' %' : ''}</td></tr>`).join('')}</table>
  ${d.html_path ? `<div class="actions left"><button data-act="openfile" data-path="${esc(d.html_path)}">${esc(t('open.html'))}</button></div>` : ''}`;
}

function serverView(): string {
  const m = S.machine;
  return `<h1>${esc(t('server.title'))}</h1><p class="note">${esc(t('server.d'))}</p>
  <div class="row"><label>${esc(t('server.port'))} <input type="number" class="num" value="${S.port}" data-port ${S.server ? 'disabled' : ''}></label>
  ${S.server ? `<button class="danger" data-act="srvstop">${esc(t('server.stop'))}</button>` : `<button class="primary" data-act="srvstart">${esc(t('server.start'))}</button>`}</div>
  ${S.server ? `<div class="banner ok">${esc(t('server.on'))} <b>${(m?.addresses ?? []).map(a => esc(a + ':' + S.port)).join(', ') || '—'}</b></div>` : `<p class="note">${esc(t('server.off'))}</p>`}
  <pre class="logbox">${S.serverLog.slice(-30).map(esc).join('\n')}</pre>`;
}

function machineView(): string {
  const m = S.machine;
  if (!m) return `<p class="note">${esc(t('loading'))}</p>`;
  return `<h1>${esc(t('machine.title'))}</h1>
  <div class="actions left"><button data-act="refresh">${esc(t('machine.refresh'))}</button></div>
  <table class="kv">${m.config.map(r => `<tr><td>${esc(r.k)}</td><td>${esc(r.v)}</td></tr>`).join('')}
  <tr><td>${esc(t('temp'))}</td><td>${m.temp_c >= 0 ? m.temp_c.toFixed(0) + ' °C' : esc(t('na'))}</td></tr>
  <tr><td>${esc(t('freq'))}</td><td>${m.freq_mhz >= 0 ? m.freq_mhz.toFixed(0) + ' MHz' : esc(t('na'))}</td></tr></table>
  <section><h2>${esc(t('machine.sensors'))}</h2><p>${esc(m.sensor_text)}</p><ul class="limits">${m.sensor_details.map(d => `<li>${esc(d)}</li>`).join('')}</ul></section>
  <section><h2>${esc(t('machine.disks'))}</h2><table class="list"><tr><th></th><th>${esc(t('model'))}</th><th>${esc(t('serial'))}</th><th class="n">${esc(t('size'))}</th><th></th></tr>
  ${m.disks.map(d => `<tr><td>${esc(d.name)}</td><td>${esc(d.model)}</td><td>${esc(d.serial)}</td><td class="n">${esc(d.size_text)}</td>
  <td>${esc(d.bus)}${d.system ? ' · ' + esc(t('system')) : ''}${d.partitions ? ` · ${d.partitions} ${esc(t('partitions'))}` : ''}${d.raw_eligible ? ' · ' + esc(t('raw.ok')) : ''}</td></tr>`).join('')}</table></section>`;
}

// ---------- Courbes en direct ----------

function mountCharts() {
  const make = (id: string, color: string) => new LiveChart(document.getElementById(id) as HTMLCanvasElement, color);
  const old = charts;
  charts = { temp: make('c-temp', '#d9480f'), freq: make('c-freq', '#1c7ed6'), thr: make('c-thr', '#2f9e44') };
  // Conserve l'historique lors d'un nouveau rendu.
  for (const k of ['temp', 'freq', 'thr'] as const) {
    const o: any = old[k], n: any = charts[k];
    if (o) { n.xs = o.xs; n.ys = o.ys; n.draw(); }
  }
}

function updateLive() {
  const r = S.run;
  const set = (id: string, v: string) => { const e = document.getElementById(id); if (e) e.textContent = v; };
  set('v-temp', Number.isFinite(r.temp) ? r.temp.toFixed(0) + ' °C' : t('na'));
  set('v-freq', Number.isFinite(r.freq) ? r.freq.toFixed(0) + ' MHz' : t('na'));
  set('v-thr', r.value > 0 ? r.value.toFixed(1) + ' ' + r.unit : '—');
  set('v-detail', r.detail);
  set('v-err', String(r.errors));
  set('c-thr-unit', r.unit);
  const bar = document.querySelector('.progress > div') as HTMLElement | null;
  if (bar) bar.style.width = (r.fraction * 100).toFixed(1) + '%';
  const meta = document.querySelector('.meta');
  if (meta) {
    const el = (Date.now() - r.start) / 1000;
    const rem = r.planned > 0 ? Math.max(0, r.planned - el) : NaN;
    meta.innerHTML = `<span>${(r.fraction * 100).toFixed(0)} %</span><span>${esc(t('elapsed'))} ${fmtDur(el)}</span>${Number.isFinite(rem) ? `<span>${esc(t('remaining'))} ${fmtDur(rem)}</span>` : ''}`;
  }
}

function onEvent(e: EngineEvent) {
  const r = S.run;
  r.fraction = e.fraction ?? r.fraction;
  const now = (Date.now() - r.start) / 1000;
  switch (e.kind) {
    case 'start':
      r.planned = (e.planned ?? 0) / 1e9;
      r.phases = (e.message ?? '').split(',').filter(Boolean);
      render();
      return;
    case 'phase':
      if (r.phase) r.done.add(r.phase);
      r.phase = e.phase ?? '';
      r.value = 0; r.detail = ''; r.unit = '';
      render();
      return;
    case 'sample': {
      const s = e.sample!;
      r.temp = s.temp ?? NaN;
      r.freq = s.freq ?? NaN;
      if (s.temp != null) charts.temp?.push(now, s.temp);
      if (s.freq != null) charts.freq?.push(now, s.freq);
      break;
    }
    case 'progress':
      r.detail = e.detail ?? '';
      r.value = e.value ?? 0;
      r.unit = e.unit ?? '';
      r.errors = e.errors ?? 0;
      if (e.phase !== thrPhase) { thrPhase = e.phase ?? ''; charts.thr?.reset(); }
      if (r.value > 0 && Number.isFinite(r.value)) charts.thr?.push(now, r.value);
      break;
    case 'log':
      r.log.push(e.message ?? '');
      break;
  }
  updateLive();
}

// ---------- Actions ----------

async function refreshMachine() {
  S.machine = await App.GetMachine() as any;
  render();
  updateBudget();
}

async function refreshReports() {
  S.reports = (await App.ListReports() as any) ?? [];
  for (const p of [...S.selected]) if (!S.reports.find(r => r.path === p)) S.selected.delete(p);
  render();
}

async function updateBudget() {
  const el = document.getElementById('budget');
  if (!el) return;
  const plan = buildPlan();
  if (plan.mode === 'bench') { el.textContent = ''; return; }
  try {
    const b = await App.Budget(plan) as Record<string, number>;
    el.textContent = t('plan.summary') + ' : ' + Object.entries(b).map(([k, v]) => `${t('phase.' + k)} ${fmtDur(v)}`).join(' · ');
  } catch { el.textContent = ''; }
}

async function start() {
  S.error = '';
  const plan = buildPlan();
  const msg = await App.ValidatePlan(plan);
  if (msg) { S.error = msg; render(); return; }
  S.run = { phases: [], phase: '', done: new Set(), fraction: 0, start: Date.now(), planned: 0, detail: '', value: 0, unit: '', errors: 0, temp: NaN, freq: NaN, stopping: false, log: [] };
  charts = {};
  thrPhase = '';
  S.stage = 'running';
  render();
  try { await App.StartRun(plan); } catch (err) { S.error = String(err); S.stage = 'setup'; render(); }
}

async function act(name: string, el: HTMLElement) {
  const path = el.dataset.path ?? '';
  try {
    switch (name) {
      case 'start': await start(); break;
      case 'stop': S.run.stopping = true; render(); await App.StopRun(); break;
      case 'new': S.stage = 'setup'; S.result = null; await refreshMachine(); break;
      case 'choose': { const d = await App.ChooseDirectory(); if (d) { S.form.diskDir = d; render(); } break; }
      case 'html': await App.OpenReportHTML(path); break;
      case 'pdf': { const p = await App.ExportPDF(path); if (p) toast(t('saved') + p); break; }
      case 'json': { const p = await App.ExportJSON(path); if (p) toast(t('saved') + p); break; }
      case 'folder': await App.OpenReportsFolder(); break;
      case 'import': await App.ImportReport(); await refreshReports(); break;
      case 'delete': if (confirm(t('confirm.delete'))) { await App.DeleteReport(path); await refreshReports(); } break;
      case 'show': S.result = await App.GetReport(path) as any; S.view = 'test'; S.stage = 'result'; render(); break;
      case 'compare': { const [a, b] = [...S.selected]; S.diff = await App.Compare(a, b); S.view = 'compare'; render(); break; }
      case 'openfile': await App.OpenFile(path); break;
      case 'srvstart': await App.StartServer(':' + S.port); S.server = true; render(); break;
      case 'srvstop': await App.StopServer(); S.server = false; render(); break;
      case 'refresh': S.machine = null; render(); await refreshMachine(); break;
      case 'dismiss': S.error = ''; render(); break;
    }
  } catch (err) {
    S.error = String(err);
    render();
  }
}

function toast(msg: string) {
  const d = document.createElement('div');
  d.className = 'toast';
  d.textContent = msg;
  document.body.appendChild(d);
  setTimeout(() => d.remove(), 4000);
}

// ---------- Événements DOM ----------

document.addEventListener('click', e => {
  const el = (e.target as HTMLElement).closest('button') as HTMLElement | null;
  if (!el) return;
  if (el.dataset.view) {
    S.view = el.dataset.view as View;
    if (S.view === 'reports') refreshReports();
    render();
    if (S.view === 'test') updateBudget();
    return;
  }
  if (el.dataset.lang) {
    setLang(el.dataset.lang as Lang);
    App.SetLang(el.dataset.lang);
    const reload = async () => {
      if (S.result) S.result = await App.GetReport(S.result.path) as any;
      if (S.view === 'reports') await refreshReports();
      await refreshMachine();
    };
    render();
    reload();
    return;
  }
  if (el.dataset.act) act(el.dataset.act, el);
});

document.addEventListener('change', e => {
  const el = e.target as HTMLInputElement;
  const f: any = S.form;
  if (el.name === 'mode' || el.name === 'profile') { f[el.name] = el.value; render(); updateBudget(); return; }
  if (el.dataset.k) { f[el.dataset.k] = el.checked; render(); updateBudget(); return; }
  if (el.dataset.sel) { el.checked ? S.selected.add(el.dataset.sel) : S.selected.delete(el.dataset.sel); if (S.selected.size > 2) { S.selected.delete([...S.selected][0]); } render(); return; }
  if (el.dataset.port !== undefined) { S.port = Number(el.value) || 7420; return; }
  if (el.dataset.f) {
    const k = el.dataset.f;
    f[k] = el.type === 'checkbox' ? el.checked : el.type === 'number' || el.type === 'range' ? Number(el.value) : el.value;
    if (k === 'raw' || k === 'hours') render();
    updateBudget();
  }
});

document.addEventListener('input', e => {
  const el = e.target as HTMLInputElement;
  if (el.dataset.f && (el.type === 'text' || el.type === 'number')) {
    (S.form as any)[el.dataset.f] = el.type === 'number' ? Number(el.value) : el.value;
  }
});

// ---------- Démarrage ----------

EventsOn('rodage:event', (e: EngineEvent) => { if (S.stage === 'running') onEvent(e); });
EventsOn('rodage:done', (v: ReportView) => { S.result = v; S.stage = 'result'; S.view = 'test'; render(); });
EventsOn('rodage:error', (m: string) => { S.error = m; if (S.stage === 'running') S.stage = 'setup'; render(); });
EventsOn('rodage:server', (on: boolean) => { S.server = on; render(); });
EventsOn('rodage:serverlog', (l: string) => { S.serverLog.push(l); if (S.view === 'server') render(); });
setInterval(() => { if (S.stage === 'running') updateLive(); }, 1000);

(async () => {
  render();
  const m = await App.GetMachine() as any;
  setLang((m.lang === 'en' ? 'en' : 'fr') as Lang);
  S.machine = m;
  S.server = await App.ServerRunning();
  if (await App.IsRunning()) { S.stage = 'running'; S.run.start = Date.now(); }
  render();
  updateBudget();
})();
