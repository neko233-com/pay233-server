const adminName = document.querySelector("#adminName");
const logoutBtn = document.querySelector("#logoutBtn");
const charts = {};

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

async function loadDashboard() {
  const data = await api("/admin/api/dashboard");
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
    tbody.innerHTML = `<tr><td colspan="8">暂无异常支付</td></tr>`;
    return;
  }
  tbody.innerHTML = rows.map((p) => `
    <tr>
      <td>${p.id}</td>
      <td>${p.out_trade_no}</td>
      <td>${p.channel}</td>
      <td>${money(p.amount && p.amount.amount)}</td>
      <td><span class="status ${statusClass(p.status)}">${p.status}</span></td>
      <td><span class="status ${statusClass(p.callback_status)}">${p.callback_status}</span></td>
      <td>${p.failure_reason || p.callback_error || "-"}</td>
      <td><button class="ghost" data-lost="${p.id}">标记丢单</button></td>
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

window.addEventListener("resize", () => Object.values(charts).forEach((c) => c.resize()));

(async function boot() {
  const session = await api("/admin/api/session");
  if (!session.authenticated) {
    window.location.replace("/admin/login.html");
    return;
  }
  adminName.textContent = session.username || "root";
  await loadDashboard();
})();
