const STORAGE_KEY = "custom-business-metrics.webview.settings.v2";

const state = {
  settings: {
    apiUrl: "http://localhost:8080",
    apiKey: "",
    rangeMode: "15m",
    rangeFrom: "",
    rangeTo: "",
    refreshInterval: 5000,
  },
  timer: null,
  events: [],
  processes: [],
  chartProcesses: [],
};

const els = {
  apiUrl: document.querySelector("#api-url"),
  apiKey: document.querySelector("#api-key"),
  rangeMode: document.querySelector("#range-mode"),
  customRange: document.querySelector("#custom-range"),
  rangeFrom: document.querySelector("#range-from"),
  rangeTo: document.querySelector("#range-to"),
  refreshInterval: document.querySelector("#refresh-interval"),
  refreshNow: document.querySelector("#refresh-now"),
  openConfig: document.querySelector("#open-config"),
  saveConfig: document.querySelector("#save-config"),
  configModal: document.querySelector("#config-modal"),
  dot: document.querySelector("#status-dot"),
  statusText: document.querySelector("#status-text"),
  lastUpdate: document.querySelector("#last-update"),
  chartSubtitle: document.querySelector("#chart-subtitle"),
  hourlyChart: document.querySelector("#hourly-chart"),
  processSummary: document.querySelector("#process-summary"),
  processFilter: document.querySelector("#process-filter"),
  processSearch: document.querySelector("#process-search"),
  processList: document.querySelector("#process-list"),
  processModal: document.querySelector("#process-modal"),
  modalClose: document.querySelector("#modal-close"),
  modalTitle: document.querySelector("#modal-title"),
  modalCopy: document.querySelector("#modal-copy"),
  modalBody: document.querySelector("#modal-body"),
};

function loadSettings() {
  try {
    state.settings = { ...state.settings, ...JSON.parse(localStorage.getItem(STORAGE_KEY) || "{}") };
  } catch {
    localStorage.removeItem(STORAGE_KEY);
  }
  els.apiUrl.value = state.settings.apiUrl;
  els.apiKey.value = state.settings.apiKey;
  els.rangeMode.value = state.settings.rangeMode;
  els.rangeFrom.value = state.settings.rangeFrom;
  els.rangeTo.value = state.settings.rangeTo;
  els.refreshInterval.value = String(state.settings.refreshInterval);
  syncRangeControls();
}

function saveSettings() {
  state.settings = {
    apiUrl: normalizeBaseURL(els.apiUrl.value),
    apiKey: els.apiKey.value.trim(),
    rangeMode: els.rangeMode.value,
    rangeFrom: els.rangeFrom.value,
    rangeTo: els.rangeTo.value,
    refreshInterval: Number(els.refreshInterval.value),
  };
  localStorage.setItem(STORAGE_KEY, JSON.stringify(state.settings));
  scheduleRefresh();
}

function normalizeBaseURL(value) {
  return String(value || "http://localhost:8080").trim().replace(/\/+$/, "");
}

function requestHeaders() {
  const headers = { "Content-Type": "application/json" };
  if (state.settings.apiKey) {
    headers.Authorization = `Bearer ${state.settings.apiKey}`;
    headers["X-API-Key"] = state.settings.apiKey;
  }
  return headers;
}

function endpoint(path, params = {}) {
  const url = new URL(path, state.settings.apiUrl);
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== "") url.searchParams.set(key, value);
  });
  return url;
}

async function getJSON(path, params = {}) {
  const response = await fetch(endpoint(path, params), { headers: requestHeaders() });
  if (!response.ok) throw new Error(`${response.status} ${response.statusText}`);
  return response.json();
}

function timeWindow() {
  if (state.settings.rangeMode === "custom") {
    const from = state.settings.rangeFrom ? new Date(state.settings.rangeFrom) : new Date(Date.now() - 15 * 60 * 1000);
    const to = state.settings.rangeTo ? new Date(state.settings.rangeTo) : new Date();
    return { from: from.toISOString(), to: to.toISOString() };
  }
  const amount = Number(state.settings.rangeMode.match(/\d+/)?.[0] || 15);
  const unit = state.settings.rangeMode.replace(String(amount), "");
  const multiplier = unit === "h" ? 60 * 60 * 1000 : 60 * 1000;
  const to = new Date();
  const from = new Date(to.getTime() - amount * multiplier);
  return { from: from.toISOString(), to: to.toISOString() };
}

async function refreshData() {
  saveSettings();
  try {
    const window = timeWindow();
    const chartWindow = hourlyChartWindow();
    const [records, chartRecords] = await Promise.all([
      getJSON("/v1/metrics/events", {
        ...window,
        source: "routing-slip-app",
        limit: 1000,
      }),
      getJSON("/v1/metrics/events", {
        ...chartWindow,
        source: "routing-slip-app",
        limit: 5000,
      }),
    ]);
    state.events = records.map((record) => record.event || record);
    state.processes = groupProcessEvents(records);
    state.chartProcesses = groupProcessEvents(chartRecords);
    setStatus(true);
    renderAll();
  } catch (error) {
    setStatus(false, error.message);
  }
}

function groupProcessEvents(records) {
  const groups = new Map();
  records.forEach((record) => {
    const event = record.event || record;
    const tags = event.tags || {};
    const key = tags.correlation_id || tags.message_id || event.trace_id || record.id || `${event.workflow}-${event.timestamp}`;
    if (!groups.has(key)) {
      groups.set(key, {
        id: key,
        workflow: event.workflow || "-",
        messageId: tags.message_id || "-",
        correlationId: tags.correlation_id || "-",
        traceId: event.trace_id || tags.trace_id || "-",
        startedAt: event.timestamp,
        updatedAt: event.timestamp,
        tags: {},
        events: [],
        completed: 0,
        failed: 0,
        stopped: 0,
        totalSteps: Number(tags.total_steps || 0),
      });
    }
    const group = groups.get(key);
    group.events.push(event);
    group.workflow = event.workflow || group.workflow;
    group.messageId = tags.message_id || group.messageId;
    group.correlationId = tags.correlation_id || group.correlationId;
    group.traceId = event.trace_id || tags.trace_id || group.traceId;
    group.totalSteps = Math.max(group.totalSteps, Number(tags.total_steps || 0));
    group.startedAt = earlier(group.startedAt, event.timestamp);
    group.updatedAt = later(group.updatedAt, event.timestamp);
    Object.assign(group.tags, tags);
    if (event.name === "routing_slip.step.completed") group.completed += 1;
    if (event.name === "routing_slip.step.failed") group.failed += 1;
    if (event.name === "routing_slip.step.stopped") group.stopped += 1;
  });

  return [...groups.values()]
    .map((group) => {
      const expected = group.totalSteps || inferExpectedSteps(group);
      group.status = group.failed > 0 ? "failed" : group.stopped > 0 ? "stopped" : group.completed >= expected ? "completed" : "running";
      group.expectedSteps = expected;
      group.remaining = Math.max(expected - group.completed - group.failed - group.stopped, 0);
      group.durationMs = Math.max(0, new Date(group.updatedAt) - new Date(group.startedAt));
      group.events.sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp));
      return group;
    })
    .sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt));
}

function inferExpectedSteps(group) {
  const indexes = group.events.map((event) => Number((event.tags || {}).step_index || 0));
  return Math.max(...indexes, group.completed + group.failed + group.stopped, 1);
}

function renderAll() {
  const window = timeWindow();
  els.lastUpdate.textContent = `Atualizado ${formatDateTime(new Date().toISOString())}`;
  const chartWindow = hourlyChartWindow();
  els.chartSubtitle.textContent = `Ultimas 24 horas: ${formatDateTime(chartWindow.from)} - ${formatDateTime(chartWindow.to)}`;
  renderHourlyChart();
  renderProcesses();
}

function renderHourlyChart() {
  const buckets = hourlyBuckets(state.chartProcesses);
  const canvas = els.hourlyChart;
  const context = prepareCanvas(canvas);
  const rect = canvas.getBoundingClientRect();
  const pad = { top: 18, right: 18, bottom: 34, left: 44 };
  const width = rect.width - pad.left - pad.right;
  const height = rect.height - pad.top - pad.bottom;
  context.clearRect(0, 0, rect.width, rect.height);
  drawGrid(context, pad, width, height);

  if (buckets.length === 0) {
    context.fillStyle = "#687782";
    context.fillText("Sem processamentos nas ultimas 24h", pad.left + 8, pad.top + 24);
    return;
  }

  const max = Math.max(...buckets.map((bucket) => bucket.value), 1);
  const gap = Math.max(4, Math.min(12, width / Math.max(buckets.length, 1) * 0.18));
  const barWidth = Math.max(8, (width - gap * (buckets.length - 1)) / buckets.length);

  buckets.forEach((bucket, index) => {
    const x = pad.left + index * (barWidth + gap);
    const barHeight = (bucket.value / max) * height;
    const y = pad.top + height - barHeight;
    context.fillStyle = bucket.failed > 0 ? "#d83b3b" : "#6bb8ff";
    context.fillRect(x, y, barWidth, barHeight);
    if (index % Math.ceil(buckets.length / 8) === 0 || buckets.length <= 8) {
      context.fillStyle = "#687782";
      context.fillText(bucket.label, x, pad.top + height + 20);
    }
  });

  context.fillStyle = "#35424a";
  context.fillText(String(max), 8, pad.top + 5);
  context.fillText("0", 16, pad.top + height + 4);
}

function hourlyBuckets(processes) {
  const window = hourlyChartWindow();
  const from = floorHour(new Date(window.from));
  const to = floorHour(new Date(window.to));
  const buckets = [];
  for (let cursor = new Date(from); cursor <= to; cursor = new Date(cursor.getTime() + 60 * 60 * 1000)) {
    buckets.push({ key: cursor.toISOString(), label: formatHour(cursor), value: 0, failed: 0 });
  }
  const index = new Map(buckets.map((bucket) => [bucket.key, bucket]));
  processes.forEach((process) => {
    const key = floorHour(new Date(process.startedAt)).toISOString();
    const bucket = index.get(key);
    if (!bucket) return;
    bucket.value += 1;
    if (process.status === "failed") bucket.failed += 1;
  });
  return buckets;
}

function hourlyChartWindow() {
  const to = new Date();
  const from = new Date(to.getTime() - 24 * 60 * 60 * 1000);
  return { from: from.toISOString(), to: to.toISOString() };
}

function renderProcesses() {
  const filtered = filterProcesses(state.processes, els.processFilter.value.trim());
  const completed = filtered.filter((item) => item.status === "completed").length;
  const failed = filtered.filter((item) => item.status === "failed").length;
  const running = filtered.filter((item) => item.status === "running").length;
  els.processSummary.textContent = `${filtered.length} processos, ${completed} concluidos, ${failed} falhas, ${running} em execucao`;

  if (filtered.length === 0) {
    els.processList.innerHTML = `<tr><td colspan="8" class="empty">Nenhum processamento encontrado no periodo.</td></tr>`;
    return;
  }

  els.processList.innerHTML = filtered.map(processRow).join("");
  els.processList.querySelectorAll("[data-process-id]").forEach((row) => {
    row.addEventListener("click", () => openProcess(row.dataset.processId));
  });
}

function processRow(group) {
  const tags = group.tags || {};
  const chipKeys = ["order_id", "pedido_id", "customer_id", "id_cliente", "event_id", "trace_id"];
  const chips = chipKeys
    .filter((key) => tags[key])
    .slice(0, 3)
    .map((key) => `<span>${escapeHTML(key)}:${escapeHTML(tags[key])}</span>`)
    .join("");
  return `
    <tr data-process-id="${escapeHTML(group.id)}">
      <td><time>${escapeHTML(formatDateTime(group.updatedAt))}</time></td>
      <td><strong>${escapeHTML(group.workflow)}</strong></td>
      <td><code>${escapeHTML(group.correlationId)}</code></td>
      <td><code>${escapeHTML(group.messageId)}</code></td>
      <td>${escapeHTML(formatDuration(group.durationMs))}</td>
      <td>${group.completed}/${group.expectedSteps}</td>
      <td><span class="status ${group.status}">${escapeHTML(statusLabel(group.status))}</span></td>
      <td><div class="tag-list">${chips || `<span>trace:${escapeHTML(group.traceId)}</span>`}</div></td>
    </tr>
  `;
}

function filterProcesses(processes, rawFilter) {
  if (!rawFilter) return processes;
  const filters = rawFilter.split(/[,\s]+/).map(parseAttributeFilter).filter(Boolean);
  if (filters.length === 0) return processes;
  return processes.filter((process) =>
    filters.every(({ key, value }) => String(processSearchFields(process)[key] || "").toLowerCase().includes(value.toLowerCase())),
  );
}

function parseAttributeFilter(text) {
  const [key, ...rest] = String(text).split(":");
  const value = rest.join(":");
  if (!key || !value) return null;
  return { key: key.trim(), value: value.trim() };
}

function processSearchFields(process) {
  return {
    workflow: process.workflow,
    message_id: process.messageId,
    correlation_id: process.correlationId,
    trace_id: process.traceId,
    status: process.status,
    ...process.tags,
  };
}

function openProcess(id) {
  const process = state.processes.find((item) => item.id === id);
  if (!process) return;
  els.modalTitle.textContent = process.workflow;
  els.modalCopy.textContent = `${process.correlationId} - ${statusLabel(process.status)} - ${formatDuration(process.durationMs)}`;
  els.modalBody.innerHTML = `
    <section class="process-kpis">
      <div><strong>${process.completed}</strong><span>Concluidas</span></div>
      <div><strong>${process.failed}</strong><span>Falhas</span></div>
      <div><strong>${process.stopped}</strong><span>Paradas</span></div>
      <div><strong>${process.remaining}</strong><span>Faltantes</span></div>
    </section>
    <section class="modal-tags">
      ${Object.entries(process.tags)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([key, value]) => `<span><strong>${escapeHTML(key)}</strong>${escapeHTML(value)}</span>`)
        .join("")}
    </section>
    <section class="step-list">
      ${processSteps(process).map((step, index) => stepItem(step, index)).join("")}
    </section>
  `;
  els.modalBody.querySelectorAll("[data-step-index]").forEach((item) => {
    item.addEventListener("click", () => item.classList.toggle("open"));
  });
  els.processModal.showModal();
}

function processSteps(process) {
  const groups = new Map();
  process.events.forEach((event) => {
    const tags = event.tags || {};
    const key = `${tags.step_index || "0"}:${event.step || tags.handler || event.name}`;
    if (!groups.has(key)) {
      groups.set(key, {
        index: Number(tags.step_index || 0),
        step: event.step || tags.handler || event.name,
        handler: tags.handler || event.step || "-",
        status: "running",
        startedAt: event.timestamp,
        updatedAt: event.timestamp,
        duration: "",
        input: tags.input_value || "",
        rule: tags.rule_applied || "",
        output: tags.output_value || "",
        failure: tags.failure_reason || "",
        events: [],
      });
    }
    const step = groups.get(key);
    step.events.push(event);
    step.startedAt = earlier(step.startedAt, event.timestamp);
    step.updatedAt = later(step.updatedAt, event.timestamp);
    if (tags.duration_ms) step.duration = `${tags.duration_ms} ms`;
    if (tags.input_value) step.input = tags.input_value;
    if (tags.rule_applied) step.rule = tags.rule_applied;
    if (tags.output_value) step.output = tags.output_value;
    if (tags.failure_reason) step.failure = tags.failure_reason;
    if (["success", "failed", "stopped"].includes(event.status)) step.status = event.status;
  });
  return [...groups.values()].sort((a, b) => a.index - b.index || new Date(a.startedAt) - new Date(b.startedAt));
}

function stepItem(step, index) {
  return `
    <article class="step-item ${step.status}" data-step-index="${index}">
      <div class="step-summary">
        <time>${escapeHTML(formatDateTime(step.startedAt))}</time>
        <div>
          <strong>${escapeHTML(step.step)}</strong>
          <p>${escapeHTML(step.handler)} - ${escapeHTML(step.duration || "-")}</p>
        </div>
        <span class="status ${step.status}">${escapeHTML(statusLabel(step.status))}</span>
      </div>
      <dl class="step-details">
        <div><dt>Valor de entrada</dt><dd>${formatDetail(step.input)}</dd></div>
        <div><dt>Regra aplicada</dt><dd>${formatDetail(step.rule)}</dd></div>
        <div><dt>Valor de saida</dt><dd>${formatDetail(step.output)}</dd></div>
        <div><dt>Status</dt><dd>${escapeHTML(statusLabel(step.status))}</dd></div>
        <div><dt>Motivo da falha</dt><dd>${escapeHTML(step.failure || "-")}</dd></div>
      </dl>
    </article>
  `;
}

function syncRangeControls() {
  const visible = els.rangeMode.value === "custom";
  els.customRange.hidden = !visible;
  els.customRange.classList.toggle("visible", visible);
}

function scheduleRefresh() {
  clearInterval(state.timer);
  if (state.settings.refreshInterval > 0) {
    state.timer = setInterval(refreshData, state.settings.refreshInterval);
  }
}

function setStatus(ok, detail = "") {
  els.dot.className = `dot ${ok ? "ok" : "fail"}`;
  els.statusText.textContent = ok ? "Online" : `Offline ${detail}`;
}

function statusLabel(status) {
  return { completed: "OK", success: "OK", failed: "Erro", stopped: "Parado", running: "Em andamento" }[status] || status || "-";
}

function formatDetail(value) {
  if (!value) return "-";
  return `<code>${escapeHTML(prettyJSON(value))}</code>`;
}

function prettyJSON(value) {
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch {
    return String(value);
  }
}

function formatDateTime(value) {
  if (!value) return "-";
  return new Intl.DateTimeFormat("pt-BR", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(new Date(value));
}

function formatHour(value) {
  return new Intl.DateTimeFormat("pt-BR", { hour: "2-digit", minute: "2-digit" }).format(value);
}

function formatDuration(ms) {
  if (!Number.isFinite(ms) || ms <= 0) return "0 ms";
  if (ms < 1000) return `${ms} ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(2)} s`;
  return `${Math.floor(ms / 60_000)}m ${Math.round((ms % 60_000) / 1000)}s`;
}

function earlier(a, b) {
  return new Date(a) <= new Date(b) ? a : b;
}

function later(a, b) {
  return new Date(a) >= new Date(b) ? a : b;
}

function floorHour(value) {
  const date = new Date(value);
  date.setMinutes(0, 0, 0);
  return date;
}

function prepareCanvas(canvas) {
  const ratio = window.devicePixelRatio || 1;
  const rect = canvas.getBoundingClientRect();
  canvas.width = Math.max(1, Math.floor(rect.width * ratio));
  canvas.height = Math.max(1, Math.floor(rect.height * ratio));
  const context = canvas.getContext("2d");
  context.scale(ratio, ratio);
  context.font = "12px Inter, system-ui, sans-serif";
  return context;
}

function drawGrid(context, pad, width, height) {
  context.strokeStyle = "#e6eaee";
  context.lineWidth = 1;
  context.beginPath();
  for (let i = 0; i <= 4; i += 1) {
    const y = pad.top + (height / 4) * i;
    context.moveTo(pad.left, y);
    context.lineTo(pad.left + width, y);
  }
  context.stroke();
}

function escapeHTML(value) {
  return String(value ?? "").replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#039;" })[char]);
}

els.openConfig.addEventListener("click", () => els.configModal.showModal());
els.saveConfig.addEventListener("click", () => {
  saveSettings();
  refreshData();
});
els.refreshNow.addEventListener("click", refreshData);
els.rangeMode.addEventListener("change", () => {
  syncRangeControls();
  refreshData();
});
els.rangeFrom.addEventListener("change", refreshData);
els.rangeTo.addEventListener("change", refreshData);
els.processSearch.addEventListener("click", renderProcesses);
els.processFilter.addEventListener("input", renderProcesses);
els.processFilter.addEventListener("keydown", (event) => {
  if (event.key === "Enter") renderProcesses();
});
els.modalClose.addEventListener("click", () => els.processModal.close());
[els.processModal, els.configModal].forEach((modal) => {
  modal.addEventListener("click", (event) => {
    if (event.target === modal) modal.close();
  });
});
window.addEventListener("resize", renderHourlyChart);

loadSettings();
scheduleRefresh();
refreshData();
