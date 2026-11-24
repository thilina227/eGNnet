"use strict";
// eGNnet single-page app (vanilla JS, no build step).

const state = {
  lang: "en",
  i18n: {},
  lookups: {},
  me: null,
  myDivisions: [],
  view: { name: "dashboard" },
};

const root = document.getElementById("root");

// ---------- helpers ----------
function t(k) { return state.i18n[k] || k; }
function esc(s) { return (s == null ? "" : String(s)).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c])); }

async function api(method, path, body) {
  const opt = { method, headers: {} };
  if (body !== undefined) { opt.headers["Content-Type"] = "application/json"; opt.body = JSON.stringify(body); }
  const res = await fetch(path, opt);
  let data = null;
  try { data = await res.json(); } catch (e) {}
  if (!res.ok) throw Object.assign(new Error((data && data.error) || res.statusText), { status: res.status, data });
  return data;
}

function toast(msg) {
  const el = document.getElementById("toast");
  el.textContent = msg; el.classList.add("show");
  setTimeout(() => el.classList.remove("show"), 1800);
}

async function setLanguage(lang) {
  state.lang = lang;
  state.i18n = await fetch("/api/i18n/" + lang).then(r => r.json()).catch(() => ({}));
  try { state.lookups = await api("GET", "/api/lookups?lang=" + lang); } catch (e) { state.lookups = {}; }
}

function options(category, selected, withBlank) {
  const list = state.lookups[category] || [];
  let html = withBlank ? `<option value="">${esc(t("none_selected"))}</option>` : "";
  for (const o of list) html += `<option value="${esc(o.code)}" ${o.code === selected ? "selected" : ""}>${esc(o.name)}</option>`;
  return html;
}
function lookupName(category, code) {
  const o = (state.lookups[category] || []).find(x => x.code === code);
  return o ? o.name : (code || "");
}

// ---------- boot ----------
async function boot() {
  const cfg = await api("GET", "/api/config").catch(() => ({ setup_complete: false, default_language: "en" }));
  await setLanguage(cfg.default_language || "en");
  if (!cfg.setup_complete) return renderSetup();
  try {
    state.me = await api("GET", "/api/me");
    if (state.me.language) await setLanguage(state.me.language);
    state.myDivisions = await api("GET", "/api/my-divisions");
    renderApp();
  } catch (e) {
    renderLogin();
  }
}

// ---------- setup wizard ----------
function renderSetup() {
  root.innerHTML = `
  <div class="center-wrap"><div class="panel wide">
    <h1>${esc(t("setup_title"))}</h1>
    <p class="sub">${esc(t("setup_intro"))}</p>
    <div id="setupErr"></div>
    <h3 style="color:var(--accent)">${esc(t("officer_details"))}</h3>
    <div class="grid g2">
      <div class="field"><label>${esc(t("officer_name"))} *</label><input id="s_officer"></div>
      <div class="field"><label>${esc(t("nic"))}</label><input id="s_nic"></div>
      <div class="field"><label>${esc(t("contact"))}</label><input id="s_contact"></div>
      <div class="field"><label>${esc(t("email"))}</label><input id="s_email"></div>
      <div class="field"><label>${esc(t("office_name"))}</label><input id="s_office"></div>
      <div class="field"><label>${esc(t("language"))}</label>
        <select id="s_lang"><option value="en">English</option><option value="si">සිංහල</option><option value="ta">தமிழ்</option></select></div>
    </div>
    <h3 style="color:var(--accent)">${esc(t("select_divisions"))}</h3>
    <p class="muted">${esc(t("select_divisions_help"))}</p>
    <div class="grid g4">
      <div class="field"><label>${esc(t("province"))}</label><select id="s_prov"></select></div>
      <div class="field"><label>${esc(t("district"))}</label><select id="s_dist"></select></div>
      <div class="field"><label>${esc(t("ds_division"))}</label><select id="s_ds"></select></div>
      <div class="field"><label>${esc(t("gn_division"))}</label><select id="s_gn"></select></div>
    </div>
    <button class="btn sec sm" id="s_add">+ ${esc(t("add_division"))}</button>
    <div class="chips" id="s_chips"></div>
    <h3 style="color:var(--accent)">${esc(t("login_credentials"))}</h3>
    <div class="grid g2">
      <div class="field"><label>${esc(t("username"))} *</label><input id="s_user" autocomplete="username"></div>
      <div class="field"><label>${esc(t("password"))} *</label><input id="s_pass" type="password" autocomplete="new-password"></div>
    </div>
    <div class="row-actions"><button class="btn" id="s_finish">${esc(t("finish_setup"))}</button></div>
  </div></div>`;

  const chosen = []; // {id,label}
  const prov = root.querySelector("#s_prov"), dist = root.querySelector("#s_dist"),
        ds = root.querySelector("#s_ds"), gn = root.querySelector("#s_gn");
  const nameOf = o => o["name_" + state.lang] || o.name_en;
  const fill = (sel, rows, ph, labelFn) => { sel.innerHTML = `<option value="">${esc(ph)}</option>` +
    rows.map(r => `<option value="${esc(r.id)}">${esc(labelFn(r))}</option>`).join(""); };

  api("GET", "/api/provinces").then(rows => fill(prov, rows, t("province"), nameOf));
  prov.onchange = async () => { dist.innerHTML = ds.innerHTML = gn.innerHTML = "";
    if (prov.value) fill(dist, await api("GET", "/api/districts?province=" + prov.value), t("district"), nameOf); };
  dist.onchange = async () => { ds.innerHTML = gn.innerHTML = "";
    if (dist.value) fill(ds, await api("GET", "/api/ds-divisions?district=" + dist.value), t("ds_division"), nameOf); };
  ds.onchange = async () => { gn.innerHTML = "";
    if (ds.value) fill(gn, await api("GET", "/api/gn-divisions?ds=" + ds.value), t("gn_division"), r => (r.gn_code ? r.gn_code + " · " : "") + nameOf(r)); };

  const renderChips = () => {
    root.querySelector("#s_chips").innerHTML = chosen.map((c, i) =>
      `<span class="chip">${esc(c.label)}<button data-i="${i}">×</button></span>`).join("");
    root.querySelectorAll("#s_chips .chip button").forEach(b => b.onclick = () => { chosen.splice(+b.dataset.i, 1); renderChips(); });
  };
  root.querySelector("#s_add").onclick = () => {
    if (!gn.value) return;
    if (chosen.some(c => c.id === gn.value)) return;
    chosen.push({ id: gn.value, label: gn.options[gn.selectedIndex].text });
    renderChips();
  };
  root.querySelector("#s_finish").onclick = async () => {
    const payload = {
      officer_name: val("#s_officer"), nic: val("#s_nic"), contact: val("#s_contact"), email: val("#s_email"),
      office_name: val("#s_office"), language: val("#s_lang"), division_ids: chosen.map(c => c.id),
      username: val("#s_user"), password: val("#s_pass"),
    };
    try {
      await api("POST", "/api/setup", payload);
      await setLanguage(payload.language || "en");
      renderLogin();
    } catch (e) {
      root.querySelector("#setupErr").innerHTML = `<div class="err">${esc(e.message)}</div>`;
    }
  };
}
function val(sel) { const e = root.querySelector(sel); return e ? e.value.trim() : ""; }

// ---------- login ----------
function renderLogin() {
  root.innerHTML = `
  <div class="center-wrap"><div class="panel">
    <h1>${esc(t("login_title"))}</h1>
    <p class="sub">${esc(t("app_title"))}</p>
    <div id="loginErr"></div>
    <div class="field"><label>${esc(t("username"))}</label><input id="l_user" autocomplete="username"></div>
    <div class="field"><label>${esc(t("password"))}</label><input id="l_pass" type="password" autocomplete="current-password"></div>
    <div class="row-actions"><button class="btn" id="l_go">${esc(t("login"))}</button></div>
  </div></div>`;
  const go = async () => {
    try {
      await api("POST", "/api/login", { username: val("#l_user"), password: val("#l_pass") });
      state.me = await api("GET", "/api/me");
      if (state.me.language) await setLanguage(state.me.language);
      state.myDivisions = await api("GET", "/api/my-divisions");
      state.view = { name: "dashboard" };
      renderApp();
    } catch (e) {
      root.querySelector("#loginErr").innerHTML = `<div class="err">${esc(t("bad_login"))}</div>`;
    }
  };
  root.querySelector("#l_go").onclick = go;
  root.querySelector("#l_pass").addEventListener("keydown", e => { if (e.key === "Enter") go(); });
}

// ---------- app shell ----------
function renderApp() {
  const nav = [["dashboard", t("dashboard")], ["households", t("households")]];
  root.innerHTML = `
  <header class="app-header"><div class="row">
    <div class="brand">${esc(t("app_short"))}<small>${esc(state.me.officer_name || t("app_title"))}</small></div>
    <nav class="main">${nav.map(([k, l]) => `<a href="#" data-nav="${k}" class="${state.view.name === k ? "active" : ""}">${esc(l)}</a>`).join("")}</nav>
    <div class="spacer"></div>
    <div class="header-tools">
      <select id="langSel" title="${esc(t("language"))}">
        <option value="en" ${state.lang === "en" ? "selected" : ""}>EN</option>
        <option value="si" ${state.lang === "si" ? "selected" : ""}>සි</option>
        <option value="ta" ${state.lang === "ta" ? "selected" : ""}>த</option>
      </select>
      <span class="who">${esc(state.me.username)}</span>
      <button class="btn sec sm" id="logout">${esc(t("logout"))}</button>
    </div>
  </div></header>
  <main id="view"></main>`;

  root.querySelectorAll("[data-nav]").forEach(a => a.onclick = e => { e.preventDefault(); state.view = { name: a.dataset.nav }; renderCurrent(); });
  root.querySelector("#logout").onclick = async () => { await api("POST", "/api/logout"); location.reload(); };
  root.querySelector("#langSel").onchange = async e => {
    await setLanguage(e.target.value);
    renderApp();
  };
  renderCurrent();
}

function setActiveNav() {
  root.querySelectorAll("[data-nav]").forEach(a => a.classList.toggle("active", a.dataset.nav === (state.view.name === "household" ? "households" : state.view.name)));
}

function renderCurrent() {
  setActiveNav();
  if (state.view.name === "dashboard") return renderDashboard();
  if (state.view.name === "households") return renderHouseholds();
  if (state.view.name === "household") return renderHousehold(state.view.id);
}

// ---------- dashboard ----------
async function renderDashboard() {
  const view = document.getElementById("view");
  const stats = await api("GET", "/api/stats").catch(() => ({ households: 0, members: 0, divisions: 0 }));
  view.innerHTML = `
    <div class="grid g3">
      <div class="stat"><div class="n">${stats.households}</div><div class="l">${esc(t("households"))}</div></div>
      <div class="stat"><div class="n">${stats.members}</div><div class="l">${esc(t("members"))}</div></div>
      <div class="stat"><div class="n">${stats.divisions}</div><div class="l">${esc(t("divisions"))}</div></div>
    </div>
    <div class="card">
      <h2>${esc(t("divisions"))}</h2>
      <table><thead><tr><th>${esc(t("gn_division"))}</th><th>${esc(t("ds_division"))}</th></tr></thead>
      <tbody>${state.myDivisions.map(d => `<tr><td>${esc((d.gn_code ? d.gn_code + " · " : "") + (d["name_" + state.lang] || d.name_en))}</td><td>${esc(d.ds_name)}</td></tr>`).join("") || `<tr><td colspan="2" class="empty">—</td></tr>`}</tbody></table>
    </div>`;
}

// ---------- households list ----------
async function renderHouseholds() {
  const view = document.getElementById("view");
  const divOpts = `<option value="">${esc(t("all_divisions"))}</option>` +
    state.myDivisions.map(d => `<option value="${esc(d.id)}">${esc((d.gn_code ? d.gn_code + " · " : "") + (d["name_" + state.lang] || d.name_en))}</option>`).join("");
  view.innerHTML = `
    <div class="toolbar">
      <div class="field grow"><label>${esc(t("search"))}</label><input id="q" placeholder="${esc(t("search_ph"))}"></div>
      <div class="field"><label>${esc(t("village"))}</label><input id="qvillage"></div>
      <div class="field"><label>${esc(t("gn_division"))}</label><select id="qdiv">${divOpts}</select></div>
      <button class="btn sec" id="qgo">${esc(t("search"))}</button>
      <div class="spacer"></div>
      <button class="btn" id="newhh">+ ${esc(t("new_household"))}</button>
    </div>
    <div id="hhlist"><p class="muted">${esc(t("loading"))}</p></div>`;

  const load = async () => {
    const params = new URLSearchParams();
    if (val("#q")) params.set("q", val("#q"));
    if (val("#qvillage")) params.set("village", val("#qvillage"));
    const d = root.querySelector("#qdiv").value; if (d) params.set("division", d);
    const rows = await api("GET", "/api/households?" + params.toString());
    const box = document.getElementById("hhlist");
    if (!rows.length) { box.innerHTML = `<div class="card"><div class="empty">${esc(t("no_results"))}</div></div>`; return; }
    box.innerHTML = `<table><thead><tr>
        <th>${esc(t("head_name"))}</th><th>${esc(t("household_number"))}</th><th>${esc(t("village"))}</th>
        <th>${esc(t("gn_division"))}</th><th>${esc(t("phone_mobile"))}</th><th>${esc(t("member_count"))}</th></tr></thead>
      <tbody>${rows.map(r => `<tr data-id="${esc(r.id)}">
        <td>${esc(r.head_name) || "—"}</td><td>${esc(r.household_number) || "—"}</td><td>${esc(r.village) || "—"}</td>
        <td>${esc((r.gn_code ? r.gn_code + " · " : "") + (r.division_name || ""))}</td>
        <td>${esc(r.phone_mobile) || "—"}</td><td><span class="pill">${r.member_count}</span></td></tr>`).join("")}</tbody></table>`;
    box.querySelectorAll("tr[data-id]").forEach(tr => tr.onclick = () => { state.view = { name: "household", id: tr.dataset.id }; renderCurrent(); });
  };
  root.querySelector("#qgo").onclick = load;
  root.querySelector("#q").addEventListener("keydown", e => { if (e.key === "Enter") load(); });
  root.querySelector("#newhh").onclick = () => { state.view = { name: "household", id: null }; renderCurrent(); };
  load();
}

// ---------- household detail / edit ----------
async function renderHousehold(id) {
  const view = document.getElementById("view");
  let hh = { household: {}, housing: {}, members: [], welfare: [] };
  if (id) hh = await api("GET", "/api/households/" + id);
  const H = hh.household || {}, HS = hh.housing || {};
  const divOpts = state.myDivisions.map(d => `<option value="${esc(d.id)}" ${d.id === H.gn_division_id ? "selected" : ""}>${esc((d.gn_code ? d.gn_code + " · " : "") + (d["name_" + state.lang] || d.name_en))}</option>`).join("");

  view.innerHTML = `
    <button class="btn sec sm" id="back">← ${esc(t("back"))}</button>
    <div class="card" style="margin-top:.8rem">
      <h2>${esc(t("household_info"))}</h2>
      <div class="grid g3">
        <div class="field"><label>${esc(t("gn_division"))} *</label><select id="h_div">${divOpts}</select></div>
        <div class="field"><label>${esc(t("household_number"))}</label><input id="h_hn" value="${esc(H.household_number)}"></div>
        <div class="field"><label>${esc(t("head_name"))}</label><input id="h_head" value="${esc(H.head_name)}"></div>
        <div class="field"><label>${esc(t("village"))}</label><input id="h_village" value="${esc(H.village)}"></div>
        <div class="field"><label>${esc(t("phone_mobile"))}</label><input id="h_mobile" value="${esc(H.phone_mobile)}"></div>
        <div class="field"><label>${esc(t("phone_home"))}</label><input id="h_home" value="${esc(H.phone_home)}"></div>
        <div class="field" style="grid-column:1/-1"><label>${esc(t("address"))}</label><input id="h_addr" value="${esc(H.address)}"></div>
      </div>
      <h3>${esc(t("housing_info"))}</h3>
      <div class="grid g3">
        <div class="field"><label>${esc(t("house_type"))}</label><select id="h_housetype">${options("house_type", HS.house_type, true)}</select></div>
        <div class="field"><label>${esc(t("wall_type"))}</label><select id="h_wall">${options("wall_type", HS.wall_type, true)}</select></div>
        <div class="field"><label>${esc(t("roof_type"))}</label><select id="h_roof">${options("roof_type", HS.roof_type, true)}</select></div>
        <div class="field"><label>${esc(t("floor_type"))}</label><select id="h_floor">${options("floor_type", HS.floor_type, true)}</select></div>
        <div class="field"><label>${esc(t("toilet_type"))}</label><select id="h_toilet">${options("toilet_type", HS.toilet_type, true)}</select></div>
        <div class="field"><label>${esc(t("drinking_water"))}</label><select id="h_water">${options("water_source", HS.drinking_water, true)}</select></div>
        <div class="field"><label>${esc(t("cooking_fuel"))}</label><select id="h_fuel">${options("cooking_fuel", HS.cooking_fuel, true)}</select></div>
        <div class="field"><label>${esc(t("garbage_disposal"))}</label><select id="h_garbage">${options("garbage_disposal", HS.garbage_disposal, true)}</select></div>
        <div class="field"><label>${esc(t("has_electricity"))}</label><select id="h_elec">
          <option value="">${esc(t("none_selected"))}</option>
          <option value="1" ${String(HS.has_electricity) === "1" ? "selected" : ""}>${esc(t("yes"))}</option>
          <option value="0" ${String(HS.has_electricity) === "0" ? "selected" : ""}>${esc(t("no"))}</option></select></div>
      </div>
      <div class="row-actions"><button class="btn" id="h_save">${esc(t("save"))}</button></div>
    </div>
    <div class="card" id="membersCard"></div>
    <div class="card" id="welfareCard"></div>`;

  root.querySelector("#back").onclick = () => { state.view = { name: "households" }; renderCurrent(); };

  const save = async () => {
    const body = {
      gn_division_id: root.querySelector("#h_div").value,
      household_number: val("#h_hn"), head_name: val("#h_head"), village: val("#h_village"),
      phone_mobile: val("#h_mobile"), phone_home: val("#h_home"), address: val("#h_addr"),
      housing: {
        house_type: root.querySelector("#h_housetype").value, wall_type: root.querySelector("#h_wall").value,
        roof_type: root.querySelector("#h_roof").value, floor_type: root.querySelector("#h_floor").value,
        toilet_type: root.querySelector("#h_toilet").value, drinking_water: root.querySelector("#h_water").value,
        cooking_fuel: root.querySelector("#h_fuel").value, garbage_disposal: root.querySelector("#h_garbage").value,
        has_electricity: root.querySelector("#h_elec").value === "" ? null : Number(root.querySelector("#h_elec").value),
      },
    };
    try {
      if (id) { await api("PUT", "/api/households/" + id, body); toast(t("saved")); }
      else { const r = await api("POST", "/api/households", body); state.view = { name: "household", id: r.id }; renderCurrent(); toast(t("saved")); }
    } catch (e) { toast(e.message); }
  };
  root.querySelector("#h_save").onclick = save;

  renderMembers(id, hh.members || []);
  renderWelfare(id, hh.welfare || []);
}

function renderMembers(householdId, members) {
  const card = document.getElementById("membersCard");
  if (!householdId) { card.innerHTML = `<p class="muted">${esc(t("members_in_household"))} — ${esc(t("save"))} ${esc(t("household_info")).toLowerCase()} first.</p>`; return; }
  card.innerHTML = `
    <div class="section-sub"><h3>${esc(t("members_in_household"))}</h3><button class="btn sec sm" id="addMember">+ ${esc(t("add_member"))}</button></div>
    <table><thead><tr><th>${esc(t("full_name"))}</th><th>${esc(t("relationship"))}</th><th>${esc(t("nic"))}</th>
      <th>${esc(t("gender"))}</th><th>${esc(t("education_level"))}</th><th></th></tr></thead>
    <tbody>${members.length ? members.map(m => `<tr>
        <td>${esc(m.full_name_en || m.full_name_si) || "—"}</td>
        <td>${esc(lookupName("relationship", m.relationship_to_head)) || "—"}</td>
        <td>${esc(m.nic) || "—"}</td>
        <td>${esc(lookupName("gender", m.gender)) || "—"}</td>
        <td>${esc(lookupName("education_level", m.education_level)) || "—"}</td>
        <td style="text-align:right"><button class="btn sec sm" data-edit="${esc(m.id)}">${esc(t("edit"))}</button>
          <button class="btn danger sm" data-del="${esc(m.id)}">${esc(t("delete"))}</button></td></tr>`).join("")
      : `<tr><td colspan="6" class="empty">—</td></tr>`}</tbody></table>`;
  card.querySelector("#addMember").onclick = () => memberModal(householdId, null);
  card.querySelectorAll("[data-edit]").forEach(b => b.onclick = () => memberModal(householdId, members.find(m => m.id === b.dataset.edit)));
  card.querySelectorAll("[data-del]").forEach(b => b.onclick = async () => {
    if (!confirm(t("confirm_delete"))) return;
    await api("DELETE", "/api/members/" + b.dataset.del); reloadHousehold(householdId);
  });
}

function memberModal(householdId, m) {
  m = m || {};
  const flags = state.lookups["member_flag"] || [];
  const checked = new Set(); // flags not loaded per-member in list; left for future detail
  const bg = document.createElement("div");
  bg.className = "modal-bg";
  bg.innerHTML = `<div class="modal">
    <h2>${esc(m.id ? t("edit") : t("add_member"))}</h2>
    <div class="grid g2">
      <div class="field"><label>${esc(t("full_name"))} (EN)</label><input id="m_en" value="${esc(m.full_name_en)}"></div>
      <div class="field"><label>${esc(t("full_name"))} (සිං)</label><input id="m_si" value="${esc(m.full_name_si)}"></div>
      <div class="field"><label>${esc(t("relationship"))}</label><select id="m_rel">${options("relationship", m.relationship_to_head, true)}</select></div>
      <div class="field"><label>${esc(t("nic"))}</label><input id="m_nic" value="${esc(m.nic)}"></div>
      <div class="field"><label>${esc(t("dob"))}</label><input id="m_dob" type="date" value="${esc(m.dob)}"></div>
      <div class="field"><label>${esc(t("gender"))}</label><select id="m_gender">${options("gender", m.gender, true)}</select></div>
      <div class="field"><label>${esc(t("religion"))}</label><select id="m_rel2">${options("religion", m.religion, true)}</select></div>
      <div class="field"><label>${esc(t("ethnicity"))}</label><select id="m_eth">${options("ethnicity", m.ethnicity, true)}</select></div>
      <div class="field"><label>${esc(t("education_level"))}</label><select id="m_edu">${options("education_level", m.education_level, true)}</select></div>
      <div class="field"><label>${esc(t("occupation"))}</label><input id="m_occ" value="${esc(m.occupation)}"></div>
    </div>
    <label>${esc(t("flags"))}</label>
    <div class="flag-row">${flags.map(f => `<label><input type="checkbox" value="${esc(f.code)}" ${checked.has(f.code) ? "checked" : ""}> ${esc(f.name)}</label>`).join("")}</div>
    <div class="row-actions"><button class="btn sec" id="m_cancel">${esc(t("cancel"))}</button><button class="btn" id="m_save">${esc(t("save"))}</button></div>
  </div>`;
  document.body.appendChild(bg);
  bg.querySelector("#m_cancel").onclick = () => bg.remove();
  bg.onclick = e => { if (e.target === bg) bg.remove(); };
  bg.querySelector("#m_save").onclick = async () => {
    const q = s => bg.querySelector(s).value.trim();
    const body = {
      full_name_en: q("#m_en"), full_name_si: q("#m_si"), relationship_to_head: q("#m_rel"),
      nic: q("#m_nic"), dob: q("#m_dob"), gender: q("#m_gender"), religion: q("#m_rel2"),
      ethnicity: q("#m_eth"), education_level: q("#m_edu"), occupation: q("#m_occ"),
      flags: Array.from(bg.querySelectorAll(".flag-row input:checked")).map(c => c.value),
    };
    try {
      if (m.id) await api("PUT", "/api/members/" + m.id, body);
      else await api("POST", "/api/households/" + householdId + "/members", body);
      bg.remove(); reloadHousehold(householdId); toast(t("saved"));
    } catch (e) { toast(e.message); }
  };
}

async function renderWelfare(householdId, welfare) {
  const card = document.getElementById("welfareCard");
  if (!householdId) { card.innerHTML = ""; return; }
  const programs = await api("GET", "/api/welfare-programs").catch(() => []);
  const progName = p => p["name_" + state.lang] || p.name_en;
  card.innerHTML = `
    <div class="section-sub"><h3>${esc(t("welfare"))}</h3></div>
    <table><thead><tr><th>${esc(t("program"))}</th><th>${esc(t("reference"))}</th><th>${esc(t("amount"))}</th><th></th></tr></thead>
    <tbody>${welfare.length ? welfare.map(w => `<tr>
        <td>${esc(w.program_name)}</td><td>${esc(w.beneficiary_ref) || "—"}</td><td>${w.amount != null ? esc(w.amount) : "—"}</td>
        <td style="text-align:right"><button class="btn danger sm" data-delw="${esc(w.id)}">${esc(t("delete"))}</button></td></tr>`).join("")
      : `<tr><td colspan="4" class="empty">—</td></tr>`}</tbody></table>
    <div class="toolbar" style="margin-top:.8rem">
      <div class="field"><label>${esc(t("program"))}</label><select id="w_prog">${programs.map(p => `<option value="${esc(p.id)}">${esc(progName(p))}</option>`).join("")}</select></div>
      <div class="field"><label>${esc(t("reference"))}</label><input id="w_ref"></div>
      <div class="field"><label>${esc(t("amount"))}</label><input id="w_amt" type="number" step="0.01"></div>
      <button class="btn sec" id="w_add">+ ${esc(t("add_welfare"))}</button>
    </div>`;
  card.querySelectorAll("[data-delw]").forEach(b => b.onclick = async () => { await api("DELETE", "/api/welfare/" + b.dataset.delw); reloadHousehold(householdId); });
  card.querySelector("#w_add").onclick = async () => {
    const prog = card.querySelector("#w_prog").value; if (!prog) return;
    try {
      await api("POST", "/api/households/" + householdId + "/welfare", {
        program_id: prog, beneficiary_ref: card.querySelector("#w_ref").value.trim(),
        amount: parseFloat(card.querySelector("#w_amt").value) || 0,
      });
      reloadHousehold(householdId); toast(t("saved"));
    } catch (e) { toast(e.message); }
  };
}

async function reloadHousehold(id) {
  const hh = await api("GET", "/api/households/" + id);
  renderMembers(id, hh.members || []);
  renderWelfare(id, hh.welfare || []);
}

boot();
