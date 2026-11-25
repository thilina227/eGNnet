-- eGNnet schema — Grama Niladhari household records
-- SQLite. UUIDs stored as TEXT, booleans as INTEGER (0/1), dates/datetimes as ISO TEXT.
PRAGMA foreign_keys = ON;

-- ---------- Administration hierarchy (reference) ----------
CREATE TABLE IF NOT EXISTS province (
  id         TEXT PRIMARY KEY,
  code       TEXT,
  name_en    TEXT NOT NULL,
  name_si    TEXT,
  name_ta    TEXT
);

CREATE TABLE IF NOT EXISTS district (
  id          TEXT PRIMARY KEY,
  province_id TEXT NOT NULL REFERENCES province(id),
  code        TEXT,
  name_en     TEXT NOT NULL,
  name_si     TEXT,
  name_ta     TEXT
);

CREATE TABLE IF NOT EXISTS ds_division (
  id          TEXT PRIMARY KEY,
  district_id TEXT NOT NULL REFERENCES district(id),
  code        TEXT,
  name_en     TEXT NOT NULL,
  name_si     TEXT,
  name_ta     TEXT
);

CREATE TABLE IF NOT EXISTS gn_division (
  id             TEXT PRIMARY KEY,
  ds_division_id TEXT NOT NULL REFERENCES ds_division(id),
  gn_code        TEXT,
  name_en        TEXT NOT NULL,
  name_si        TEXT,
  name_ta        TEXT
);

-- ---------- Officers & access ----------
CREATE TABLE IF NOT EXISTS officer (
  id          TEXT PRIMARY KEY,
  full_name   TEXT NOT NULL,
  nic         TEXT,
  contact     TEXT,
  email       TEXT,
  photo_path  TEXT,
  started_date TEXT,
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL,
  deleted_at  TEXT
);

CREATE TABLE IF NOT EXISTS officer_assignment (
  id             TEXT PRIMARY KEY,
  officer_id     TEXT NOT NULL REFERENCES officer(id),
  gn_division_id TEXT NOT NULL REFERENCES gn_division(id),
  start_date     TEXT,
  end_date       TEXT,
  is_current     INTEGER NOT NULL DEFAULT 1,
  created_at     TEXT NOT NULL,
  updated_at     TEXT NOT NULL,
  deleted_at     TEXT
);
CREATE INDEX IF NOT EXISTS ix_assign_officer ON officer_assignment(officer_id, is_current);
CREATE INDEX IF NOT EXISTS ix_assign_division ON officer_assignment(gn_division_id, is_current);

CREATE TABLE IF NOT EXISTS app_user (
  id            TEXT PRIMARY KEY,
  officer_id    TEXT REFERENCES officer(id),
  username      TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role          TEXT NOT NULL DEFAULT 'gn_officer', -- gn_officer | ds_admin | district_admin | province_admin | super_admin
  scope_level   TEXT,   -- gn | ds | district | province | national
  scope_id      TEXT,
  language_pref TEXT NOT NULL DEFAULT 'en',
  is_active     INTEGER NOT NULL DEFAULT 1,
  last_login    TEXT,
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL,
  deleted_at    TEXT
);

CREATE TABLE IF NOT EXISTS audit_log (
  id           TEXT PRIMARY KEY,
  user_id      TEXT,
  entity_table TEXT NOT NULL,
  entity_id    TEXT NOT NULL,
  action       TEXT NOT NULL,       -- create | update | delete
  changes      TEXT,                -- JSON
  created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_audit_entity ON audit_log(entity_table, entity_id);
CREATE INDEX IF NOT EXISTS ix_audit_time ON audit_log(created_at);

-- ---------- Household & housing ----------
CREATE TABLE IF NOT EXISTS household (
  id               TEXT PRIMARY KEY,
  gn_division_id   TEXT NOT NULL REFERENCES gn_division(id),
  household_number TEXT,
  village          TEXT,
  village_part     TEXT,
  road             TEXT,
  post_office      TEXT,
  address          TEXT,
  head_name        TEXT,
  phone_home       TEXT,
  phone_mobile     TEXT,
  members_female   INTEGER,
  members_male     INTEGER,
  members_total    INTEGER,
  family_units     INTEGER,
  living_arrangement TEXT,          -- alone | shared
  shared_with      TEXT,
  gps_lat          REAL,
  gps_lng          REAL,
  file_name        TEXT,
  file_code        TEXT,
  file_number      TEXT,
  surveyed_by      TEXT REFERENCES officer(id),
  survey_date      TEXT,
  created_at       TEXT NOT NULL,
  updated_at       TEXT NOT NULL,
  deleted_at       TEXT
);
CREATE INDEX IF NOT EXISTS ix_household_division ON household(gn_division_id);
CREATE INDEX IF NOT EXISTS ix_household_number ON household(household_number);
CREATE INDEX IF NOT EXISTS ix_household_village ON household(village);

CREATE TABLE IF NOT EXISTS housing (
  id             TEXT PRIMARY KEY,
  household_id   TEXT NOT NULL UNIQUE REFERENCES household(id),
  plot_ownership TEXT,
  deed_no        TEXT,
  lot_no         TEXT,
  plot_acres     INTEGER,
  plot_roods     INTEGER,
  plot_perches   REAL,
  house_length   REAL,
  house_width    REAL,
  floor_area     REAL,
  room_count     INTEGER,
  year_built     INTEGER,
  year_occupied  INTEGER,
  house_ownership TEXT,
  house_type     TEXT,
  wall_type      TEXT,
  roof_type      TEXT,
  floor_type     TEXT,
  toilet_type    TEXT,
  drinking_water TEXT,
  other_water    TEXT,
  has_electricity INTEGER,
  monthly_electricity_bill REAL,
  no_power_alternative TEXT,
  cooking_fuel   TEXT,
  telephone_fixed INTEGER,
  mobile_normal  INTEGER,
  mobile_smart   INTEGER,
  access_road    INTEGER,
  vehicle_access INTEGER,
  garbage_disposal TEXT,
  created_at     TEXT NOT NULL,
  updated_at     TEXT NOT NULL,
  deleted_at     TEXT
);

CREATE TABLE IF NOT EXISTS household_service (
  id           TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES household(id),
  service_type TEXT NOT NULL,       -- lookup: communication_service
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  deleted_at   TEXT
);

-- ---------- Members & welfare ----------
CREATE TABLE IF NOT EXISTS member (
  id             TEXT PRIMARY KEY,
  household_id   TEXT NOT NULL REFERENCES household(id),
  serial_no      INTEGER,
  full_name_en   TEXT,
  full_name_si   TEXT,
  relationship_to_head TEXT,
  is_head        INTEGER NOT NULL DEFAULT 0,
  title          TEXT,
  nic            TEXT,
  passport_no    TEXT,
  dob            TEXT,
  age            INTEGER,
  gender         TEXT,
  civil_status   TEXT,
  ethnicity      TEXT,
  religion       TEXT,
  mother_language TEXT,
  nationality    TEXT,
  residant_status TEXT,
  birthplace     TEXT,
  education_level TEXT,
  occupation     TEXT,
  occupation_detail TEXT,
  job_status     TEXT,
  mobile         TEXT,
  email          TEXT,
  photo_path     TEXT,
  created_at     TEXT NOT NULL,
  updated_at     TEXT NOT NULL,
  deleted_at     TEXT
);
CREATE INDEX IF NOT EXISTS ix_member_household ON member(household_id);
CREATE INDEX IF NOT EXISTS ix_member_nic ON member(nic);
CREATE INDEX IF NOT EXISTS ix_member_name ON member(full_name_en);

CREATE TABLE IF NOT EXISTS member_flag (
  id         TEXT PRIMARY KEY,
  member_id  TEXT NOT NULL REFERENCES member(id),
  flag_type  TEXT NOT NULL,         -- lookup: member_flag
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT
);

CREATE TABLE IF NOT EXISTS household_event (
  id           TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES household(id),
  member_id    TEXT REFERENCES member(id),
  event_type   TEXT NOT NULL,       -- birth | death
  person_name  TEXT,
  event_date   TEXT,
  details      TEXT,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  deleted_at   TEXT
);

CREATE TABLE IF NOT EXISTS welfare_program (
  id           TEXT PRIMARY KEY,
  code         TEXT,
  name_en      TEXT NOT NULL,
  name_si      TEXT,
  name_ta      TEXT,
  active       INTEGER NOT NULL DEFAULT 1,
  started_date TEXT,
  ended_date   TEXT,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  deleted_at   TEXT
);

CREATE TABLE IF NOT EXISTS welfare_enrolment (
  id              TEXT PRIMARY KEY,
  household_id    TEXT NOT NULL REFERENCES household(id),
  member_id       TEXT REFERENCES member(id),
  program_id      TEXT NOT NULL REFERENCES welfare_program(id),
  beneficiary_ref TEXT,
  amount          REAL,
  status          TEXT,             -- active | suspended | ended
  start_date      TEXT,
  end_date        TEXT,
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL,
  deleted_at      TEXT
);
CREATE INDEX IF NOT EXISTS ix_welfare_household ON welfare_enrolment(household_id);

-- ---------- Land & agriculture ----------
CREATE TABLE IF NOT EXISTS land_holding (
  id           TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES household(id),
  land_class   TEXT,                -- private_freehold | state | paddy
  tenure_type  TEXT,                -- deed | reserved | lrc | unauthorized | permit
  acres        INTEGER,
  roods        INTEGER,
  perches      REAL,
  is_main      INTEGER DEFAULT 0,
  notes        TEXT,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  deleted_at   TEXT
);

CREATE TABLE IF NOT EXISTS land_crop (
  id              TEXT PRIMARY KEY,
  household_id    TEXT NOT NULL REFERENCES household(id),
  crop_type       TEXT,             -- lookup: crop_type
  extent_or_trees TEXT,
  is_main         INTEGER DEFAULT 0,
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL,
  deleted_at      TEXT
);

CREATE TABLE IF NOT EXISTS paddy_cultivation (
  id           TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES household(id),
  season       TEXT,                -- yala | maha
  extent       TEXT,
  method       TEXT,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  deleted_at   TEXT
);

-- ---------- Assets ----------
CREATE TABLE IF NOT EXISTS livestock (
  id           TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES household(id),
  animal_type  TEXT,                -- lookup: livestock_type
  count        INTEGER,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  deleted_at   TEXT
);

CREATE TABLE IF NOT EXISTS poultry (
  id           TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES household(id),
  poultry_type TEXT,                -- lookup: poultry_type
  count        INTEGER,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  deleted_at   TEXT
);

CREATE TABLE IF NOT EXISTS appliance (
  id             TEXT PRIMARY KEY,
  household_id   TEXT NOT NULL REFERENCES household(id),
  appliance_type TEXT,              -- lookup: appliance_type
  count          INTEGER,
  created_at     TEXT NOT NULL,
  updated_at     TEXT NOT NULL,
  deleted_at     TEXT
);

CREATE TABLE IF NOT EXISTS vehicle (
  id              TEXT PRIMARY KEY,
  household_id    TEXT NOT NULL REFERENCES household(id),
  vehicle_type    TEXT,             -- lookup: vehicle_type
  registration_no TEXT,
  count           INTEGER DEFAULT 1,
  income_use      INTEGER DEFAULT 0,
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL,
  deleted_at      TEXT
);

CREATE TABLE IF NOT EXISTS equipment (
  id             TEXT PRIMARY KEY,
  household_id   TEXT NOT NULL REFERENCES household(id),
  equipment_type TEXT,              -- lookup: equipment_type
  count          INTEGER DEFAULT 1,
  income_use     INTEGER DEFAULT 0,
  created_at     TEXT NOT NULL,
  updated_at     TEXT NOT NULL,
  deleted_at     TEXT
);

CREATE TABLE IF NOT EXISTS permit (
  id           TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES household(id),
  member_id    TEXT REFERENCES member(id),
  permit_type  TEXT,                -- firearm | kithul
  permit_no    TEXT,
  holder_name  TEXT,
  valid_until  TEXT,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  deleted_at   TEXT
);

-- ---------- Economy ----------
CREATE TABLE IF NOT EXISTS loan (
  id             TEXT PRIMARY KEY,
  household_id   TEXT NOT NULL REFERENCES household(id),
  is_microfinance INTEGER DEFAULT 0,
  provider       TEXT,              -- samurdhi_bank | public_bank | other
  details        TEXT,
  created_at     TEXT NOT NULL,
  updated_at     TEXT NOT NULL,
  deleted_at     TEXT
);

CREATE TABLE IF NOT EXISTS state_aid (
  id           TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES household(id),
  project      TEXT,
  year         INTEGER,
  aid_item     TEXT,
  progress     TEXT,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  deleted_at   TEXT
);

CREATE TABLE IF NOT EXISTS household_finance (
  id                     TEXT PRIMARY KEY,
  household_id           TEXT NOT NULL UNIQUE REFERENCES household(id),
  monthly_income_bracket TEXT,      -- lookup: income_bracket
  created_at             TEXT NOT NULL,
  updated_at             TEXT NOT NULL,
  deleted_at             TEXT
);

CREATE TABLE IF NOT EXISTS income_source (
  id           TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES household(id),
  source_type  TEXT,                -- lookup: income_source
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  deleted_at   TEXT
);

CREATE TABLE IF NOT EXISTS monthly_expense (
  id           TEXT PRIMARY KEY,
  household_id TEXT NOT NULL REFERENCES household(id),
  category     TEXT,                -- food | loans_savings | education | health | other
  amount       REAL,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  deleted_at   TEXT
);

-- ---------- Lookups & localisation ----------
CREATE TABLE IF NOT EXISTS lookup_option (
  id         TEXT PRIMARY KEY,
  category   TEXT NOT NULL,
  code       TEXT NOT NULL,
  name_en    TEXT NOT NULL,
  name_si    TEXT,
  name_ta    TEXT,
  sort_order INTEGER DEFAULT 0,
  active     INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS ix_lookup_category ON lookup_option(category, sort_order);
CREATE UNIQUE INDEX IF NOT EXISTS ux_lookup_cat_code ON lookup_option(category, code);

CREATE TABLE IF NOT EXISTS ui_string (
  key   TEXT NOT NULL,
  lang  TEXT NOT NULL,
  value TEXT NOT NULL,
  PRIMARY KEY (key, lang)
);

-- ---------- App configuration (single row) ----------
CREATE TABLE IF NOT EXISTS app_config (
  id              INTEGER PRIMARY KEY CHECK (id = 1),
  setup_complete  INTEGER NOT NULL DEFAULT 0,
  default_language TEXT NOT NULL DEFAULT 'en',
  office_name     TEXT,
  log_retention_days INTEGER NOT NULL DEFAULT 180,
  created_at      TEXT,
  updated_at      TEXT
);

-- Operational / application log, DISTINCT from audit_log.
-- audit_log = per-row data-change trail (what citizen data changed).
-- app_log   = operational events: logins, errors, admin actions, sync, system.
-- Records actors + actions only; never citizen PII.
CREATE TABLE IF NOT EXISTS app_log (
  id          TEXT PRIMARY KEY,
  level       TEXT NOT NULL DEFAULT 'info',   -- info | warn | error
  category    TEXT NOT NULL DEFAULT 'system', -- auth | admin | sync | error | system
  user_id     TEXT,
  username    TEXT,
  action      TEXT NOT NULL,
  detail      TEXT,
  created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_applog_time ON app_log(created_at);
CREATE INDEX IF NOT EXISTS ix_applog_cat ON app_log(category, created_at);

-- Office inventory: a stock register (inventory_item) with a movement
-- history (inventory_txn). Division-scoped, like households.
CREATE TABLE IF NOT EXISTS inventory_item (
  id             TEXT PRIMARY KEY,
  gn_division_id TEXT NOT NULL REFERENCES gn_division(id),
  name           TEXT NOT NULL,
  category       TEXT,
  unit           TEXT,                      -- pcs, box, kg, ...
  quantity       REAL NOT NULL DEFAULT 0,   -- running balance
  min_quantity   REAL,                      -- optional reorder level
  notes          TEXT,
  created_at     TEXT NOT NULL,
  updated_at     TEXT NOT NULL,
  deleted_at     TEXT
);
CREATE INDEX IF NOT EXISTS ix_inv_div ON inventory_item(gn_division_id, deleted_at);

CREATE TABLE IF NOT EXISTS inventory_txn (
  id             TEXT PRIMARY KEY,
  item_id        TEXT NOT NULL REFERENCES inventory_item(id),
  txn_type       TEXT NOT NULL,             -- in | out | adjust
  quantity       REAL NOT NULL,             -- positive magnitude entered
  balance_after  REAL,                      -- running balance snapshot
  reference      TEXT,                      -- GRN / issue note / ref
  note           TEXT,
  user_id        TEXT,
  txn_date       TEXT,                      -- date of movement (user-set)
  created_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_invtxn_item ON inventory_txn(item_id, created_at);

-- Work diary / day-book: the officer's dated log of office work.
-- Division-scoped; records the author (user) but no citizen PII by design.
CREATE TABLE IF NOT EXISTS diary_entry (
  id             TEXT PRIMARY KEY,
  gn_division_id TEXT NOT NULL REFERENCES gn_division(id),
  user_id        TEXT,
  entry_date     TEXT NOT NULL,             -- date of the work (user-set)
  category       TEXT,
  title          TEXT,
  note           TEXT,
  created_at     TEXT NOT NULL,
  updated_at     TEXT NOT NULL,
  deleted_at     TEXT
);
CREATE INDEX IF NOT EXISTS ix_diary_div ON diary_entry(gn_division_id, entry_date);
