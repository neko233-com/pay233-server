const adminName = document.querySelector("#adminName");
const adminRole = document.querySelector("#adminRole");
const logoutBtn = document.querySelector("#logoutBtn");
const envButtons = Array.from(document.querySelectorAll("[data-env]"));
const userPanel = document.querySelector("#userPanel");
const userForm = document.querySelector("#userForm");
const pruneAuditBtn = document.querySelector("#pruneAuditBtn");
const charts = {};
let currentEnv = new URLSearchParams(window.location.search).get("envType") || "test";
let currentRole = "employee";

async function api(path, options = {}) {
  const res = await fetch(path, {
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  if (res.status === 401) {
    window.location.replace("/admin/login.html");
    throw new Error("admin login required");
  }
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

function money(v) {
  return new Intl.NumberFormat("zh-CN").format(v || 0);
}

function pct(v) {
  return `${Math.round((v || 0) * 1000) / 10}%`;
}

function statusClass(value) {
  if (["paid", "success", "ok"].includes(value)) return "ok";
  if (["failed", "lost"].includes(value)) return "bad";
  return "warn";
}

function rowActions(p) {
  if (!["root", "admin"].includes(currentRole)) return "-";
  const retry = p.notify_url && p.callback_status !== "success"
    ? `<button class="ghost" data-retry="${p.id}">重试回调</button>`
    : "";
  return `<div class="row-actions">${retry}<button class="ghost" data-lost="${p.id}">标记丢单</button></div>`;
}

async function loadDashboard() {
  syncEnvButtons();
  const suffix = currentEnv === "all" ? "?envType=all" : `?envType=${encodeURIComponent(currentEnv)}`;
  const data = await api(`/admin/api/dashboard${suffix}`);
  document.querySelector("#kpiGmv").textContent = money(data.kpis.gmv);
  document.querySelector("#kpiSuccess").textContent = pct(data.kpis.success_rate);
  document.querySelector("#kpiCallback").textContent = data.kpis.callback_failures;
  document.querySelector("#kpiLost").textContent = data.kpis.lost_orders;
  document.querySelector("#kpiUnsettled").textContent = money(data.kpis.unsettled_amount);

  renderTrend(data.series || []);
  renderChannels(data.channels || []);
  renderFailures(data.failures || []);
  renderHealth(data.channel_info || []);
  renderAbnormal(data.abnormal || []);
  await loadAudit();
  if (currentRole === "root") await loadUsers();
}

function chart(id) {
  if (!charts[id]) charts[id] = echarts.init(document.getElementById(id));
  return charts[id];
}

function renderTrend(series) {
  chart("trendChart").setOption({
    tooltip: { trigger: "axis" },
    legend: { data: ["交易金额", "成功率"] },
    grid: { left: 42, right: 42, top: 42, bottom: 34 },
    xAxis: { type: "category", data: series.map((x) => x.date) },
    yAxis: [{ type: "value" }, { type: "value", max: 1, axisLabel: { formatter: (v) => `${v * 100}%` } }],
    series: [
      { name: "交易金额", type: "line", smooth: true, data: series.map((x) => x.amount), color: "#2364e8" },
      { name: "成功率", type: "line", smooth: true, yAxisIndex: 1, data: series.map((x) => x.success_rate), color: "#0e9f95" },
    ],
    graphic: emptyGraphic(series.length, "暂无交易数据"),
  });
}

function renderChannels(channels) {
  chart("channelChart").setOption({
    tooltip: { trigger: "item" },
    series: [{
      type: "pie",
      radius: ["46%", "72%"],
      data: channels.map((x) => ({ name: x.channel, value: x.amount })),
      color: ["#2364e8", "#0e9f95", "#7c5cff", "#f59f00", "#d14343", "#2f80ed"],
    }],
    graphic: emptyGraphic(channels.length, "暂无渠道交易"),
  });
}

function renderFailures(failures) {
  chart("failureChart").setOption({
    tooltip: { trigger: "axis" },
    grid: { left: 80, right: 16, top: 20, bottom: 28 },
    xAxis: { type: "value" },
    yAxis: { type: "category", data: failures.map((x) => x.reason) },
    series: [{ type: "bar", data: failures.map((x) => x.count), color: "#d14343" }],
    graphic: emptyGraphic(failures.length, "暂无失败记录"),
  });
}

function emptyGraphic(count, text) {
  if (count > 0) return [];
  return {
    type: "text",
    left: "center",
    top: "middle",
    style: { text, fill: "#697386", fontSize: 14, fontWeight: 600 },
  };
}

function renderHealth(channels) {
  const box = document.querySelector("#channelHealth");
  box.innerHTML = channels.map((c) => `
    <div class="channel-item">
      <div>
        <strong>${c.display_name || c.name}</strong>
        <small>${c.name} · ${(c.capabilities || []).slice(0, 3).join(" / ")}</small>
      </div>
      <span class="status ${statusClass(c.health)}">${c.health}</span>
    </div>
  `).join("");
}

function renderAbnormal(rows) {
  const tbody = document.querySelector("#abnormalRows");
  if (rows.length === 0) {
    tbody.innerHTML = `<tr><td colspan="9">暂无异常支付</td></tr>`;
    return;
  }
  tbody.innerHTML = rows.map((p) => `
    <tr>
      <td>${p.id}</td>
      <td><span class="status ${p.env_type === "release" ? "bad" : "ok"}">${p.env_type}</span></td>
      <td>${p.out_trade_no}</td>
      <td>${p.channel}</td>
      <td>${money(p.amount && p.amount.amount)}</td>
      <td><span class="status ${statusClass(p.status)}">${p.status}</span></td>
      <td><span class="status ${statusClass(p.callback_status)}">${p.callback_status}</span></td>
      <td>${p.failure_reason || p.callback_error || "-"}</td>
      <td>${rowActions(p)}</td>
    </tr>
  `).join("");
}

function syncEnvButtons() {
  envButtons.forEach((button) => {
    button.classList.toggle("active", button.dataset.env === currentEnv);
  });
}

async function loadUsers() {
  const data = await api("/admin/api/users");
  const tbody = document.querySelector("#userRows");
  const users = data.users || [];
  tbody.innerHTML = users.map((u) => `
    <tr>
      <td>${u.username}</td>
      <td><span class="status ${u.role === "root" ? "bad" : u.role === "admin" ? "warn" : "ok"}">${u.role}</span></td>
      <td>${u.created_by || "-"}</td>
      <td>${u.role === "root" ? "-" : `<button class="ghost" data-delete-user="${u.username}">删除</button>`}</td>
    </tr>
  `).join("");
}

async function loadAudit() {
  const data = await api("/admin/api/audit?limit=50");
  const tbody = document.querySelector("#auditRows");
  const entries = data.entries || [];
  if (entries.length === 0) {
    tbody.innerHTML = `<tr><td colspan="4">暂无操作日志</td></tr>`;
    return;
  }
  tbody.innerHTML = entries.map((entry) => `
    <tr>
      <td>${new Date(entry.created_at).toLocaleString("zh-CN")}</td>
      <td>${entry.actor}<br><small>${entry.role || "-"}</small></td>
      <td>${entry.action}</td>
      <td>${entry.target || "-"}</td>
    </tr>
  `).join("");
}

logoutBtn.addEventListener("click", async () => {
  await api("/admin/logout", { method: "POST", body: "{}" }).catch(() => {});
  window.location.replace("/admin/login.html");
});

document.addEventListener("click", async (event) => {
  const id = event.target && event.target.dataset && event.target.dataset.lost;
  if (!id) return;
  await api(`/admin/api/payments/${id}/mark-lost`, {
    method: "POST",
    body: JSON.stringify({ reason: "admin marked as lost order" }),
  });
  await loadDashboard();
});

document.addEventListener("click", async (event) => {
  const id = event.target && event.target.dataset && event.target.dataset.retry;
  if (!id) return;
  await api(`/admin/api/payments/${id}/retry-callback`, {
    method: "POST",
    body: "{}",
  });
  await loadDashboard();
});

document.addEventListener("click", async (event) => {
  const username = event.target && event.target.dataset && event.target.dataset.deleteUser;
  if (!username) return;
  await api(`/admin/api/users/${encodeURIComponent(username)}`, { method: "DELETE" });
  await loadUsers();
  await loadAudit();
});

userForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const form = new FormData(userForm);
  await api("/admin/api/users", {
    method: "POST",
    body: JSON.stringify({
      username: String(form.get("username") || "").trim(),
      password: String(form.get("password") || ""),
      role: String(form.get("role") || "employee"),
    }),
  });
  userForm.reset();
  await loadUsers();
  await loadAudit();
});

pruneAuditBtn.addEventListener("click", async () => {
  await api("/admin/api/audit/prune", { method: "POST", body: "{}" });
  await loadAudit();
});

envButtons.forEach((button) => {
  button.addEventListener("click", async () => {
    currentEnv = button.dataset.env;
    const url = new URL(window.location.href);
    url.searchParams.set("envType", currentEnv);
    window.history.replaceState({}, "", url);
    await loadDashboard();
  });
});

window.addEventListener("resize", () => Object.values(charts).forEach((c) => c.resize()));

(async function boot() {
  const session = await api("/admin/api/session");
  if (!session.authenticated) {
    window.location.replace("/admin/login.html");
    return;
  }
  currentRole = session.role || "employee";
  adminName.textContent = session.username || "root";
  adminRole.textContent = currentRole;
  userPanel.classList.toggle("hidden", currentRole !== "root");
  pruneAuditBtn.classList.toggle("hidden", currentRole !== "root");
  await loadDashboard();
})();
