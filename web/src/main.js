// Operator frontend: drives the full pressure-housing qualification flow through
// the real HTTP API — configuration freeze, trial creation, atomic startup,
// sampling, step completion, review, and admission credential issuance.

const state = { digest: null, trialId: null, logicalMs: 1000, stage: "precheck" };

const sampleConfig = {
  chambers: [
    { id: "c-main", name: "主承压舱段", volume_ul: 1000 },
    { id: "c-end", name: "端盖舱段", volume_ul: 500 },
  ],
  ports: [
    { id: "p-inlet", chamber: "c-main", kind: "pressure_inlet", channel: "" },
    { id: "p-sensor", chamber: "c-main", kind: "pressure_sensor", channel: "ch-1" },
    { id: "p-temp", chamber: "c-end", kind: "temperature_sensor", channel: "ch-2" },
  ],
  pipes: [{ id: "pipe-1", from: "p-sensor", to: "p-temp" }],
  seal_boundaries: [{ id: "s-1", chamber: "c-main", checks: ["外观检查", "密封复查"] }],
  steps: [
    { index: 1, target_pa: 5000000, ramp_up_pa_per_s: 100000, ramp_down_pa_per_s: 100000, hold_ms: 600000, leak_limit_ul_per_s: 10, max_drop_pa: 50000 },
    { index: 2, target_pa: 10000000, ramp_up_pa_per_s: 100000, ramp_down_pa_per_s: 100000, hold_ms: 600000, leak_limit_ul_per_s: 10, max_drop_pa: 50000 },
  ],
  calibrations: [
    { channel: "ch-1", serial: "SN-P-001", expires_at_ms: 2000000000000, summary: "压力传感器校准" },
    { channel: "ch-2", serial: "SN-T-001", expires_at_ms: 2000000000000, summary: "温度传感器校准" },
  ],
  compensation: { ref_temp_mc: 20000, temp_coeff_ppm: 10 },
};

function setText(id, text) {
  const el = document.getElementById(id);
  if (el) el.textContent = text;
}

function log(text) {
  const el = document.getElementById("log");
  if (el) el.textContent = text + "\n" + (el.textContent || "");
}

async function api(path, opts) {
  const res = await fetch(path, opts);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(JSON.stringify(data));
  return data;
}

function json(method, body) {
  return { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) };
}

async function refreshStatus() {
  try {
    const d = await api("/health");
    setText("status", "在线: " + JSON.stringify(d));
  } catch (err) {
    setText("status", "后端不可达: " + err);
  }
}

async function freeze() {
  try {
    const snap = await api("/api/v1/configurations/freeze", json("POST", sampleConfig));
    state.digest = snap.digest;
    log("构型冻结摘要: " + snap.digest);
  } catch (err) { log("冻结失败: " + err); }
}

async function createTrial() {
  if (!state.digest) return log("请先冻结构型");
  try {
    const tr = await api("/api/v1/trials", json("POST", { config_digest: state.digest }));
    state.trialId = tr.id;
    log("试验已创建: " + tr.id);
  } catch (err) { log("创建试验失败: " + err); }
}

async function startup() {
  if (!state.trialId) return log("请先创建试验");
  try {
    await api("/api/v1/trials/" + state.trialId + "/startup", json("POST", {
      bindings: [
        { serial: "SN-P-001", type: "pressure_sensor", position: "p-sensor" },
        { serial: "SN-T-001", type: "temperature_sensor", position: "p-temp" },
      ],
      leases: [
        { resource_id: "chamber-1" }, { resource_id: "pump-1" },
        { resource_id: "collector-1" }, { resource_id: "valve-1" },
      ],
    }));
    log("原子开试完成：部件绑定与资源租约已提交");
  } catch (err) { log("开试失败: " + err); }
}

async function submitSample() {
  if (!state.trialId) return log("请先创建试验");
  try {
    await api("/api/v1/trials/" + state.trialId + "/samples", json("POST", {
      logical_ms: state.logicalMs, pressure_pa: 5000000, temp_mc: 20000,
    }));
    log("样点已提交 @ " + state.logicalMs);
    state.logicalMs += 1;
  } catch (err) { log("样点提交失败: " + err); }
}

async function completeStep() {
  if (!state.trialId) return log("请先创建试验");
  try {
    const tr = await api("/api/v1/trials/" + state.trialId + "/steps/1/complete", json("POST", {
      start_ms: state.logicalMs - 5, end_ms: state.logicalMs + 1,
    }));
    setText("trial", JSON.stringify(tr, null, 2));
    log("阶梯1已通过证据判定");
  } catch (err) { log("完成阶梯失败: " + err); }
}

async function advanceStage() {
  if (!state.trialId) return log("请先创建试验");
  const order = ["precheck", "fill_vent", "step_ramp", "hold", "controlled_vent", "repressurize", "visual_check", "seal_check", "review", "admission"];
  const idx = order.indexOf(state.stage);
  const next = order[idx + 1];
  if (!next) return log("已到达最终阶段");
  try {
    const tr = await api("/api/v1/trials/" + state.trialId + "/stages/" + next, json("POST", {}));
    state.stage = next;
    setText("trial", JSON.stringify(tr, null, 2));
    log("阶段推进到: " + next);
  } catch (err) { log("推进阶段失败: " + err); }
}

async function submitReview() {
  if (!state.trialId) return log("请先创建试验");
  try {
    await api("/api/v1/trials/" + state.trialId + "/reviews", json("POST", {
      operator: "alice", qualification: "高级检验员", qual_expires_at: 2000000000000,
    }));
    await api("/api/v1/trials/" + state.trialId + "/reviews", json("POST", {
      operator: "bob", qualification: "高级检验员", qual_expires_at: 2000000000000,
    }));
    log("双人独立复核已提交");
  } catch (err) { log("复核提交失败: " + err); }
}

async function finalize() {
  if (!state.trialId) return log("请先创建试验");
  try {
    const cred = await api("/api/v1/trials/" + state.trialId + "/finalize", json("POST", {}));
    log("航次准入凭据: " + cred.digest);
  } catch (err) { log("准入签发失败: " + err); }
}

function bind(id, fn) { document.getElementById(id).addEventListener("click", fn); }

bind("freeze", freeze);
bind("create-trial", createTrial);
bind("startup", startup);
bind("sample", submitSample);
bind("complete-step", completeStep);
bind("advance-stage", advanceStage);
bind("review", submitReview);
bind("finalize", finalize);
refreshStatus();
