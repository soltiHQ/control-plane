package spec

import (
	"encoding/json"
	"fmt"
	"strings"

	restv1 "github.com/soltiHQ/control-plane/api/rest/v1"
)

// backoffPreset defines a named backoff configuration preset.
type backoffPreset struct {
	Label   string  `json:"label"`
	Jitter  string  `json:"jitter"`
	FirstMs int64   `json:"firstMs"`
	MaxMs   int64   `json:"maxMs"`
	Factor  float64 `json:"factor"`
}

var presets = []backoffPreset{
	{Label: "Standard", Jitter: "none", FirstMs: 1000, MaxMs: 5000, Factor: 2.0},
	{Label: "Aggressive", Jitter: "full", FirstMs: 500, MaxMs: 30000, Factor: 3.0},
	{Label: "Gentle", Jitter: "equal", FirstMs: 2000, MaxMs: 10000, Factor: 1.5},
}

// builderXData returns the Alpine x-data expression for the spec
// builder. `seedJSON` must be a valid JSON value (either `null` or an
// object from builderSeed). When non-null the builder pre-populates
// itself from it, turning the create form into an edit form.
//
// Admission is NOT exposed as a field: the CP reconciles desired state and
// upgrades via ApplyTask, which force-replaces at the agent — the only policy
// that converges safely. There is no user knob.
//
// Subprocess supports two modes, reflecting `solti.task.v1.SubprocessTask.mode`:
//
//   - command — a direct binary to exec with args.
//   - script — a body of text (bash/python/node/custom interpreter)
//     passed to the runtime with a flag. The body is transmitted
//     base64-encoded (UTF-8 bytes) on the wire.
func builderXData(agentsEndpoint, seedJSON string) string {
	presetsJSON, _ := json.Marshal(presets)
	if seedJSON == "" {
		seedJSON = "null"
	}
	return fmt.Sprintf(`(function() {
  const seed = %s;

  // b64encode handles UTF-8 safely: btoa() alone chokes on non-Latin-1
  // characters (emoji, Cyrillic, CJK …). The agent expects base64 of
  // the raw UTF-8 bytes of the script body.
  const b64encode = (s) => {
    try {
      const bytes = new TextEncoder().encode(s);
      let bin = '';
      for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
      return btoa(bin);
    } catch (_) { return ''; }
  };
  const b64decode = (s) => {
    try {
      const bin = atob(s || '');
      const bytes = new Uint8Array(bin.length);
      for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
      return new TextDecoder().decode(bytes);
    } catch (_) { return ''; }
  };

  const base = {
    name: '', slot: '', kind_type: 'subprocess',
    timeout_ms: 30000, restart_type: 'never', interval_ms: 0,
    backoff_preset: 'standard',
    jitter: 'none', backoff_first_ms: 1000, backoff_max_ms: 5000, backoff_factor: 2.0,
    // CAS token. Populated from the seed when editing; stays 0 for
    // create (the server ignores it on create paths). The Save handler
    // echoes it back in createSpec.version so Upsert can reject stale
    // edits (HTTP 409).
    loaded_version: 0,

    // Subprocess common (both modes).
    subprocess_mode: 'command',  // 'command' | 'script'
    args: '', env_rows: [], cwd: '', fail_on_non_zero: true,

    // Command mode.
    cmd: '',

    // Script mode.
    script_runtime_kind: 'wellKnown', // 'wellKnown' | 'custom'
    script_runtime_well_known: 'SCRIPT_RUNTIME_BASH',
    script_runtime_custom_command: '',
    script_runtime_custom_flag: '',
    script_body: '',
    script_drag_over: false,

    // Other kinds — raw JSON for now.
    wasm_json: '{}', container_json: '{}',

    // Targets / labels.
    target_mode: 'agents',
    agents: [], agents_opts: [], agents_open: false,
    label_rows: [],
    runner_label_rows: [],
  };

  // Apply seed fields over base. Missing seed fields keep base defaults
  // so a forward-compatible REST DTO (new field added) doesn't break
  // older cached UI code.
  if (seed) {
    for (const k of [
      'name','slot','kind_type','timeout_ms','restart_type','interval_ms',
      'jitter','backoff_first_ms','backoff_max_ms','backoff_factor',
    ]) {
      if (seed[k] !== undefined && seed[k] !== null) base[k] = seed[k];
    }
    // CAS token for optimistic concurrency on Save.
    if (typeof seed.version === 'number') base.loaded_version = seed.version;
    const kc = seed.kind_config || {};
    if (base.kind_type === 'subprocess') {
      if (kc.script) {
        base.subprocess_mode = 'script';
        if (kc.script.wellKnown) {
          base.script_runtime_kind = 'wellKnown';
          base.script_runtime_well_known = kc.script.wellKnown;
        } else if (kc.script.custom) {
          base.script_runtime_kind = 'custom';
          base.script_runtime_custom_command = kc.script.custom.command || '';
          base.script_runtime_custom_flag = kc.script.custom.flag || '';
        }
        base.script_body = b64decode(kc.script.body);
        base.args = Array.isArray(kc.script.args) ? kc.script.args.join(' ') : '';
      } else {
        base.subprocess_mode = 'command';
        const cmd = (kc.command || {});
        base.cmd = cmd.command || '';
        base.args = Array.isArray(cmd.args) ? cmd.args.join(' ') : '';
      }
      if (Array.isArray(kc.env)) {
        base.env_rows = kc.env.map(e => ({key: e.key || '', value: e.value || ''}));
      }
      base.cwd = kc.cwd || '';
      if (typeof kc.failOnNonZero === 'boolean') base.fail_on_non_zero = kc.failOnNonZero;
    } else if (base.kind_type === 'wasm') {
      base.wasm_json = JSON.stringify(kc, null, 2);
    } else if (base.kind_type === 'container') {
      base.container_json = JSON.stringify(kc, null, 2);
    }
    if (Array.isArray(seed.targets)) {
      base.agents = seed.targets.slice();
      base.target_mode = 'agents';
    }
    if (seed.target_labels && Object.keys(seed.target_labels).length) {
      base.label_rows = Object.entries(seed.target_labels).map(([k,v]) => ({key:k,value:v}));
      if (!base.agents.length) base.target_mode = 'labels';
    }
    if (seed.runner_labels) {
      base.runner_label_rows = Object.entries(seed.runner_labels).map(([k,v]) => ({key:k,value:v}));
    }
    base.backoff_preset = 'custom';
  }

  return {
    ...base,

    presets: %s,
    submitting: false,
    agents_endpoint: '%s',

    // Canonical proto3-JSON for solti.task.v1.TaskKind payloads.
    //
    // SubprocessTask: oneof {command|script} is inlined (NO "mode"
    // wrapper); env is repeated KeyValue ([{key, value}]), not a map.
    // Script body is base64 of UTF-8 bytes.
    //
    // WasmTask / ContainerTask: passed through verbatim from the raw
    // JSON textareas.
    get kindConfig() {
      if (this.kind_type === 'subprocess') {
        const cfg = {};
        if (this.subprocess_mode === 'script') {
          const script = { body: b64encode(this.script_body) };
          if (this.script_runtime_kind === 'custom') {
            script.custom = {
              command: this.script_runtime_custom_command,
              flag: this.script_runtime_custom_flag,
            };
          } else {
            script.wellKnown = this.script_runtime_well_known;
          }
          const a = this.args.split(/\s+/).filter(Boolean);
          if (a.length) script.args = a;
          cfg.script = script;
        } else {
          const cmdObj = {};
          if (this.cmd) cmdObj.command = this.cmd;
          const a = this.args.split(/\s+/).filter(Boolean);
          if (a.length) cmdObj.args = a;
          cfg.command = cmdObj;
        }
        const envList = [];
        for (const r of this.env_rows) {
          const k = r.key.trim();
          if (k) envList.push({ key: k, value: r.value });
        }
        if (envList.length) cfg.env = envList;
        if (this.cwd) cfg.cwd = this.cwd;
        cfg.failOnNonZero = this.fail_on_non_zero;
        return cfg;
      }
      if (this.kind_type === 'wasm') { try { return JSON.parse(this.wasm_json); } catch { return {}; } }
      if (this.kind_type === 'container') { try { return JSON.parse(this.container_json); } catch { return {}; } }
      return {};
    },

    get targetLabels() {
      const out = {};
      for (const r of this.label_rows) { const k = r.key.trim(); if (k) out[k] = r.value; }
      return out;
    },

    get runnerLabels() {
      const out = {};
      for (const r of this.runner_label_rows) { const k = r.key.trim(); if (k) out[k] = r.value; }
      return out;
    },

    get createSpec() {
      const spec = {
        name: this.name, slot: this.slot,
        kind_type: this.kind_type, kind_config: this.kindConfig,
        timeout_ms: Number(this.timeout_ms),
        restart_type: this.restart_type,
        jitter: this.jitter,
        backoff_first_ms: Number(this.backoff_first_ms),
        backoff_max_ms: Number(this.backoff_max_ms),
        backoff_factor: Number(this.backoff_factor),
      };
      if (this.loaded_version > 0) {
        spec.version = this.loaded_version;
      }
      if (this.restart_type === 'always' && this.interval_ms > 0) {
        spec.interval_ms = Number(this.interval_ms);
      }
      if (this.target_mode === 'agents' && this.agents.length) {
        spec.targets = this.agents;
      }
      const tl = this.targetLabels;
      if (Object.keys(tl).length) spec.target_labels = tl;
      const rl = this.runnerLabels;
      if (Object.keys(rl).length) spec.runner_labels = rl;
      return spec;
    },

    get previewJSON() {
      return JSON.stringify(this.createSpec, null, 2);
    },

    applyPreset(name) {
      const p = this.presets.find(x => x.label.toLowerCase() === name);
      if (p) {
        this.jitter = p.jitter;
        this.backoff_first_ms = p.firstMs;
        this.backoff_max_ms = p.maxMs;
        this.backoff_factor = p.factor;
      }
      this.backoff_preset = name;
    },

    // onScriptFileDrop / onScriptFileChange read the dropped (or
    // picked) file as text and place it into script_body. A single
    // file is taken — the wire format is a single-blob body, not an
    // archive.
    onScriptFileDrop(evt) {
      this.script_drag_over = false;
      const f = evt.dataTransfer && evt.dataTransfer.files ? evt.dataTransfer.files[0] : null;
      this._readScriptFile(f);
    },
    onScriptFileChange(evt) {
      const f = evt.target && evt.target.files ? evt.target.files[0] : null;
      this._readScriptFile(f);
    },
    _readScriptFile(f) {
      if (!f) return;
      const reader = new FileReader();
      reader.onload = () => { this.script_body = String(reader.result || ''); };
      reader.readAsText(f);
    },
  };
})()`, seedJSON, string(presetsJSON), agentsEndpoint)
}

// builderSeed renders a Spec DTO as a JSON literal suitable for
// embedding in Alpine x-data. Returns `null` when no initial value is
// provided (create mode).
func builderSeed(initial *restv1.Spec) string {
	if initial == nil {
		return "null"
	}
	b, err := json.Marshal(initial)
	if err != nil {
		// On a marshalling error fall back to "null": the form will
		// render blank and the user can retype. Swallowing the error
		// here is preferable to an outright render failure.
		return "null"
	}
	return string(b)
}

// builderInitExpr returns the Alpine x-init expression that loads active agents.
func builderInitExpr() string {
	return `fetch(agents_endpoint).then(r => r.json()).then(d => {
  agents_opts = (d.items || []).map(a => a.id);
}).catch(() => {})`
}

// builderSubmitExpr returns the Alpine submit expression for the
// builder form. `method` is "POST" for create, "PUT" for edit;
// `endpoint` is the full URL.
//
// On 409 we surface the optimistic-concurrency conflict to the user
// through an alert and reload the page so they pick up the latest
// version — silently accepting the reject would strand the edits they
// typed without explanation.
func builderSubmitExpr(endpoint, method string) string {
	return fmt.Sprintf(
		`submitting = true;
fetch('%s', {
  method: '%s',
  headers: { 'Content-Type': 'application/json', 'HX-Request': 'true' },
  body: JSON.stringify(createSpec)
}).then(async r => {
  if (r.ok) {
    const redirect = r.headers.get('HX-Redirect');
    if (redirect) { window.location.href = redirect; return; }
    show = false;
    htmx.trigger(document.body, 'spec_update');
    return;
  }
  if (r.status === 409) {
    let msg = 'This spec was modified by someone else while you were editing. The page will reload with the latest version.';
    try {
      const body = await r.json();
      if (body && body.message) msg = body.message + '\n\nThe page will reload.';
    } catch {}
    alert(msg);
    window.location.reload();
  }
}).catch(() => {}).finally(() => submitting = false)`,
		strings.ReplaceAll(endpoint, "'", "\\'"),
		method,
	)
}
