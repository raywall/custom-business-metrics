const state = {
  dashboards: [],
  dashboard: null,
  selectedWidgetId: "",
  editing: false,
  timer: null,
  processGroups: [],
};

const els = {
  apiUrl: document.querySelector("#api-url"),
  dashboardSelect: document.querySelector("#dashboard-select"),
  dashboardName: document.querySelector("#dashboard-name"),
  dashboardDescription: document.querySelector("#dashboard-description"),
  dashboardJSON: document.querySelector("#dashboard-json"),
  dashboardTitle: document.querySelector("#dashboard-title"),
  dashboardCopy: document.querySelector("#dashboard-copy"),
  grid: document.querySelector("#dashboard-grid"),
  window: document.querySelector("#window"),
  refresh: document.querySelector("#refresh"),
  retentionDays: document.querySelector("#retention-days"),
  saveConfig: document.querySelector("#save-config"),
  lastUpdate: document.querySelector("#last-update"),
  dot: document.querySelector("#status-dot"),
  statusText: document.querySelector("#status-text"),
  toggleEdit: document.querySelector("#toggle-edit"),
  saveDashboard: document.querySelector("#save-dashboard"),
  newDashboard: document.querySelector("#new-dashboard"),
  deleteDashboard: document.querySelector("#delete-dashboard"),
  refreshNow: document.querySelector("#refresh-now"),
  applyJSON: document.querySelector("#apply-json"),
  addWidget: document.querySelector("#add-widget"),
  widgetType: document.querySelector("#widget-type"),
  widgetTitle: document.querySelector("#widget-title"),
  widgetQuery: document.querySelector("#widget-query"),
  widgetX: document.querySelector("#widget-x"),
  widgetY: document.querySelector("#widget-y"),
  widgetW: document.querySelector("#widget-w"),
  widgetH: document.querySelector("#widget-h"),
  updateWidget: document.querySelector("#update-widget"),
  duplicateWidget: document.querySelector("#duplicate-widget"),
  removeWidget: document.querySelector("#remove-widget"),
  processFilter: document.querySelector("#process-filter"),
  processSearch: document.querySelector("#process-search"),
  processSummary: document.querySelector("#process-summary"),
  processList: document.querySelector("#process-list"),
  processModal: document.querySelector("#process-modal"),
  modalClose: document.querySelector("#modal-close"),
  modalTitle: document.querySelector("#modal-title"),
  modalCopy: document.querySelector("#modal-copy"),
  modalBody: document.querySelector("#modal-body"),
};

function endpoint(path, params = {}) {
  const url = new URL(path, els.apiUrl.value);
  Object.entries(params).forEach(([key, value]) => {
    if (value !== "" && value !== undefined && value !== null) {
      url.searchParams.set(key, value);
    }
  });
  return url;
}

async function getJSON(path, params) {
  const response = await fetch(endpoint(path, params));
  if (!response.ok) throw new Error(`${response.status} ${response.statusText}`);
  return response.json();
}

async function sendJSON(path, body, method = "POST") {
  const response = await fetch(endpoint(path), {
    method,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!response.ok) throw new Error(`${response.status} ${response.statusText}`);
  return response.json();
}

async function loadDashboards() {
  try {
    const [dashboards, config] = await Promise.all([getJSON("/v1/dashboards"), getJSON("/v1/config")]);
    state.dashboards = dashboards.map(normalizeDashboardDefinition);
    els.retentionDays.value = config.retentionDays || 7;
    if (state.dashboards.length === 0) state.dashboards = [newDashboardDefinition()];
    const previous = state.dashboard?.id || state.dashboards[0].id;
    state.dashboard = state.dashboards.find((item) => item.id === previous) || state.dashboards[0];
    renderDashboardSelect();
    syncDashboardEditor();
    await renderDashboard();
    setStatus(true);
  } catch (error) {
    setStatus(false, error.message);
  }
}

function normalizeDashboardDefinition(dashboard) {
  if (dashboard?.id !== "routing-slip-observability") return dashboard;
  const hasLegacyInstallmentWidget = (dashboard.widgets || []).some((widget) => String(widget.query || widget.metric || "").includes("installments."));
  return {
    ...dashboard,
    name: "Processamento",
    description: "Visao realtime do processamento de pedidos",
    widgets: hasLegacyInstallmentWidget ? newDashboardDefinition().widgets : dashboard.widgets,
  };
}

async function saveConfig() {
  try {
    await sendJSON("/v1/config", { retentionDays: Number(els.retentionDays.value) }, "PUT");
    setStatus(true);
  } catch (error) {
    setStatus(false, error.message);
  }
}

async function loadProcesses() {
  try {
    const events = await getJSON("/v1/metrics/events", {
      ...timeWindow(),
      source: "routing-slip-app",
      limit: 1000,
    });
    state.processGroups = groupProcessEvents(events);
    renderProcesses();
  } catch (error) {
    els.processSummary.innerHTML = `<span class="muted">${escapeHTML(error.message)}</span>`;
    els.processList.innerHTML = "";
  }
}

function groupProcessEvents(records) {
  const groups = new Map();
  records.forEach((record) => {
    const event = record.event || record;
    const tags = event.tags || {};
    const key = tags.correlation_id || tags.message_id || record.id || `${event.workflow}-${event.timestamp}`;
    if (!groups.has(key)) {
      groups.set(key, {
        id: key,
        workflow: event.workflow || "-",
        messageId: tags.message_id || "-",
        correlationId: tags.correlation_id || "-",
        tags: {},
        events: [],
        completed: 0,
        failed: 0,
        stopped: 0,
        totalSteps: Number(tags.total_steps || 0),
        startedAt: event.timestamp,
        updatedAt: event.timestamp,
      });
    }
    const group = groups.get(key);
    group.events.push(event);
    group.workflow = event.workflow || group.workflow;
    group.messageId = tags.message_id || group.messageId;
    group.correlationId = tags.correlation_id || group.correlationId;
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
      group.status = group.failed > 0 ? "failed" : group.stopped > 0 ? "stopped" : group.totalSteps > 0 && group.completed >= group.totalSteps ? "completed" : "running";
      group.remaining = Math.max((group.totalSteps || expectedSteps(group.workflow)) - group.completed - group.failed - group.stopped, 0);
      group.events.sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp));
      return group;
    })
    .sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt));
}

function renderProcesses() {
  const filtered = filterProcesses(state.processGroups, els.processFilter.value.trim());
  const completed = filtered.filter((item) => item.status === "completed").length;
  const failed = filtered.filter((item) => item.status === "failed").length;
  const stopped = filtered.filter((item) => item.status === "stopped").length;
  const pending = filtered.reduce((sum, item) => sum + item.remaining, 0);
  els.processSummary.innerHTML = `
    <div><strong>${filtered.length}</strong><span>processos</span></div>
    <div><strong>${completed}</strong><span>concluídos</span></div>
    <div><strong>${failed}</strong><span>falhas</span></div>
    <div><strong>${stopped}</strong><span>parados</span></div>
    <div><strong>${pending}</strong><span>etapas faltantes</span></div>
  `;
  if (filtered.length === 0) {
    els.processList.innerHTML = `<span class="muted">Nenhum processo encontrado no período.</span>`;
    return;
  }
  els.processList.innerHTML = filtered.map(processRow).join("");
  els.processList.querySelectorAll("[data-process-id]").forEach((row) => {
    row.addEventListener("click", () => openProcess(row.dataset.processId));
  });
}

function processRow(group) {
  const tags = group.tags || {};
  const chips = ["pedido_id", "id_cliente", "pagamento_id", "nota_fiscal_id", "codigo_rastreio"]
    .filter((key) => tags[key])
    .map((key) => `<span>${escapeHTML(key)}:${escapeHTML(tags[key])}</span>`)
    .join("");
  return `
    <button class="process-row" type="button" data-process-id="${escapeHTML(group.id)}">
      <div>
        <strong>${escapeHTML(group.workflow)}</strong>
        <div class="muted">${escapeHTML(group.correlationId)} · ${escapeHTML(group.messageId)}</div>
        <div class="process-tags">${chips}</div>
      </div>
      <div class="process-progress">
        <span class="status ${group.status}">${statusLabel(group.status)}</span>
        <strong>${group.completed}/${group.totalSteps || expectedSteps(group.workflow)}</strong>
        <span class="muted">${group.remaining} faltantes</span>
      </div>
      <time>${formatTime(group.updatedAt)}</time>
    </button>
  `;
}

function filterProcesses(processes, rawFilter) {
  if (!rawFilter) return processes;
  const parts = rawFilter.split(/\s+/).map(parseAttributeFilter).filter(Boolean);
  if (parts.length === 0) return processes;
  return processes.filter((process) =>
    parts.every(({ key, value }) => {
      const haystack = processSearchFields(process);
      return String(haystack[key] || "").toLowerCase().includes(value.toLowerCase());
    }),
  );
}

function parseAttributeFilter(text) {
  const [key, ...rest] = text.split(":");
  const value = rest.join(":");
  if (!key || !value) return null;
  return { key: key.trim(), value: value.trim() };
}

function processSearchFields(process) {
  return {
    workflow: process.workflow,
    message_id: process.messageId,
    correlation_id: process.correlationId,
    status: process.status,
    ...process.tags,
  };
}

function openProcess(id) {
  const process = state.processGroups.find((item) => item.id === id);
  if (!process) return;
  const tags = process.tags || {};
  els.modalTitle.textContent = process.workflow;
  els.modalCopy.textContent = `${process.correlationId} · ${statusLabel(process.status)} · ${process.remaining} etapas faltantes`;
  els.modalBody.innerHTML = `
    <section class="modal-kpis">
      <div><strong>${process.completed}</strong><span>concluídas</span></div>
      <div><strong>${process.failed}</strong><span>falhas</span></div>
      <div><strong>${process.stopped}</strong><span>paradas</span></div>
      <div><strong>${process.remaining}</strong><span>faltantes</span></div>
    </section>
    <section class="modal-tags">
      ${Object.entries(tags)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([key, value]) => `<span><strong>${escapeHTML(key)}</strong>${escapeHTML(value)}</span>`)
        .join("")}
    </section>
    <section class="timeline">
      ${processSteps(process).map((step, index) => timelineItem(step, index)).join("")}
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
        step: event.step || tags.handler || event.name,
        index: Number(tags.step_index || 0),
        startedAt: event.timestamp,
        updatedAt: event.timestamp,
        status: "running",
        input: tags.input_value || "",
        rule: tags.rule_applied || "",
        output: tags.output_value || "",
        failure: tags.failure_reason || "",
        duration: "",
      });
    }
    const step = groups.get(key);
    step.startedAt = earlier(step.startedAt, event.timestamp);
    step.updatedAt = later(step.updatedAt, event.timestamp);
    if (tags.input_value) step.input = tags.input_value;
    if (tags.rule_applied) step.rule = tags.rule_applied;
    if (tags.output_value) step.output = tags.output_value;
    if (tags.failure_reason) step.failure = tags.failure_reason;
    if (tags.duration_ms) step.duration = `${tags.duration_ms} ms`;
    if (["success", "failed", "stopped"].includes(event.status)) step.status = event.status;
  });
  return [...groups.values()].sort((a, b) => a.index - b.index || new Date(a.startedAt) - new Date(b.startedAt));
}

function timelineItem(step, index) {
  return `
    <article class="timeline-item ${step.status || ""}" data-step-index="${index}">
      <time>${formatTime(step.startedAt)}</time>
      <div>
        <strong>${escapeHTML(step.step)}</strong>
        <p>${escapeHTML(statusLabel(step.status))} · ${escapeHTML(step.duration || "-")}</p>
        <dl class="step-details">
          <div><dt>Valor de entrada</dt><dd>${formatDetail(step.input)}</dd></div>
          <div><dt>Regra aplicada</dt><dd>${formatDetail(step.rule)}</dd></div>
          <div><dt>Valor de saída</dt><dd>${formatDetail(step.output)}</dd></div>
          <div><dt>Status</dt><dd>${escapeHTML(statusLabel(step.status))}</dd></div>
          <div><dt>Motivo da falha/parada</dt><dd>${escapeHTML(step.failure || "-")}</dd></div>
        </dl>
      </div>
      <code>#${escapeHTML(step.index)}</code>
    </article>
  `;
}

function expectedSteps(workflow) {
  if (workflow === "payment-event-fulfillment") return 8;
  if (workflow === "order-fail-and-resume") return 7;
  return 6;
}

function statusLabel(status) {
  return { completed: "Concluído", success: "Concluído", failed: "Falhou", stopped: "Parado", running: "Em execução" }[status] || status;
}

function earlier(a, b) {
  return new Date(a) <= new Date(b) ? a : b;
}

function later(a, b) {
  return new Date(a) >= new Date(b) ? a : b;
}

function formatDetail(value) {
  if (!value) return "-";
  return `<code>${escapeHTML(prettyJSON(value))}</code>`;
}

function prettyJSON(value) {
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch {
    return value;
  }
}

function renderDashboardSelect() {
  els.dashboardSelect.innerHTML = "";
  state.dashboards.forEach((dashboard) => {
    const option = document.createElement("option");
    option.value = dashboard.id;
    option.textContent = dashboard.name;
    els.dashboardSelect.append(option);
  });
  els.dashboardSelect.value = state.dashboard?.id || "";
}

function syncDashboardEditor() {
  if (!state.dashboard) return;
  els.dashboardName.value = state.dashboard.name || "";
  els.dashboardDescription.value = state.dashboard.description || "";
  els.dashboardJSON.value = JSON.stringify(state.dashboard, null, 2);
  els.dashboardTitle.textContent = state.dashboard.name || "Dashboard";
  els.dashboardCopy.textContent = state.dashboard.description || "Dashboard parametrizado em JSON.";
  syncWidgetForm();
}

async function renderDashboard() {
  if (!state.dashboard) return;
  els.grid.innerHTML = "";
  const widgets = [...(state.dashboard.widgets || [])].sort((a, b) => (a.layout?.y || 0) - (b.layout?.y || 0) || (a.layout?.x || 0) - (b.layout?.x || 0));
  await Promise.all([Promise.all(widgets.map(renderWidget)), loadProcesses()]);
  els.lastUpdate.textContent = `Atualizado ${formatTime(new Date().toISOString())}`;
}

async function renderWidget(widget) {
  const card = document.createElement("article");
  card.className = `widget ${widget.id === state.selectedWidgetId ? "selected" : ""}`;
  const layout = normalizeLayout(widget.layout);
  card.style.gridColumn = `${layout.x + 1} / span ${layout.w}`;
  card.style.gridRow = `${layout.y + 1} / span ${layout.h}`;
  card.dataset.id = widget.id;
  card.innerHTML = `
    <div class="widget-header">
      <div class="widget-title">
        <strong>${escapeHTML(widget.title || "Widget")}</strong>
        <small>${escapeHTML(widget.query || "")}</small>
      </div>
      <div class="widget-actions">
        <button data-action="left" title="Mover esquerda" type="button">&lt;</button>
        <button data-action="right" title="Mover direita" type="button">&gt;</button>
        <button data-action="up" title="Mover acima" type="button">^</button>
        <button data-action="down" title="Mover abaixo" type="button">v</button>
      </div>
    </div>
    <div class="widget-body"><span class="muted">Carregando</span></div>
  `;
  card.addEventListener("click", (event) => {
    const action = event.target?.dataset?.action;
    if (action) {
      moveWidget(widget.id, action);
      return;
    }
    selectWidget(widget.id);
  });
  els.grid.append(card);

  try {
    const result = await runWidgetQuery(widget);
    drawWidget(card.querySelector(".widget-body"), widget, result);
  } catch (error) {
    card.querySelector(".widget-body").innerHTML = `<span class="muted">${escapeHTML(error.message)}</span>`;
  }
}

async function runWidgetQuery(widget) {
  const query = parseQuery(widget.query || fallbackQuery(widget));
  const params = {
    ...timeWindow(),
    name: query.metric,
    groupBy: query.groupBy || widget.groupBy || "",
  };
  Object.entries(query.tags).forEach(([key, value]) => {
    params[`tag.${key}`] = value;
  });
  Object.entries(query.tagIn).forEach(([key, values]) => {
    params[`tagIn.${key}`] = values.join(",");
  });
  Object.entries(query.dimensions).forEach(([key, value]) => {
    params[key] = value;
  });

  if ((widget.type || widget.chart) === "timeseries" && !params.groupBy) {
    return { query, series: await getJSON("/v1/metrics/series", { ...params, bucket: "1m" }) };
  }
  return { query, summaries: await getJSON("/v1/metrics", params) };
}

function parseQuery(raw) {
  const text = raw.trim();
  const match = text.match(/^(\w+):([A-Za-z0-9_.-]+)\{([^}]*)\}(?:\s+by\s+\{([^}]+)\})?(?:\.(\w+)\(\))?$/);
  if (!match) throw new Error("Query invalida");

  const [, aggregation, metric, filterText, groupByRaw, rollup] = match;
  const query = { aggregation, metric, rollup: rollup || "", groupBy: normalizeGroupBy(groupByRaw || ""), tags: {}, tagIn: {}, dimensions: {} };
  splitConditions(filterText).forEach((condition) => {
    const inMatch = condition.match(/^([A-Za-z0-9_.-]+)\s+in\s*\(([^)]+)\)$/i);
    if (inMatch) {
      query.tagIn[inMatch[1]] = inMatch[2].split(",").map((item) => item.trim()).filter(Boolean);
      return;
    }
    const eqMatch = condition.match(/^([A-Za-z0-9_.-]+)\s*:\s*([A-Za-z0-9_.-]+)$/);
    if (!eqMatch) return;
    const key = eqMatch[1];
    const value = eqMatch[2];
    if (["segment", "workflow", "step", "status", "source"].includes(key)) {
      query.dimensions[key] = value;
    } else {
      query.tags[key] = value;
    }
  });
  return query;
}

function splitConditions(text) {
  return text
    .split(/\s+(?:and|or)\s+/i)
    .map((item) => item.trim())
    .filter(Boolean);
}

function normalizeGroupBy(value) {
  const clean = value.trim();
  if (!clean) return "";
  if (["segment", "workflow", "step", "status", "source"].includes(clean)) return clean;
  if (clean.startsWith("tag:")) return clean;
  return `tag:${clean}`;
}

function drawWidget(body, widget, result) {
  const type = widget.type || widget.chart || "timeseries";
  if (type === "indicator") return drawIndicator(body, widget, result.summaries || []);
  if (type === "table") return drawTable(body, result.summaries || []);
  if (type === "list") return drawList(body, result.summaries || []);
  if (type === "bar") return drawBar(body, result.summaries || []);
  return drawLine(body, result.series || []);
}

function drawIndicator(body, widget, summaries) {
  const summary = summaries[0];
  const value = summary ? formatValue(summary.sum, widget.display?.unit || summary.unit) : "0";
  body.innerHTML = `<div class="indicator-value">${value}</div><span class="muted">${summary ? `${summary.count} eventos` : "sem dados"}</span>`;
}

function drawTable(body, summaries) {
  body.innerHTML = `
    <table>
      <thead><tr><th>Grupo</th><th>Total</th><th>Eventos</th></tr></thead>
      <tbody>${summaries
        .map((item) => `<tr><td>${escapeHTML(item.group || item.name)}</td><td>${formatValue(item.sum, item.unit)}</td><td>${item.count}</td></tr>`)
        .join("")}</tbody>
    </table>
  `;
}

function drawList(body, summaries) {
  body.className = "widget-body list";
  body.innerHTML = summaries
    .map((item) => `<div class="list-row"><span>${escapeHTML(item.group || item.name)}</span><strong>${formatValue(item.sum, item.unit)}</strong></div>`)
    .join("") || `<span class="muted">Sem dados</span>`;
}

function drawLine(body, series) {
  body.innerHTML = `<canvas></canvas>`;
  const canvas = body.querySelector("canvas");
  const context = prepareCanvas(canvas);
  const rect = canvas.getBoundingClientRect();
  const pad = 24;
  const width = rect.width - pad * 2;
  const height = rect.height - pad * 2;
  axis(context, pad, width, height);
  if (series.length === 0) return emptyCanvas(context, pad, "Sem dados para a query");
  const max = Math.max(...series.map((point) => point.value), 1);
  context.strokeStyle = "#246bfe";
  context.lineWidth = 3;
  context.beginPath();
  series.forEach((point, index) => {
    const x = pad + (series.length === 1 ? width : (index / (series.length - 1)) * width);
    const y = pad + height - (point.value / max) * height;
    index === 0 ? context.moveTo(x, y) : context.lineTo(x, y);
  });
  context.stroke();
}

function drawBar(body, summaries) {
  body.innerHTML = `<canvas></canvas>`;
  const canvas = body.querySelector("canvas");
  const context = prepareCanvas(canvas);
  const rect = canvas.getBoundingClientRect();
  const pad = 24;
  const width = rect.width - pad * 2;
  const height = rect.height - pad * 2;
  axis(context, pad, width, height);
  if (summaries.length === 0) return emptyCanvas(context, pad, "Sem dados para a query");
  const max = Math.max(...summaries.map((item) => item.sum), 1);
  const gap = 8;
  const barWidth = Math.max(18, (width - gap * (summaries.length - 1)) / summaries.length);
  summaries.forEach((item, index) => {
    const x = pad + index * (barWidth + gap);
    const barHeight = (item.sum / max) * height;
    const y = pad + height - barHeight;
    context.fillStyle = index % 2 ? "#158f72" : "#246bfe";
    context.fillRect(x, y, Math.min(barWidth, 72), barHeight);
    context.fillStyle = "#637074";
    context.fillText(truncate(item.group || item.name, 12), x, pad + height + 16);
  });
}

function prepareCanvas(canvas) {
  const ratio = window.devicePixelRatio || 1;
  const rect = canvas.getBoundingClientRect();
  canvas.width = Math.floor(rect.width * ratio);
  canvas.height = Math.floor(rect.height * ratio);
  const context = canvas.getContext("2d");
  context.scale(ratio, ratio);
  context.clearRect(0, 0, rect.width, rect.height);
  return context;
}

function axis(context, pad, width, height) {
  context.strokeStyle = "#d8e0e3";
  context.lineWidth = 1;
  context.beginPath();
  context.moveTo(pad, pad);
  context.lineTo(pad, pad + height);
  context.lineTo(pad + width, pad + height);
  context.stroke();
}

function emptyCanvas(context, pad, text) {
  context.fillStyle = "#637074";
  context.fillText(text, pad + 10, pad + 24);
}

function selectWidget(id) {
  state.selectedWidgetId = id;
  syncWidgetForm();
  renderDashboard();
}

function syncWidgetForm() {
  const widget = selectedWidget();
  if (!widget) return;
  const layout = normalizeLayout(widget.layout);
  els.widgetType.value = widget.type || "timeseries";
  els.widgetTitle.value = widget.title || "";
  els.widgetQuery.value = widget.query || fallbackQuery(widget);
  els.widgetX.value = layout.x;
  els.widgetY.value = layout.y;
  els.widgetW.value = layout.w;
  els.widgetH.value = layout.h;
}

function selectedWidget() {
  return state.dashboard?.widgets?.find((widget) => widget.id === state.selectedWidgetId) || state.dashboard?.widgets?.[0];
}

function updateSelectedWidget() {
  const widget = selectedWidget();
  if (!widget) return;
  widget.type = els.widgetType.value;
  widget.chart = els.widgetType.value;
  widget.title = els.widgetTitle.value;
  widget.query = els.widgetQuery.value;
  widget.layout = {
    x: Number(els.widgetX.value),
    y: Number(els.widgetY.value),
    w: Number(els.widgetW.value),
    h: Number(els.widgetH.value),
  };
  syncDashboardJSONFromForm();
}

function syncDashboardJSONFromForm() {
  state.dashboard.name = els.dashboardName.value;
  state.dashboard.description = els.dashboardDescription.value;
  els.dashboardJSON.value = JSON.stringify(state.dashboard, null, 2);
  syncDashboardEditor();
  renderDashboard();
}

function addWidget() {
  const y = Math.max(0, ...(state.dashboard.widgets || []).map((widget) => (widget.layout?.y || 0) + (widget.layout?.h || 3)));
  const widget = {
    id: crypto.randomUUID ? crypto.randomUUID() : `widget-${Date.now()}`,
    type: "timeseries",
    title: "Novo widget",
    query: "sum:routing_slip.step.completed{source:routing-slip-app}.as_count()",
    aggregation: "sum",
    chart: "timeseries",
    layout: { x: 0, y, w: 6, h: 3 },
    display: { legend: true },
  };
  state.dashboard.widgets = [...(state.dashboard.widgets || []), widget];
  state.selectedWidgetId = widget.id;
  syncDashboardJSONFromForm();
}

function duplicateWidget() {
  const widget = selectedWidget();
  if (!widget) return;
  const copy = JSON.parse(JSON.stringify(widget));
  copy.id = crypto.randomUUID ? crypto.randomUUID() : `widget-${Date.now()}`;
  copy.title = `${copy.title} copia`;
  copy.layout.y = (copy.layout.y || 0) + 1;
  state.dashboard.widgets.push(copy);
  state.selectedWidgetId = copy.id;
  syncDashboardJSONFromForm();
}

function removeWidget() {
  const widget = selectedWidget();
  if (!widget) return;
  state.dashboard.widgets = state.dashboard.widgets.filter((item) => item.id !== widget.id);
  state.selectedWidgetId = state.dashboard.widgets[0]?.id || "";
  syncDashboardJSONFromForm();
}

function moveWidget(id, action) {
  const widget = state.dashboard.widgets.find((item) => item.id === id);
  if (!widget) return;
  widget.layout = normalizeLayout(widget.layout);
  if (action === "left") widget.layout.x = Math.max(0, widget.layout.x - 1);
  if (action === "right") widget.layout.x = Math.min(11, widget.layout.x + 1);
  if (action === "up") widget.layout.y = Math.max(0, widget.layout.y - 1);
  if (action === "down") widget.layout.y += 1;
  syncDashboardJSONFromForm();
}

function applyJSON() {
  try {
    state.dashboard = JSON.parse(els.dashboardJSON.value);
    state.selectedWidgetId = state.dashboard.widgets?.[0]?.id || "";
    syncDashboardEditor();
    renderDashboard();
    setStatus(true);
  } catch (error) {
    setStatus(false, error.message);
  }
}

async function saveDashboard() {
  try {
    updateSelectedWidget();
    state.dashboard = await sendJSON("/v1/dashboards", state.dashboard);
    await loadDashboards();
    setStatus(true);
  } catch (error) {
    setStatus(false, error.message);
  }
}

async function deleteDashboard() {
  if (!state.dashboard?.id) return;
  try {
    await fetch(endpoint(`/v1/dashboards/${state.dashboard.id}`), { method: "DELETE" });
    state.dashboard = null;
    await loadDashboards();
  } catch (error) {
    setStatus(false, error.message);
  }
}

function newDashboardDefinition() {
  return {
    id: "routing-slip-observability",
    schemaVersion: 1,
    name: "Processamento",
    description: "Visao realtime do processamento de pedidos",
    refreshSeconds: 5,
    variables: [],
    widgets: [
      {
        id: "routing-slip-completed",
        type: "indicator",
        title: "Etapas concluidas",
        query: "sum:routing_slip.step.completed{source:routing-slip-app}.as_count()",
        aggregation: "sum",
        chart: "indicator",
        layout: { x: 0, y: 0, w: 3, h: 2 },
        display: { label: "eventos" },
      },
      {
        id: "routing-slip-failed",
        type: "indicator",
        title: "Falhas tecnicas",
        query: "sum:routing_slip.step.failed{source:routing-slip-app}.as_count()",
        aggregation: "sum",
        chart: "indicator",
        layout: { x: 3, y: 0, w: 3, h: 2 },
        display: { label: "eventos" },
      },
      {
        id: "routing-slip-stopped",
        type: "indicator",
        title: "Paradas funcionais",
        query: "sum:routing_slip.step.stopped{source:routing-slip-app}.as_count()",
        aggregation: "sum",
        chart: "indicator",
        layout: { x: 6, y: 0, w: 3, h: 2 },
        display: { label: "eventos" },
      },
      {
        id: "routing-slip-duration",
        type: "bar",
        title: "Duracao por etapa",
        query: "sum:routing_slip.step.duration_ms{source:routing-slip-app} by {step}",
        aggregation: "sum",
        chart: "bar",
        layout: { x: 9, y: 0, w: 3, h: 2 },
        display: { unit: "ms" },
      },
      {
        id: "routing-slip-throughput",
        type: "timeseries",
        title: "Throughput em tempo real",
        query: "sum:routing_slip.step.completed{source:routing-slip-app}.as_count()",
        aggregation: "sum",
        chart: "timeseries",
        layout: { x: 0, y: 2, w: 8, h: 3 },
        display: { legend: true },
      },
      {
        id: "routing-slip-steps-table",
        type: "table",
        title: "Execucoes por etapa",
        query: "sum:routing_slip.step.completed{source:routing-slip-app} by {step}",
        aggregation: "sum",
        chart: "table",
        layout: { x: 8, y: 2, w: 4, h: 3 },
        display: { label: "etapas" },
      },
    ],
  };
}

function fallbackQuery(widget) {
  return `${widget.aggregation || "sum"}:${widget.metric || "routing_slip.step.completed"}{source:routing-slip-app}.as_count()`;
}

function normalizeLayout(layout = {}) {
  return { x: layout.x || 0, y: layout.y || 0, w: layout.w || 6, h: layout.h || 3 };
}

function timeWindow() {
  const to = new Date();
  const from = new Date(to.getTime() - Number(els.window.value) * 1000);
  return { from: from.toISOString(), to: to.toISOString() };
}

function formatValue(value, unit) {
  if (unit === "BRL") return new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(value);
  return new Intl.NumberFormat("pt-BR", { maximumFractionDigits: 2 }).format(value || 0);
}

function formatTime(value) {
  return new Intl.DateTimeFormat("pt-BR", { hour: "2-digit", minute: "2-digit", second: "2-digit" }).format(new Date(value));
}

function truncate(value, length) {
  return value.length > length ? `${value.slice(0, length - 1)}...` : value;
}

function escapeHTML(value) {
  return String(value).replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#039;" })[char]);
}

function setStatus(ok, detail = "") {
  els.dot.className = `dot ${ok ? "ok" : "fail"}`;
  els.statusText.textContent = ok ? "Online" : `Offline ${detail}`;
}

function schedule() {
  clearInterval(state.timer);
  state.timer = setInterval(renderDashboard, Number(els.refresh.value));
}

els.dashboardSelect.addEventListener("change", () => {
  state.dashboard = state.dashboards.find((dashboard) => dashboard.id === els.dashboardSelect.value);
  state.selectedWidgetId = state.dashboard?.widgets?.[0]?.id || "";
  syncDashboardEditor();
  renderDashboard();
});
els.toggleEdit.addEventListener("click", () => {
  state.editing = !state.editing;
  document.body.classList.toggle("editing", state.editing);
  els.toggleEdit.textContent = state.editing ? "Visualizar" : "Editar";
});
els.saveDashboard.addEventListener("click", saveDashboard);
els.saveConfig.addEventListener("click", saveConfig);
els.newDashboard.addEventListener("click", () => {
  state.dashboard = newDashboardDefinition();
  state.selectedWidgetId = state.dashboard.widgets[0].id;
  syncDashboardEditor();
  renderDashboard();
});
els.deleteDashboard.addEventListener("click", deleteDashboard);
els.refreshNow.addEventListener("click", renderDashboard);
els.applyJSON.addEventListener("click", applyJSON);
els.addWidget.addEventListener("click", addWidget);
els.updateWidget.addEventListener("click", updateSelectedWidget);
els.duplicateWidget.addEventListener("click", duplicateWidget);
els.removeWidget.addEventListener("click", removeWidget);
els.processSearch.addEventListener("click", renderProcesses);
els.processFilter.addEventListener("keydown", (event) => {
  if (event.key === "Enter") renderProcesses();
});
els.modalClose.addEventListener("click", () => els.processModal.close());
els.processModal.addEventListener("click", (event) => {
  if (event.target === els.processModal) els.processModal.close();
});
els.refresh.addEventListener("change", schedule);
els.window.addEventListener("change", renderDashboard);
els.apiUrl.addEventListener("change", loadDashboards);
window.addEventListener("resize", renderDashboard);

schedule();
loadDashboards();
