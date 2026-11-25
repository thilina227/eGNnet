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
  const nav = [["dashboard", t("dashboard")], ["households", t("households")], ["inventory", t("inventory")], ["diary", t("diary")], ["settings", t("settings")]];
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
  if (state.view.name === "inventory") return renderInventory();
  if (state.view.name === "diary") return renderDiary();
  if (state.view.name === "settings") return renderSettings();
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
    <div class="card" id="welfareCard"></div>
    <div class="card" id="assetsCard"></div>`;

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
  renderAssets(id);
}

function renderMembers(householdId, members) {
  const card = document.getElementById("membersCard");
  if (!householdId) { card.innerHTML = `<p class="muted">${esc(t("members_in_household"))} — ${esc(t("save"))} ${esc(t("household_info")).toLowerCase()} first.</p>`; return; }
  card.innerHTML = `
    <div class="section-sub"><h3>${esc(t("members_in_household"))}</h3><button class="btn sec sm" id="addMember">+ ${esc(t("add_member"))}</button></div>
    <table><thead><tr><th></th><th>${esc(t("full_name"))}</th><th>${esc(t("relationship"))}</th><th>${esc(t("nic"))}</th>
      <th>${esc(t("gender"))}</th><th>${esc(t("education_level"))}</th><th></th></tr></thead>
    <tbody>${members.length ? members.map(m => `<tr>
        <td>${m.photo_path ? `<img class="avatar" src="/api/members/${esc(m.id)}/photo?v=${esc(m.updated_at)}" alt="">` : `<span class="avatar avatar-ph">${esc((m.full_name_en || m.full_name_si || "?").trim().charAt(0).toUpperCase())}</span>`}</td>
        <td>${esc(m.full_name_en || m.full_name_si) || "—"}</td>
        <td>${esc(lookupName("relationship", m.relationship_to_head)) || "—"}</td>
        <td>${esc(m.nic) || "—"}</td>
        <td>${esc(lookupName("gender", m.gender)) || "—"}</td>
        <td>${esc(lookupName("education_level", m.education_level)) || "—"}</td>
        <td style="text-align:right"><button class="btn sec sm" data-edit="${esc(m.id)}">${esc(t("edit"))}</button>
          <button class="btn danger sm" data-del="${esc(m.id)}">${esc(t("delete"))}</button></td></tr>`).join("")
      : `<tr><td colspan="7" class="empty">—</td></tr>`}</tbody></table>`;
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
    <div class="photo-edit">
      <div class="photo-frame" id="m_photo_frame">
        ${m.photo_path ? `<img src="/api/members/${esc(m.id)}/photo?v=${esc(m.updated_at)}" alt="">` : `<span class="photo-ph">${esc(t("no_photo"))}</span>`}
      </div>
      <div class="photo-actions">
        <label class="btn sec sm">${esc(t("choose_photo"))}<input type="file" id="m_photo_input" accept="image/jpeg,image/png,image/webp" hidden></label>
        <button type="button" class="btn danger sm" id="m_photo_remove" ${m.photo_path ? "" : "style=display:none"}>${esc(t("remove_photo"))}</button>
        <div class="muted">${esc(t("photo_hint"))}</div>
      </div>
    </div>
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

  // photo handling
  let photoFile = null, photoRemoved = false;
  const frame = bg.querySelector("#m_photo_frame");
  const removeBtn = bg.querySelector("#m_photo_remove");
  bg.querySelector("#m_photo_input").onchange = e => {
    const f = e.target.files[0];
    if (!f) return;
    if (f.size > 5 * 1024 * 1024) { toast(t("photo_too_large")); e.target.value = ""; return; }
    photoFile = f; photoRemoved = false;
    frame.innerHTML = `<img src="${URL.createObjectURL(f)}" alt="">`;
    removeBtn.style.display = "";
  };
  removeBtn.onclick = () => {
    photoFile = null; photoRemoved = true;
    frame.innerHTML = `<span class="photo-ph">${esc(t("no_photo"))}</span>`;
    removeBtn.style.display = "none";
    bg.querySelector("#m_photo_input").value = "";
  };

  bg.querySelector("#m_save").onclick = async () => {
    const q = s => bg.querySelector(s).value.trim();
    const body = {
      full_name_en: q("#m_en"), full_name_si: q("#m_si"), relationship_to_head: q("#m_rel"),
      nic: q("#m_nic"), dob: q("#m_dob"), gender: q("#m_gender"), religion: q("#m_rel2"),
      ethnicity: q("#m_eth"), education_level: q("#m_edu"), occupation: q("#m_occ"),
      flags: Array.from(bg.querySelectorAll(".flag-row input:checked")).map(c => c.value),
    };
    try {
      let mid = m.id;
      if (mid) await api("PUT", "/api/members/" + mid, body);
      else { const r = await api("POST", "/api/households/" + householdId + "/members", body); mid = r.id; }
      if (photoFile) {
        const fd = new FormData(); fd.append("photo", photoFile);
        const res = await fetch("/api/members/" + mid + "/photo", { method: "POST", body: fd });
        if (!res.ok) { const d = await res.json().catch(() => ({})); throw new Error(d.error || t("photo_upload_failed")); }
      } else if (photoRemoved && m.id) {
        await api("DELETE", "/api/members/" + mid + "/photo");
      }
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
  renderAssets(id);
}

// ---------- assets & economy ----------
const ASSET_SECTIONS = [
  { coll: "land_holding", title: "land_holdings", fields: [
    { k: "land_class", t: "land_class", type: "lookup", cat: "land_class" },
    { k: "tenure_type", t: "land_tenure", type: "lookup", cat: "land_tenure" },
    { k: "acres", t: "acres", type: "number" }, { k: "roods", t: "roods", type: "number" }, { k: "perches", t: "perches", type: "number" },
    { k: "is_main", t: "is_main", type: "bool" }, { k: "notes", t: "notes", type: "text" } ] },
  { coll: "land_crop", title: "crops", fields: [
    { k: "crop_type", t: "crop_type", type: "lookup", cat: "crop_type" },
    { k: "extent_or_trees", t: "extent_or_trees", type: "text" }, { k: "is_main", t: "is_main", type: "bool" } ] },
  { coll: "paddy_cultivation", title: "paddy", fields: [
    { k: "season", t: "season", type: "lookup", cat: "paddy_season" }, { k: "extent", t: "extent_or_trees", type: "text" }, { k: "method", t: "method", type: "text" } ] },
  { coll: "livestock", title: "livestock", fields: [
    { k: "animal_type", t: "animal_type", type: "lookup", cat: "livestock_type" }, { k: "count", t: "count", type: "number" } ] },
  { coll: "poultry", title: "poultry", fields: [
    { k: "poultry_type", t: "poultry_type", type: "lookup", cat: "poultry_type" }, { k: "count", t: "count", type: "number" } ] },
  { coll: "appliance", title: "appliances", fields: [
    { k: "appliance_type", t: "appliance_type", type: "lookup", cat: "appliance_type" }, { k: "count", t: "count", type: "number" } ] },
  { coll: "vehicle", title: "vehicles", fields: [
    { k: "vehicle_type", t: "vehicle_type", type: "lookup", cat: "vehicle_type" }, { k: "registration_no", t: "registration_no", type: "text" },
    { k: "count", t: "count", type: "number" }, { k: "income_use", t: "income_use", type: "bool" } ] },
  { coll: "equipment", title: "equipment", fields: [
    { k: "equipment_type", t: "equipment_type", type: "lookup", cat: "equipment_type" }, { k: "count", t: "count", type: "number" }, { k: "income_use", t: "income_use", type: "bool" } ] },
  { coll: "permit", title: "permits", fields: [
    { k: "permit_type", t: "permit_type", type: "lookup", cat: "permit_type" }, { k: "permit_no", t: "permit_no", type: "text" },
    { k: "holder_name", t: "holder_name", type: "text" }, { k: "valid_until", t: "valid_until", type: "date" } ] },
  { coll: "loan", title: "loans", fields: [
    { k: "is_microfinance", t: "microfinance", type: "bool" }, { k: "provider", t: "provider", type: "lookup", cat: "loan_provider" }, { k: "details", t: "details", type: "text" } ] },
  { coll: "state_aid", title: "state_aid", fields: [
    { k: "project", t: "project", type: "text" }, { k: "year", t: "year", type: "number" }, { k: "aid_item", t: "aid_item", type: "text" }, { k: "progress", t: "progress", type: "text" } ] },
  { coll: "income_source", title: "income_sources", fields: [
    { k: "source_type", t: "source_type", type: "lookup", cat: "income_source" } ] },
  { coll: "monthly_expense", title: "monthly_expenses", fields: [
    { k: "category", t: "expense_category", type: "lookup", cat: "expense_category" }, { k: "amount", t: "amount", type: "number" } ] },
];

function assetCell(f, row) {
  const v = row[f.k];
  if (f.type === "bool") return (String(v) === "1" || v === true) ? esc(t("yes")) : (v == null || v === "" ? "—" : esc(t("no")));
  if (f.type === "lookup") return esc(lookupName(f.cat, v)) || "—";
  return (v == null || v === "") ? "—" : esc(v);
}

async function renderAssets(householdId) {
  const card = document.getElementById("assetsCard");
  if (!card) return;
  if (!householdId) { card.innerHTML = ""; return; }
  const data = await api("GET", "/api/households/" + householdId + "/assets").catch(() => ({}));
  const fin = (data && data.household_finance) || {};
  let html = `<div class="section-sub"><h3>${esc(t("assets_economy"))}</h3></div>
    <div class="toolbar">
      <div class="field grow"><label>${esc(t("monthly_income_bracket"))}</label>
        <select id="fin_bracket">${options("income_bracket", fin.monthly_income_bracket, true)}</select></div>
      <button class="btn sec" id="fin_save">${esc(t("save"))}</button>
    </div>`;
  for (const sec of ASSET_SECTIONS) {
    const rows = (data && data[sec.coll]) || [];
    html += `<div class="section-sub" style="margin-top:1rem"><h4>${esc(t(sec.title))}</h4>
        <button class="btn sec sm" data-add="${sec.coll}">+ ${esc(t("add_row"))}</button></div>
      <table><thead><tr>${sec.fields.map(f => `<th>${esc(t(f.t))}</th>`).join("")}<th></th></tr></thead>
      <tbody>${rows.length ? rows.map(r => `<tr>
          ${sec.fields.map(f => `<td>${assetCell(f, r)}</td>`).join("")}
          <td style="text-align:right"><button class="btn sec sm" data-edit="${sec.coll}|${esc(r.id)}">${esc(t("edit"))}</button>
            <button class="btn danger sm" data-del="${sec.coll}|${esc(r.id)}">${esc(t("delete"))}</button></td></tr>`).join("")
        : `<tr><td colspan="${sec.fields.length + 1}" class="empty">—</td></tr>`}</tbody></table>`;
  }
  card.innerHTML = html;
  card.querySelector("#fin_save").onclick = async () => {
    try { await api("PUT", "/api/households/" + householdId + "/finance", { monthly_income_bracket: card.querySelector("#fin_bracket").value }); toast(t("saved")); }
    catch (e) { toast(e.message); }
  };
  card.querySelectorAll("[data-add]").forEach(b => b.onclick = () => assetModal(householdId, b.dataset.add, null));
  card.querySelectorAll("[data-edit]").forEach(b => b.onclick = () => {
    const [coll, rid] = b.dataset.edit.split("|");
    const row = ((data && data[coll]) || []).find(x => x.id === rid);
    assetModal(householdId, coll, row);
  });
  card.querySelectorAll("[data-del]").forEach(b => b.onclick = async () => {
    const [coll, rid] = b.dataset.del.split("|");
    if (!confirm(t("confirm_delete"))) return;
    await api("DELETE", "/api/assets/" + coll + "/" + rid); renderAssets(householdId);
  });
}

function assetModal(householdId, coll, row) {
  row = row || {};
  const sec = ASSET_SECTIONS.find(s => s.coll === coll);
  const inputFor = (f) => {
    const id = "a_" + f.k;
    if (f.type === "lookup") return `<select id="${id}">${options(f.cat, row[f.k], true)}</select>`;
    if (f.type === "bool") return `<select id="${id}">
        <option value="">${esc(t("none_selected"))}</option>
        <option value="1" ${String(row[f.k]) === "1" || row[f.k] === true ? "selected" : ""}>${esc(t("yes"))}</option>
        <option value="0" ${String(row[f.k]) === "0" || row[f.k] === false ? "selected" : ""}>${esc(t("no"))}</option></select>`;
    if (f.type === "number") return `<input id="${id}" type="number" step="any" value="${esc(row[f.k])}">`;
    if (f.type === "date") return `<input id="${id}" type="date" value="${esc(row[f.k])}">`;
    return `<input id="${id}" value="${esc(row[f.k])}">`;
  };
  const bg = document.createElement("div");
  bg.className = "modal-bg";
  bg.innerHTML = `<div class="modal"><h2>${esc(row.id ? t("edit") : t("add_row"))} — ${esc(t(sec.title))}</h2>
    <div class="grid g2">${sec.fields.map(f => `<div class="field"><label>${esc(t(f.t))}</label>${inputFor(f)}</div>`).join("")}</div>
    <div class="row-actions"><button class="btn sec" id="a_cancel">${esc(t("cancel"))}</button><button class="btn" id="a_save">${esc(t("save"))}</button></div></div>`;
  document.body.appendChild(bg);
  bg.querySelector("#a_cancel").onclick = () => bg.remove();
  bg.onclick = e => { if (e.target === bg) bg.remove(); };
  bg.querySelector("#a_save").onclick = async () => {
    const body = {};
    for (const f of sec.fields) {
      const el = bg.querySelector("#a_" + f.k);
      let v = el.value;
      if (f.type === "number") v = v === "" ? null : Number(v);
      else if (f.type === "bool") v = v === "" ? null : Number(v);
      else v = v.trim();
      body[f.k] = v;
    }
    try {
      if (row.id) await api("PUT", "/api/assets/" + coll + "/" + row.id, body);
      else await api("POST", "/api/households/" + householdId + "/assets/" + coll, body);
      bg.remove(); renderAssets(householdId); toast(t("saved"));
    } catch (e) { toast(e.message); }
  };
}

// ---------- settings: application log + retention ----------
const LOG_CATEGORIES = ["", "auth", "admin", "error", "sync", "system"];
const LOG_LEVELS = ["", "info", "warn", "error"];
const logFilter = { level: "", category: "", q: "" };

function fmtBytes(n) {
  n = Number(n) || 0;
  if (n < 1024) return n + " B";
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + " KB";
  return (n / (1024 * 1024)).toFixed(1) + " MB";
}
function fmtTime(s) { return (s || "").replace("T", " ").replace("Z", ""); }

async function renderSettings() {
  const view = document.getElementById("view");
  const cfg = await api("GET", "/api/admin/log-config").catch(() => ({ retention_days: 180, app_log_count: 0, audit_log_count: 0, archives: [] }));
  view.innerHTML = `
    <div class="grid g3">
      <div class="stat"><div class="n">${cfg.app_log_count}</div><div class="l">${esc(t("app_log_entries"))}</div></div>
      <div class="stat"><div class="n">${cfg.audit_log_count}</div><div class="l">${esc(t("audit_log_entries"))}</div></div>
      <div class="stat"><div class="n">${(cfg.archives || []).length}</div><div class="l">${esc(t("archived_files"))}</div></div>
    </div>
    <div class="card">
      <h2>${esc(t("log_retention"))}</h2>
      <p class="muted">${esc(t("log_retention_help"))}</p>
      <div class="toolbar">
        <div class="field"><label>${esc(t("retention_days"))}</label>
          <input id="ret_days" type="number" min="1" max="3650" value="${esc(cfg.retention_days)}"></div>
        <button class="btn" id="ret_save">${esc(t("save"))}</button>
        <button class="btn sec" id="ret_sweep">${esc(t("run_cleanup"))}</button>
        <a class="btn sec" href="/api/admin/logs/export">${esc(t("export_csv"))}</a>
      </div>
      ${(cfg.archives || []).length ? `<h3 style="margin-top:1rem">${esc(t("archived_files"))}</h3>
        <table><thead><tr><th>${esc(t("file"))}</th><th>${esc(t("size"))}</th><th>${esc(t("date"))}</th></tr></thead>
        <tbody>${cfg.archives.map(a => `<tr><td>${esc(a.name)}</td><td>${esc(fmtBytes(a.size_bytes))}</td><td>${esc(fmtTime(a.modified_at))}</td></tr>`).join("")}</tbody></table>` : ""}
    </div>
    <div class="card">
      <h2>${esc(t("application_log"))}</h2>
      <p class="muted">${esc(t("application_log_help"))}</p>
      <div class="toolbar">
        <div class="field"><label>${esc(t("log_level"))}</label>
          <select id="f_level">${LOG_LEVELS.map(l => `<option value="${l}" ${logFilter.level === l ? "selected" : ""}>${l ? esc(t("level_" + l)) : esc(t("all"))}</option>`).join("")}</select></div>
        <div class="field"><label>${esc(t("category"))}</label>
          <select id="f_cat">${LOG_CATEGORIES.map(c => `<option value="${c}" ${logFilter.category === c ? "selected" : ""}>${c ? esc(t("cat_" + c)) : esc(t("all"))}</option>`).join("")}</select></div>
        <div class="field grow"><label>${esc(t("search"))}</label><input id="f_q" value="${esc(logFilter.q)}"></div>
      </div>
      <div id="logTable"></div>
    </div>`;

  view.querySelector("#ret_save").onclick = async () => {
    const days = Number(view.querySelector("#ret_days").value);
    try { await api("PUT", "/api/admin/log-config", { retention_days: days }); toast(t("saved")); renderSettings(); }
    catch (e) { toast(e.message); }
  };
  view.querySelector("#ret_sweep").onclick = async () => {
    if (!confirm(t("run_cleanup_confirm"))) return;
    try { const r = await api("POST", "/api/admin/logs/sweep"); toast(t("saved")); renderSettings(); }
    catch (e) { toast(e.message); }
  };
  const reload = () => loadLogTable(view);
  view.querySelector("#f_level").onchange = e => { logFilter.level = e.target.value; reload(); };
  view.querySelector("#f_cat").onchange = e => { logFilter.category = e.target.value; reload(); };
  let deb;
  view.querySelector("#f_q").oninput = e => { logFilter.q = e.target.value; clearTimeout(deb); deb = setTimeout(reload, 250); };
  loadLogTable(view);
}

async function loadLogTable(view) {
  const el = view.querySelector("#logTable");
  if (!el) return;
  const qs = new URLSearchParams();
  if (logFilter.level) qs.set("level", logFilter.level);
  if (logFilter.category) qs.set("category", logFilter.category);
  if (logFilter.q) qs.set("q", logFilter.q);
  const rows = await api("GET", "/api/admin/logs?" + qs.toString()).catch(() => []);
  el.innerHTML = `<table><thead><tr>
      <th>${esc(t("time"))}</th><th>${esc(t("log_level"))}</th><th>${esc(t("category"))}</th>
      <th>${esc(t("user"))}</th><th>${esc(t("action"))}</th><th>${esc(t("detail"))}</th></tr></thead>
    <tbody>${rows.length ? rows.map(r => `<tr>
        <td>${esc(fmtTime(r.created_at))}</td>
        <td><span class="badge badge-${esc(r.level)}">${esc(t("level_" + r.level) || r.level)}</span></td>
        <td>${esc(t("cat_" + r.category) || r.category)}</td>
        <td>${esc(r.username || "")}</td>
        <td>${esc(r.action)}</td>
        <td>${esc(r.detail || "")}</td></tr>`).join("")
      : `<tr><td colspan="6" class="empty">—</td></tr>`}</tbody></table>`;
}

// ---------- office inventory ----------
const invFilter = { division: "", q: "" };

async function renderInventory() {
  const view = document.getElementById("view");
  const divSel = state.myDivisions.length > 1
    ? `<div class="field"><label>${esc(t("gn_division"))}</label><select id="inv_div">
        <option value="">${esc(t("all_divisions"))}</option>
        ${state.myDivisions.map(d => `<option value="${esc(d.id)}" ${invFilter.division === d.id ? "selected" : ""}>${esc((d.gn_code ? d.gn_code + " · " : "") + (d["name_" + state.lang] || d.name_en))}</option>`).join("")}</select></div>`
    : "";
  view.innerHTML = `
    <div class="card">
      <div class="section-sub"><h2>${esc(t("inventory"))}</h2><button class="btn sec sm" id="inv_add">+ ${esc(t("add_item"))}</button></div>
      <div class="toolbar">
        ${divSel}
        <div class="field grow"><label>${esc(t("search"))}</label><input id="inv_q" value="${esc(invFilter.q)}" placeholder="${esc(t("item_name"))}…"></div>
      </div>
      <div id="invTable"></div>
    </div>`;
  const sel = view.querySelector("#inv_div");
  if (sel) sel.onchange = e => { invFilter.division = e.target.value; loadInventory(view); };
  let deb;
  view.querySelector("#inv_q").oninput = e => { invFilter.q = e.target.value; clearTimeout(deb); deb = setTimeout(() => loadInventory(view), 250); };
  view.querySelector("#inv_add").onclick = () => itemModal(null);
  loadInventory(view);
}

async function loadInventory(view) {
  const el = view.querySelector("#invTable");
  if (!el) return;
  const qs = new URLSearchParams();
  if (invFilter.division) qs.set("division", invFilter.division);
  if (invFilter.q) qs.set("q", invFilter.q);
  const items = await api("GET", "/api/inventory?" + qs.toString()).catch(() => []);
  const multi = state.myDivisions.length > 1;
  el.innerHTML = `<table><thead><tr>
      <th>${esc(t("item_name"))}</th><th>${esc(t("category"))}</th><th>${esc(t("unit"))}</th>
      <th>${esc(t("quantity"))}</th>${multi ? `<th>${esc(t("gn_division"))}</th>` : ""}<th></th></tr></thead>
    <tbody>${items.length ? items.map(it => {
      const low = it.min_quantity != null && Number(it.quantity) <= Number(it.min_quantity);
      return `<tr>
        <td>${esc(it.name)}</td>
        <td>${esc(it.category) || "—"}</td>
        <td>${esc(it.unit) || "—"}</td>
        <td><strong>${esc(fmtNum(it.quantity))}</strong>${low ? ` <span class="badge badge-warn">${esc(t("low_stock"))}</span>` : ""}</td>
        ${multi ? `<td>${esc((it.gn_code ? it.gn_code + " · " : "") + it.division_name)}</td>` : ""}
        <td style="text-align:right">
          <button class="btn sec sm" data-mov="${esc(it.id)}">${esc(t("movements"))}</button>
          <button class="btn sec sm" data-edit="${esc(it.id)}">${esc(t("edit"))}</button>
          <button class="btn danger sm" data-del="${esc(it.id)}">${esc(t("delete"))}</button></td></tr>`;
    }).join("")
      : `<tr><td colspan="${multi ? 6 : 5}" class="empty">${esc(t("no_items"))}</td></tr>`}</tbody></table>`;
  el.querySelectorAll("[data-mov]").forEach(b => b.onclick = () => movementsModal(items.find(i => i.id === b.dataset.mov)));
  el.querySelectorAll("[data-edit]").forEach(b => b.onclick = () => itemModal(items.find(i => i.id === b.dataset.edit)));
  el.querySelectorAll("[data-del]").forEach(b => b.onclick = async () => {
    if (!confirm(t("confirm_delete"))) return;
    await api("DELETE", "/api/inventory/" + b.dataset.del);
    loadInventory(view);
  });
}

function fmtNum(n) { n = Number(n) || 0; return Number.isInteger(n) ? String(n) : n.toFixed(2).replace(/\.?0+$/, ""); }

function itemModal(it) {
  it = it || {};
  const isNew = !it.id;
  const divOpts = state.myDivisions.map(d => `<option value="${esc(d.id)}" ${d.id === it.gn_division_id ? "selected" : ""}>${esc((d.gn_code ? d.gn_code + " · " : "") + (d["name_" + state.lang] || d.name_en))}</option>`).join("");
  const bg = document.createElement("div");
  bg.className = "modal-bg";
  bg.innerHTML = `<div class="modal">
    <h2>${esc(isNew ? t("add_item") : t("edit"))}</h2>
    <div class="grid g2">
      ${isNew ? `<div class="field"><label>${esc(t("gn_division"))} *</label><select id="i_div">${divOpts}</select></div>` : ""}
      <div class="field"><label>${esc(t("item_name"))} *</label><input id="i_name" value="${esc(it.name)}"></div>
      <div class="field"><label>${esc(t("category"))}</label><input id="i_cat" value="${esc(it.category)}"></div>
      <div class="field"><label>${esc(t("unit"))}</label><input id="i_unit" value="${esc(it.unit)}" placeholder="${esc(t("unit_hint"))}"></div>
      <div class="field"><label>${esc(t("reorder_level"))}</label><input id="i_min" type="number" step="any" value="${esc(it.min_quantity)}"></div>
      ${isNew ? `<div class="field"><label>${esc(t("opening_quantity"))}</label><input id="i_open" type="number" step="any" value=""></div>` : ""}
      <div class="field" style="grid-column:1/-1"><label>${esc(t("notes"))}</label><input id="i_notes" value="${esc(it.notes)}"></div>
    </div>
    <div class="row-actions"><button class="btn sec" id="i_cancel">${esc(t("cancel"))}</button><button class="btn" id="i_save">${esc(t("save"))}</button></div></div>`;
  document.body.appendChild(bg);
  bg.querySelector("#i_cancel").onclick = () => bg.remove();
  bg.onclick = e => { if (e.target === bg) bg.remove(); };
  bg.querySelector("#i_save").onclick = async () => {
    const q = s => { const e = bg.querySelector(s); return e ? e.value.trim() : ""; };
    const minV = q("#i_min");
    try {
      if (isNew) {
        const body = {
          gn_division_id: bg.querySelector("#i_div").value, name: q("#i_name"),
          category: q("#i_cat"), unit: q("#i_unit"), notes: q("#i_notes"),
          min_quantity: minV === "" ? null : Number(minV),
          opening_quantity: q("#i_open") === "" ? 0 : Number(q("#i_open")),
        };
        if (!body.name) return toast(t("item_name"));
        await api("POST", "/api/inventory", body);
      } else {
        await api("PUT", "/api/inventory/" + it.id, {
          name: q("#i_name"), category: q("#i_cat"), unit: q("#i_unit"), notes: q("#i_notes"),
          min_quantity: minV === "" ? null : Number(minV),
        });
      }
      bg.remove(); loadInventory(document.getElementById("view")); toast(t("saved"));
    } catch (e) { toast(e.message); }
  };
}

async function movementsModal(it) {
  const bg = document.createElement("div");
  bg.className = "modal-bg";
  bg.innerHTML = `<div class="modal wide">
    <h2>${esc(t("movements"))} — ${esc(it.name)}</h2>
    <p class="muted">${esc(t("current_balance"))}: <strong id="mv_bal">${esc(fmtNum(it.quantity))}</strong> ${esc(it.unit || "")}</p>
    <div class="toolbar">
      <div class="field"><label>${esc(t("movement_type"))}</label><select id="mv_type">
        <option value="in">${esc(t("stock_in"))}</option>
        <option value="out">${esc(t("stock_out"))}</option>
        <option value="adjust">${esc(t("adjust"))}</option></select></div>
      <div class="field"><label>${esc(t("quantity"))}</label><input id="mv_qty" type="number" step="any"></div>
      <div class="field"><label>${esc(t("date"))}</label><input id="mv_date" type="date"></div>
      <div class="field grow"><label>${esc(t("reference"))}</label><input id="mv_ref"></div>
      <button class="btn" id="mv_add">${esc(t("record_movement"))}</button>
    </div>
    <div id="mv_list"></div>
    <div class="row-actions"><button class="btn sec" id="mv_close">${esc(t("back"))}</button></div></div>`;
  document.body.appendChild(bg);
  bg.querySelector("#mv_close").onclick = () => { bg.remove(); loadInventory(document.getElementById("view")); };
  bg.onclick = e => { if (e.target === bg) { bg.remove(); loadInventory(document.getElementById("view")); } };

  const loadTxns = async () => {
    const txns = await api("GET", "/api/inventory/" + it.id + "/txns").catch(() => []);
    const typeLabel = { in: t("stock_in"), out: t("stock_out"), adjust: t("adjust") };
    bg.querySelector("#mv_list").innerHTML = `<table><thead><tr>
        <th>${esc(t("date"))}</th><th>${esc(t("movement_type"))}</th><th>${esc(t("quantity"))}</th>
        <th>${esc(t("balance"))}</th><th>${esc(t("reference"))}</th><th>${esc(t("notes"))}</th></tr></thead>
      <tbody>${txns.length ? txns.map(x => `<tr>
          <td>${esc(x.txn_date || (x.created_at || "").slice(0, 10))}</td>
          <td>${esc(typeLabel[x.txn_type] || x.txn_type)}</td>
          <td>${x.txn_type === "out" ? "−" : (x.txn_type === "in" ? "+" : "=")}${esc(fmtNum(x.quantity))}</td>
          <td>${esc(fmtNum(x.balance_after))}</td>
          <td>${esc(x.reference) || "—"}</td>
          <td>${esc(x.note) || "—"}</td></tr>`).join("")
        : `<tr><td colspan="6" class="empty">—</td></tr>`}</tbody></table>`;
  };
  bg.querySelector("#mv_add").onclick = async () => {
    const qty = bg.querySelector("#mv_qty").value;
    if (qty === "" || Number(qty) <= 0) return toast(t("quantity"));
    try {
      const r = await api("POST", "/api/inventory/" + it.id + "/txns", {
        txn_type: bg.querySelector("#mv_type").value, quantity: Number(qty),
        reference: bg.querySelector("#mv_ref").value.trim(), txn_date: bg.querySelector("#mv_date").value,
      });
      it.quantity = r.balance_after;
      bg.querySelector("#mv_bal").textContent = fmtNum(r.balance_after);
      bg.querySelector("#mv_qty").value = ""; bg.querySelector("#mv_ref").value = "";
      await loadTxns(); toast(t("saved"));
    } catch (e) { toast(e.message); }
  };
  loadTxns();
}

// ---------- work diary / day-book ----------
const diaryFilter = { division: "", from: "", to: "", q: "" };

async function renderDiary() {
  const view = document.getElementById("view");
  const divSel = state.myDivisions.length > 1
    ? `<div class="field"><label>${esc(t("gn_division"))}</label><select id="d_div">
        <option value="">${esc(t("all_divisions"))}</option>
        ${state.myDivisions.map(d => `<option value="${esc(d.id)}" ${diaryFilter.division === d.id ? "selected" : ""}>${esc((d.gn_code ? d.gn_code + " · " : "") + (d["name_" + state.lang] || d.name_en))}</option>`).join("")}</select></div>`
    : "";
  view.innerHTML = `
    <div class="card">
      <div class="section-sub"><h2>${esc(t("diary"))}</h2><button class="btn sec sm" id="d_add">+ ${esc(t("add_entry"))}</button></div>
      <div class="toolbar">
        ${divSel}
        <div class="field"><label>${esc(t("from_date"))}</label><input id="d_from" type="date" value="${esc(diaryFilter.from)}"></div>
        <div class="field"><label>${esc(t("to_date"))}</label><input id="d_to" type="date" value="${esc(diaryFilter.to)}"></div>
        <div class="field grow"><label>${esc(t("search"))}</label><input id="d_q" value="${esc(diaryFilter.q)}"></div>
      </div>
      <div id="diaryList"></div>
    </div>`;
  const sel = view.querySelector("#d_div");
  if (sel) sel.onchange = e => { diaryFilter.division = e.target.value; loadDiary(view); };
  view.querySelector("#d_from").onchange = e => { diaryFilter.from = e.target.value; loadDiary(view); };
  view.querySelector("#d_to").onchange = e => { diaryFilter.to = e.target.value; loadDiary(view); };
  let deb;
  view.querySelector("#d_q").oninput = e => { diaryFilter.q = e.target.value; clearTimeout(deb); deb = setTimeout(() => loadDiary(view), 250); };
  view.querySelector("#d_add").onclick = () => diaryModal(null);
  loadDiary(view);
}

async function loadDiary(view) {
  const el = view.querySelector("#diaryList");
  if (!el) return;
  const qs = new URLSearchParams();
  if (diaryFilter.division) qs.set("division", diaryFilter.division);
  if (diaryFilter.from) qs.set("from", diaryFilter.from);
  if (diaryFilter.to) qs.set("to", diaryFilter.to);
  if (diaryFilter.q) qs.set("q", diaryFilter.q);
  const rows = await api("GET", "/api/diary?" + qs.toString()).catch(() => []);
  const multi = state.myDivisions.length > 1;
  el.innerHTML = `<table><thead><tr>
      <th>${esc(t("date"))}</th><th>${esc(t("category"))}</th><th>${esc(t("entry"))}</th>
      <th>${esc(t("author"))}</th>${multi ? `<th>${esc(t("gn_division"))}</th>` : ""}<th></th></tr></thead>
    <tbody>${rows.length ? rows.map(d => `<tr>
        <td>${esc(d.entry_date)}</td>
        <td>${esc(d.category) || "—"}</td>
        <td>${d.title ? `<strong>${esc(d.title)}</strong>` : ""}${d.title && d.note ? "<br>" : ""}${esc(d.note) || (d.title ? "" : "—")}</td>
        <td>${esc(d.author) || "—"}</td>
        ${multi ? `<td>${esc((d.gn_code ? d.gn_code + " · " : "") + d.division_name)}</td>` : ""}
        <td style="text-align:right"><button class="btn sec sm" data-edit="${esc(d.id)}">${esc(t("edit"))}</button>
          <button class="btn danger sm" data-del="${esc(d.id)}">${esc(t("delete"))}</button></td></tr>`).join("")
      : `<tr><td colspan="${multi ? 6 : 5}" class="empty">${esc(t("no_entries"))}</td></tr>`}</tbody></table>`;
  el.querySelectorAll("[data-edit]").forEach(b => b.onclick = () => diaryModal(rows.find(x => x.id === b.dataset.edit)));
  el.querySelectorAll("[data-del]").forEach(b => b.onclick = async () => {
    if (!confirm(t("confirm_delete"))) return;
    await api("DELETE", "/api/diary/" + b.dataset.del); loadDiary(view);
  });
}

function diaryModal(d) {
  d = d || {};
  const isNew = !d.id;
  const today = new Date().toISOString().slice(0, 10);
  const divOpts = state.myDivisions.map(x => `<option value="${esc(x.id)}" ${x.id === d.gn_division_id ? "selected" : ""}>${esc((x.gn_code ? x.gn_code + " · " : "") + (x["name_" + state.lang] || x.name_en))}</option>`).join("");
  const bg = document.createElement("div");
  bg.className = "modal-bg";
  bg.innerHTML = `<div class="modal">
    <h2>${esc(isNew ? t("add_entry") : t("edit"))}</h2>
    <div class="grid g2">
      ${isNew ? `<div class="field"><label>${esc(t("gn_division"))} *</label><select id="d_m_div">${divOpts}</select></div>` : ""}
      <div class="field"><label>${esc(t("date"))}</label><input id="d_m_date" type="date" value="${esc(d.entry_date || today)}"></div>
      <div class="field"><label>${esc(t("category"))}</label><input id="d_m_cat" value="${esc(d.category)}" placeholder="${esc(t("diary_cat_hint"))}"></div>
      <div class="field" style="grid-column:1/-1"><label>${esc(t("title"))}</label><input id="d_m_title" value="${esc(d.title)}"></div>
      <div class="field" style="grid-column:1/-1"><label>${esc(t("note"))}</label><textarea id="d_m_note" rows="4">${esc(d.note)}</textarea></div>
    </div>
    <div class="row-actions"><button class="btn sec" id="d_m_cancel">${esc(t("cancel"))}</button><button class="btn" id="d_m_save">${esc(t("save"))}</button></div></div>`;
  document.body.appendChild(bg);
  bg.querySelector("#d_m_cancel").onclick = () => bg.remove();
  bg.onclick = e => { if (e.target === bg) bg.remove(); };
  bg.querySelector("#d_m_save").onclick = async () => {
    const q = s => { const e = bg.querySelector(s); return e ? e.value.trim() : ""; };
    const payload = { entry_date: q("#d_m_date"), category: q("#d_m_cat"), title: q("#d_m_title"), note: q("#d_m_note") };
    if (!payload.title && !payload.note) return toast(t("diary_need_text"));
    try {
      if (isNew) { payload.gn_division_id = bg.querySelector("#d_m_div").value; await api("POST", "/api/diary", payload); }
      else await api("PUT", "/api/diary/" + d.id, payload);
      bg.remove(); loadDiary(document.getElementById("view")); toast(t("saved"));
    } catch (e) { toast(e.message); }
  };
}

boot();
